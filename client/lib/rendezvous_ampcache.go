package snowflake_client

import (
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/amp"
)

// ampCacheRendezvous is a RendezvousMethod that communicates with the
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

	// We cannot POST a body through an AMP cache, so instead we GET and
	// encode the client poll request message into the URL.
	reqURL := r.brokerURL.ResolveReference(&url.URL{
		Path: "amp/client/" + amp.EncodePath(encPollReq),
	})

	if r.cacheURL != nil {
		// Rewrite reqURL to its AMP cache version.
		var err error
		reqURL, err = amp.CacheURL(reqURL, r.cacheURL, "c")
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest("GET", reqURL.String(), nil)
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
		// A non-200 status indicates an error:
		// * If the broker returns a page with invalid AMP, then the AMP
		//   cache returns a redirect that would bypass the cache.
		// * If the broker returns a 5xx status, the AMP cache
		//   translates it to a 404.
		// https://amp.dev/documentation/guides-and-tutorials/learn/amp-caches-and-cors/amp-cache-urls/#redirect-%26-error-handling
		return nil, errors.New(brokerErrorUnexpected)
	}
	if _, err := resp.Location(); err == nil {
		// The Google AMP Cache may return a "silent redirect" with
		// status 200, a Location header set, and a JavaScript redirect
		// in the body. The redirect points directly at the origin
		// server for the request (bypassing the AMP cache). We do not
		// follow redirects nor execute JavaScript, but in any case we
		// cannot extract information from this response and can only
		// treat it as an error.
		return nil, errors.New(brokerErrorUnexpected)
	}

	lr := io.LimitReader(resp.Body, readLimit+1)
	dec, err := amp.NewArmorDecoder(lr)
	if err != nil {
		return nil, err
	}
	encPollResp, err := ioutil.ReadAll(dec)
	if err != nil {
		return nil, err
	}
	if lr.(*io.LimitedReader).N == 0 {
		// We hit readLimit while decoding AMP armor, that's an error.
		return nil, io.ErrUnexpectedEOF
	}

	return encPollResp, err
}
