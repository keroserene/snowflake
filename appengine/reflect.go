// A web app for Google App Engine that proxies HTTP requests and responses to
// the Snowflake broker.
package main

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/urlfetch"
)

const (
	forwardURL = "https://snowflake-broker.bamsoftware.com/"
	// A timeout of 0 means to use the App Engine default (5 seconds).
	urlFetchTimeout = 20 * time.Second
)

var ctx context.Context

// Join two URL paths.
func pathJoin(a, b string) string {
	if len(a) > 0 && a[len(a)-1] == '/' {
		a = a[:len(a)-1]
	}
	if len(b) == 0 || b[0] != '/' {
		b = "/" + b
	}
	return a + b
}

// We reflect only a whitelisted set of header fields. Otherwise, we may copy
// headers like Transfer-Encoding that interfere with App Engine's own
// hop-by-hop headers.
var reflectedHeaderFields = []string{
	"Content-Type",
	"X-Session-Id",
}

// Make a copy of r, with the URL being changed to be relative to forwardURL,
// and including only the headers in reflectedHeaderFields.
func copyRequest(r *http.Request) (*http.Request, error) {
	u, err := url.Parse(forwardURL)
	if err != nil {
		return nil, err
	}
	// Append the requested path to the path in forwardURL, so that
	// forwardURL can be something like "https://example.com/reflect".
	u.Path = pathJoin(u.Path, r.URL.Path)
	c, err := http.NewRequest(r.Method, u.String(), r.Body)
	if err != nil {
		return nil, err
	}
	for _, key := range reflectedHeaderFields {
		values, ok := r.Header[key]
		if ok {
			for _, value := range values {
				c.Header.Add(key, value)
			}
		}
	}
	return c, nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	ctx = appengine.NewContext(r)
	fr, err := copyRequest(r)
	if err != nil {
		log.Errorf(ctx, "copyRequest: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if urlFetchTimeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, urlFetchTimeout)
		defer cancel()
	}
	// Use urlfetch.Transport directly instead of urlfetch.Client because we
	// want only a single HTTP transaction, not following redirects.
	transport := urlfetch.Transport{
		Context: ctx,
	}
	resp, err := transport.RoundTrip(fr)
	if err != nil {
		log.Errorf(ctx, "RoundTrip: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	for _, key := range reflectedHeaderFields {
		values, ok := resp.Header[key]
		if ok {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
	}
	w.WriteHeader(resp.StatusCode)
	n, err := io.Copy(w, resp.Body)
	if err != nil {
		log.Errorf(ctx, "io.Copy after %d bytes: %s", n, err)
	}
}

func init() {
	http.HandleFunc("/", handler)
}
