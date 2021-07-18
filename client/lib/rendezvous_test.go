package lib

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"testing"

	"git.torproject.org/pluggable-transports/snowflake.git/common/messages"
	"git.torproject.org/pluggable-transports/snowflake.git/common/nat"
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
	}).EncodePollRequest()
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

		fakeEncPollReq := makeEncPollReq(`{"type":"offer","sdp":"test"}`)

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
			So(err.Error(), ShouldResemble, BrokerErrorUnexpected)
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
