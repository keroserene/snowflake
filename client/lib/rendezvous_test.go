package snowflake_client

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"testing"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/amp"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/messages"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/nat"
	. "github.com/smartystreets/goconvey/convey"
)

// mockTransport's RoundTrip method returns a response with a fake status and
// body.
type mockTransport struct {
	statusCode int
	body       []byte
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		Status:     fmt.Sprintf("%d %s", t.statusCode, http.StatusText(t.statusCode)),
		StatusCode: t.statusCode,
		Body:       ioutil.NopCloser(bytes.NewReader(t.body)),
	}, nil
}

// errorTransport's RoundTrip method returns an error.
type errorTransport struct {
	err error
}

func (t errorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, t.err
}

// makeEncPollReq returns an encoded client poll request containing a given
// offer.
func makeEncPollReq(offer string) []byte {
	encPollReq, err := (&messages.ClientPollRequest{
		Offer: offer,
		NAT:   nat.NATUnknown,
	}).EncodeClientPollRequest()
	if err != nil {
		panic(err)
	}
	return encPollReq
}

// makeEncPollResp returns an encoded client poll response with given answer and
// error strings.
func makeEncPollResp(answer, errorStr string) []byte {
	encPollResp, err := (&messages.ClientPollResponse{
		Answer: answer,
		Error:  errorStr,
	}).EncodePollResponse()
	if err != nil {
		panic(err)
	}
	return encPollResp
}

var fakeEncPollReq = makeEncPollReq(`{"type":"offer","sdp":"test"}`)

func TestHTTPRendezvous(t *testing.T) {
	Convey("HTTP rendezvous", t, func() {
		Convey("Construct httpRendezvous with no front domain", func() {
			transport := &mockTransport{http.StatusOK, []byte{}}
			rend, err := newHTTPRendezvous("http://test.broker", "", transport)
			So(err, ShouldBeNil)
			So(rend.brokerURL, ShouldNotBeNil)
			So(rend.brokerURL.Host, ShouldResemble, "test.broker")
			So(rend.front, ShouldResemble, "")
			So(rend.transport, ShouldEqual, transport)
		})

		Convey("Construct httpRendezvous *with* front domain", func() {
			transport := &mockTransport{http.StatusOK, []byte{}}
			rend, err := newHTTPRendezvous("http://test.broker", "front", transport)
			So(err, ShouldBeNil)
			So(rend.brokerURL, ShouldNotBeNil)
			So(rend.brokerURL.Host, ShouldResemble, "test.broker")
			So(rend.front, ShouldResemble, "front")
			So(rend.transport, ShouldEqual, transport)
		})

		Convey("httpRendezvous.Exchange responds with answer", func() {
			fakeEncPollResp := makeEncPollResp(
				`{"answer": "{\"type\":\"answer\",\"sdp\":\"fake\"}" }`,
				"",
			)
			rend, err := newHTTPRendezvous("http://test.broker", "",
				&mockTransport{http.StatusOK, fakeEncPollResp})
			So(err, ShouldBeNil)
			answer, err := rend.Exchange(fakeEncPollReq)
			So(err, ShouldBeNil)
			So(answer, ShouldResemble, fakeEncPollResp)
		})

		Convey("httpRendezvous.Exchange responds with no answer", func() {
			fakeEncPollResp := makeEncPollResp(
				"",
				`{"error": "no snowflake proxies currently available"}`,
			)
			rend, err := newHTTPRendezvous("http://test.broker", "",
				&mockTransport{http.StatusOK, fakeEncPollResp})
			So(err, ShouldBeNil)
			answer, err := rend.Exchange(fakeEncPollReq)
			So(err, ShouldBeNil)
			So(answer, ShouldResemble, fakeEncPollResp)
		})

		Convey("httpRendezvous.Exchange fails with unexpected HTTP status code", func() {
			rend, err := newHTTPRendezvous("http://test.broker", "",
				&mockTransport{http.StatusInternalServerError, []byte{}})
			So(err, ShouldBeNil)
			answer, err := rend.Exchange(fakeEncPollReq)
			So(err, ShouldNotBeNil)
			So(answer, ShouldBeNil)
			So(err.Error(), ShouldResemble, brokerErrorUnexpected)
		})

		Convey("httpRendezvous.Exchange fails with error", func() {
			transportErr := errors.New("error")
			rend, err := newHTTPRendezvous("http://test.broker", "",
				&errorTransport{err: transportErr})
			So(err, ShouldBeNil)
			answer, err := rend.Exchange(fakeEncPollReq)
			So(err, ShouldEqual, transportErr)
			So(answer, ShouldBeNil)
		})

		Convey("httpRendezvous.Exchange fails with large read", func() {
			rend, err := newHTTPRendezvous("http://test.broker", "",
				&mockTransport{http.StatusOK, make([]byte, readLimit+1)})
			So(err, ShouldBeNil)
			_, err = rend.Exchange(fakeEncPollReq)
			So(err, ShouldEqual, io.ErrUnexpectedEOF)
		})
	})
}

