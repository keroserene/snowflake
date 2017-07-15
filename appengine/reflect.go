// A web app for Google App Engine that proxies HTTP requests and responses to a
// Tor relay running meek-server.
package reflect

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"appengine"
	"appengine/urlfetch"
)

const (
	forwardURL = "https://meek.bamsoftware.com/"
	// A timeout of 0 means to use the App Engine default (5 seconds).
	urlFetchTimeout = 20 * time.Second
)

var context appengine.Context

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

// We reflect only a whitelisted set of header fields. In requests, the full
// list includes things like User-Agent and X-Appengine-Country that the Tor
// bridge doesn't need to know. In responses, there may be things like
// Transfer-Encoding that interfere with App Engine's own hop-by-hop headers.
var reflectedHeaderFields = []string{
	"Content-Type",
	"X-Session-Id",
}

// Get the original client IP address as a string. When using the standard
// net/http server, Request.RemoteAddr is a "host:port" string; however App
// Engine seems to use just "host". We check for both to be safe.
func getClientAddr(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
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
	// Set the original client IP address in a Meek-IP header. We would use
	// X-Forwarded-For, but App Engine prohibits setting that header:
	// https://cloud.google.com/appengine/docs/standard/go/outbound-requests#request_headers
	// We could use Forwarded from RFC 7239, but other CDNs already use
	// X-Forwarded-For and this way we only need one parser.
	c.Header.Add("Meek-IP", getClientAddr(r))
	return c, nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	context = appengine.NewContext(r)
	fr, err := copyRequest(r)
	if err != nil {
		context.Errorf("copyRequest: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Use urlfetch.Transport directly instead of urlfetch.Client because we
	// want only a single HTTP transaction, not following redirects.
	transport := urlfetch.Transport{
		Context: context,
		// Despite the name, Transport.Deadline is really a timeout and
		// not an absolute deadline as used in the net package. In
		// other words it is a time.Duration, not a time.Time.
		Deadline: urlFetchTimeout,
	}
	resp, err := transport.RoundTrip(fr)
	if err != nil {
		context.Errorf("RoundTrip: %s", err)
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
		context.Errorf("io.Copy after %d bytes: %s", n, err)
	}
}

func init() {
	http.HandleFunc("/", handler)
}
