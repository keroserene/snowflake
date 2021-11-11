package main

import (
	"bytes"
	"container/heap"
	"fmt"
	"log"
	"net"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/messages"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	ClientTimeout = 10
	ProxyTimeout  = 10

	NATUnknown      = "unknown"
	NATRestricted   = "restricted"
	NATUnrestricted = "unrestricted"
)

type clientVersion int

const (
	v1 clientVersion = iota
)

type IPC struct {
	ctx *BrokerContext
}

func (i *IPC) Debug(_ interface{}, response *string) error {
	var webexts, browsers, standalones, unknowns int
	var natRestricted, natUnrestricted, natUnknown int

	i.ctx.snowflakeLock.Lock()
	s := fmt.Sprintf("current snowflakes available: %d\n", len(i.ctx.idToSnowflake))
	for _, snowflake := range i.ctx.idToSnowflake {
		if snowflake.proxyType == "badge" {
			browsers++
		} else if snowflake.proxyType == "webext" {
			webexts++
		} else if snowflake.proxyType == "standalone" {
			standalones++
		} else {
			unknowns++
		}

		switch snowflake.natType {
		case NATRestricted:
			natRestricted++
		case NATUnrestricted:
			natUnrestricted++
		default:
			natUnknown++
		}

	}
	i.ctx.snowflakeLock.Unlock()

	s += fmt.Sprintf("\tstandalone proxies: %d", standalones)
	s += fmt.Sprintf("\n\tbrowser proxies: %d", browsers)
	s += fmt.Sprintf("\n\twebext proxies: %d", webexts)
	s += fmt.Sprintf("\n\tunknown proxies: %d", unknowns)

	s += fmt.Sprintf("\nNAT Types available:")
	s += fmt.Sprintf("\n\trestricted: %d", natRestricted)
	s += fmt.Sprintf("\n\tunrestricted: %d", natUnrestricted)
	s += fmt.Sprintf("\n\tunknown: %d", natUnknown)

	*response = s
	return nil
}

func (i *IPC) ProxyPolls(arg messages.Arg, response *[]byte) error {
	sid, proxyType, natType, clients, err := messages.DecodePollRequest(arg.Body)
	if err != nil {
		return messages.ErrBadRequest
	}

	// Log geoip stats
	remoteIP, _, err := net.SplitHostPort(arg.RemoteAddr)
	if err != nil {
		log.Println("Error processing proxy IP: ", err.Error())
	} else {
		i.ctx.metrics.lock.Lock()
		i.ctx.metrics.UpdateCountryStats(remoteIP, proxyType, natType)
		i.ctx.metrics.lock.Unlock()
	}

	var b []byte

	// Wait for a client to avail an offer to the snowflake, or timeout if nil.
	offer := i.ctx.RequestOffer(sid, proxyType, natType, clients)

	if offer == nil {
		i.ctx.metrics.lock.Lock()
		i.ctx.metrics.proxyIdleCount++
		i.ctx.metrics.promMetrics.ProxyPollTotal.With(prometheus.Labels{"nat": natType, "status": "idle"}).Inc()
		i.ctx.metrics.lock.Unlock()

		b, err = messages.EncodePollResponse("", false, "")
		if err != nil {
			return messages.ErrInternal
		}

		*response = b
		return nil
	}

	i.ctx.metrics.promMetrics.ProxyPollTotal.With(prometheus.Labels{"nat": natType, "status": "matched"}).Inc()
	b, err = messages.EncodePollResponse(string(offer.sdp), true, offer.natType)
	if err != nil {
		return messages.ErrInternal
	}
	*response = b

	return nil
}

func sendClientResponse(resp *messages.ClientPollResponse, response *[]byte) error {
	data, err := resp.EncodePollResponse()
	if err != nil {
		log.Printf("error encoding answer")
		return messages.ErrInternal
	} else {
		*response = []byte(data)
		return nil
	}
}

