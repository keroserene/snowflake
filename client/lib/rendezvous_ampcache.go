package lib

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"net/url"
)

// ampCacheRendezvous is a rendezvousMethod that communicates with the
// .../amp/client route of the broker, optionally over an AMP cache proxy, and
// with optional domain fronting.
type ampCacheRendezvous struct {
	brokerURL *url.URL
	cacheURL  *url.URL          // Optional AMP cache URL.
	front     string            // Optional front domain to replace url.Host in requests.
	transport http.RoundTripper // Used to make all requests.
}

// newAMPCacheRendezvous creates a new ampCacheRendezvous that contacts the
// broker at the given URL, optionally proxying through an AMP cache, and with
// an optional front domain. transport is the http.RoundTripper used to make all
// requests.
func newAMPCacheRendezvous(broker, cache, front string, transport http.RoundTripper) (*ampCacheRendezvous, error) {
	brokerURL, err := url.Parse(broker)
	if err != nil {
		return nil, err
	}
	var cacheURL *url.URL
	if cache != "" {
		var err error
		cacheURL, err = url.Parse(cache)
		if err != nil {
			return nil, err
		}
	}
	return &ampCacheRendezvous{
		brokerURL: brokerURL,
		cacheURL:  cacheURL,
		front:     front,
		transport: transport,
	}, nil
}

func (r *ampCacheRendezvous) Exchange(encPollReq []byte) ([]byte, error) {
	log.Println("Negotiating via AMP cache rendezvous...")
	log.Println("Broker URL:", r.brokerURL)
	log.Println("AMP cache URL:", r.cacheURL)
	log.Println("Front domain:", r.front)

	// Suffix the path with the broker's client registration handler.
	reqURL := r.brokerURL.ResolveReference(&url.URL{Path: "client"})
	req, err := http.NewRequest("POST", reqURL.String(), bytes.NewReader(encPollReq))
	if err != nil {
		return nil, err
	}

	if r.front != "" {
		// Do domain fronting. Replace the domain in the URL's with the
		// front, and store the original domain the HTTP Host header.
		req.Host = req.URL.Host
		req.URL.Host = r.front
	}

	resp, err := r.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	log.Printf("AMP cache rendezvous response: %s", resp.Status)
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(BrokerErrorUnexpected)
	}

	return limitedRead(resp.Body, readLimit)
}