func ampArmorEncode(p []byte) []byte {
	var buf bytes.Buffer
	enc, err := amp.NewArmorEncoder(&buf)
	if err != nil {
		panic(err)
	}
	_, err = enc.Write(p)
	if err != nil {
		panic(err)
	}
	err = enc.Close()
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func TestAMPCacheRendezvous(t *testing.T) {
	Convey("AMP cache rendezvous", t, func() {
		Convey("Construct ampCacheRendezvous with no cache and no front domain", func() {
			transport := &mockTransport{http.StatusOK, []byte{}}
			rend, err := newAMPCacheRendezvous("http://test.broker", "", "", transport)
			So(err, ShouldBeNil)
			So(rend.brokerURL, ShouldNotBeNil)
			So(rend.brokerURL.String(), ShouldResemble, "http://test.broker")
			So(rend.cacheURL, ShouldBeNil)
			So(rend.front, ShouldResemble, "")
			So(rend.transport, ShouldEqual, transport)
		})

		Convey("Construct ampCacheRendezvous with cache and no front domain", func() {
			transport := &mockTransport{http.StatusOK, []byte{}}
			rend, err := newAMPCacheRendezvous("http://test.broker", "https://amp.cache/", "", transport)
			So(err, ShouldBeNil)
			So(rend.brokerURL, ShouldNotBeNil)
			So(rend.brokerURL.String(), ShouldResemble, "http://test.broker")
			So(rend.cacheURL, ShouldNotBeNil)
			So(rend.cacheURL.String(), ShouldResemble, "https://amp.cache/")
			So(rend.front, ShouldResemble, "")
			So(rend.transport, ShouldEqual, transport)
		})

		Convey("Construct ampCacheRendezvous with no cache and front domain", func() {
			transport := &mockTransport{http.StatusOK, []byte{}}
			rend, err := newAMPCacheRendezvous("http://test.broker", "", "front", transport)
			So(err, ShouldBeNil)
			So(rend.brokerURL, ShouldNotBeNil)
			So(rend.brokerURL.String(), ShouldResemble, "http://test.broker")
			So(rend.cacheURL, ShouldBeNil)
			So(rend.front, ShouldResemble, "front")
			So(rend.transport, ShouldEqual, transport)
		})

		Convey("Construct ampCacheRendezvous with cache and front domain", func() {
			transport := &mockTransport{http.StatusOK, []byte{}}
			rend, err := newAMPCacheRendezvous("http://test.broker", "https://amp.cache/", "front", transport)
			So(err, ShouldBeNil)
			So(rend.brokerURL, ShouldNotBeNil)
			So(rend.brokerURL.String(), ShouldResemble, "http://test.broker")
			So(rend.cacheURL, ShouldNotBeNil)
			So(rend.cacheURL.String(), ShouldResemble, "https://amp.cache/")
			So(rend.front, ShouldResemble, "front")
			So(rend.transport, ShouldEqual, transport)
		})

		Convey("ampCacheRendezvous.Exchange responds with answer", func() {
			fakeEncPollResp := makeEncPollResp(
				`{"answer": "{\"type\":\"answer\",\"sdp\":\"fake\"}" }`,
				"",
			)
			rend, err := newAMPCacheRendezvous("http://test.broker", "", "",
				&mockTransport{http.StatusOK, ampArmorEncode(fakeEncPollResp)})
			So(err, ShouldBeNil)
			answer, err := rend.Exchange(fakeEncPollReq)
			So(err, ShouldBeNil)
			So(answer, ShouldResemble, fakeEncPollResp)
		})

		Convey("ampCacheRendezvous.Exchange responds with no answer", func() {
			fakeEncPollResp := makeEncPollResp(
				"",
				`{"error": "no snowflake proxies currently available"}`,
			)
			rend, err := newAMPCacheRendezvous("http://test.broker", "", "",
				&mockTransport{http.StatusOK, ampArmorEncode(fakeEncPollResp)})
			So(err, ShouldBeNil)
			answer, err := rend.Exchange(fakeEncPollReq)
			So(err, ShouldBeNil)
			So(answer, ShouldResemble, fakeEncPollResp)
		})

		Convey("ampCacheRendezvous.Exchange fails with unexpected HTTP status code", func() {
			rend, err := newAMPCacheRendezvous("http://test.broker", "", "",
				&mockTransport{http.StatusInternalServerError, []byte{}})
			So(err, ShouldBeNil)
			answer, err := rend.Exchange(fakeEncPollReq)
			So(err, ShouldNotBeNil)
			So(answer, ShouldBeNil)
			So(err.Error(), ShouldResemble, brokerErrorUnexpected)
		})

		Convey("ampCacheRendezvous.Exchange fails with error", func() {
			transportErr := errors.New("error")
			rend, err := newAMPCacheRendezvous("http://test.broker", "", "",
				&errorTransport{err: transportErr})
			So(err, ShouldBeNil)
			answer, err := rend.Exchange(fakeEncPollReq)
			So(err, ShouldEqual, transportErr)
			So(answer, ShouldBeNil)
		})

		Convey("ampCacheRendezvous.Exchange fails with large read", func() {
			// readLimit should apply to the raw HTTP body, not the
			// encoded bytes. Encode readLimit bytes—the encoded
			// size will be larger—and try to read the body. It
			// should fail.
			rend, err := newAMPCacheRendezvous("http://test.broker", "", "",
				&mockTransport{http.StatusOK, ampArmorEncode(make([]byte, readLimit))})
			So(err, ShouldBeNil)
			_, err = rend.Exchange(fakeEncPollReq)
			// We may get io.ErrUnexpectedEOF here, or something
			// like "missing </pre> tag".
			So(err, ShouldNotBeNil)
		})
	})
}