func (i *IPC) ClientOffers(arg messages.Arg, response *[]byte) error {
	var version clientVersion

	startTime := time.Now()
	body := arg.Body

	parts := bytes.SplitN(body, []byte("\n"), 2)
	if len(parts) < 2 {
		// no version number found
		err := fmt.Errorf("unsupported message version")
		return sendClientResponse(&messages.ClientPollResponse{Error: err.Error()}, response)
	}
	body = parts[1]
	if string(parts[0]) == "1.0" {
		version = v1
	} else {
		err := fmt.Errorf("unsupported message version")
		return sendClientResponse(&messages.ClientPollResponse{Error: err.Error()}, response)
	}

	var offer *ClientOffer
	switch version {
	case v1:
		req, err := messages.DecodeClientPollRequest(body)
		if err != nil {
			return sendClientResponse(&messages.ClientPollResponse{Error: err.Error()}, response)
		}
		offer = &ClientOffer{
			natType: req.NAT,
			sdp:     []byte(req.Offer),
		}
	default:
		panic("unknown version")
	}

	// Only hand out known restricted snowflakes to unrestricted clients
	var snowflakeHeap *SnowflakeHeap
	if offer.natType == NATUnrestricted {
		snowflakeHeap = i.ctx.restrictedSnowflakes
	} else {
		snowflakeHeap = i.ctx.snowflakes
	}

	// Immediately fail if there are no snowflakes available.
	i.ctx.snowflakeLock.Lock()
	numSnowflakes := snowflakeHeap.Len()
	i.ctx.snowflakeLock.Unlock()
	if numSnowflakes <= 0 {
		i.ctx.metrics.lock.Lock()
		i.ctx.metrics.clientDeniedCount++
		i.ctx.metrics.promMetrics.ClientPollTotal.With(prometheus.Labels{"nat": offer.natType, "status": "denied"}).Inc()
		if offer.natType == NATUnrestricted {
			i.ctx.metrics.clientUnrestrictedDeniedCount++
		} else {
			i.ctx.metrics.clientRestrictedDeniedCount++
		}
		i.ctx.metrics.lock.Unlock()
		switch version {
		case v1:
			resp := &messages.ClientPollResponse{Error: messages.StrNoProxies}
			return sendClientResponse(resp, response)
		default:
			panic("unknown version")
		}
	}

	// Otherwise, find the most available snowflake proxy, and pass the offer to it.
	// Delete must be deferred in order to correctly process answer request later.
	i.ctx.snowflakeLock.Lock()
	snowflake := heap.Pop(snowflakeHeap).(*Snowflake)
	i.ctx.snowflakeLock.Unlock()
	snowflake.offerChannel <- offer

	var err error

	// Wait for the answer to be returned on the channel or timeout.
	select {
	case answer := <-snowflake.answerChannel:
		i.ctx.metrics.lock.Lock()
		i.ctx.metrics.clientProxyMatchCount++
		i.ctx.metrics.promMetrics.ClientPollTotal.With(prometheus.Labels{"nat": offer.natType, "status": "matched"}).Inc()
		i.ctx.metrics.lock.Unlock()
		switch version {
		case v1:
			resp := &messages.ClientPollResponse{Answer: answer}
			err = sendClientResponse(resp, response)
		default:
			panic("unknown version")
		}
		// Initial tracking of elapsed time.
		i.ctx.metrics.clientRoundtripEstimate = time.Since(startTime) / time.Millisecond
	case <-time.After(time.Second * ClientTimeout):
		log.Println("Client: Timed out.")
		switch version {
		case v1:
			resp := &messages.ClientPollResponse{Error: messages.StrTimedOut}
			err = sendClientResponse(resp, response)
		default:
			panic("unknown version")
		}
	}

	i.ctx.snowflakeLock.Lock()
	i.ctx.metrics.promMetrics.AvailableProxies.With(prometheus.Labels{"nat": snowflake.natType, "type": snowflake.proxyType}).Dec()
	delete(i.ctx.idToSnowflake, snowflake.id)
	i.ctx.snowflakeLock.Unlock()

	return err
}

func (i *IPC) ProxyAnswers(arg messages.Arg, response *[]byte) error {
	answer, id, err := messages.DecodeAnswerRequest(arg.Body)
	if err != nil || answer == "" {
		return messages.ErrBadRequest
	}

	var success = true
	i.ctx.snowflakeLock.Lock()
	snowflake, ok := i.ctx.idToSnowflake[id]
	i.ctx.snowflakeLock.Unlock()
	if !ok || snowflake == nil {
		// The snowflake took too long to respond with an answer, so its client
		// disappeared / the snowflake is no longer recognized by the Broker.
		success = false
	}

	b, err := messages.EncodeAnswerResponse(success)
	if err != nil {
		log.Printf("Error encoding answer: %s", err.Error())
		return messages.ErrInternal
	}
	*response = b

	if success {
		snowflake.answerChannel <- answer
	}

	return nil
}
