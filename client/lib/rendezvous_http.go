package snowflake_client

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
)

// httpRendezvous is a RendezvousMethod that communicates with the .../client
// route of the broker over HTTP or HTTPS, with optional domain fronting.
type httpRendezvous struct {
	brokerURL *url.URL
	front     string            // Optional front domain to replace url.Host in requests.
	transport http.RoundTripper // Used to make all requests.
}

// newHTTPRendezvous creates a new httpRendezvous that contacts the broker at
// the given URL, with an optional front domain. transport is the
// http.RoundTripper used to make all requests.
func newHTTPRendezvous(broker, front string, transport http.RoundTripper) (*httpRendezvous, error) {
	brokerURL, err := url.Parse(broker)
	if err != nil {
		return nil, err
	}
	return &httpRendezvous{
		brokerURL: brokerURL,
		front:     front,
		transport: transport,
	}, nil
}

func (r *httpRendezvous) Exchange(encPollReq []byte) ([]byte, error) {
	log.Println("Negotiating via HTTP rendezvous...")
	log.Println("Target URL: ", r.brokerURL.Host)
	log.Println("Front URL:  ", r.front)

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

	log.Printf("HTTP rendezvous response: %s", resp.Status)
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(brokerErrorUnexpected)
	}

	return limitedRead(resp.Body, readLimit)
}

func limitedRead(r io.Reader, limit int64) ([]byte, error) {
	p, err := ioutil.ReadAll(&io.LimitedReader{R: r, N: limit + 1})
	if err != nil {
		return p, err
	} else if int64(len(p)) == limit+1 {
		return p[0:limit], io.ErrUnexpectedEOF
	}
	return p, err
}
