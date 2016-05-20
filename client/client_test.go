package main

import (
	"bytes"
	"fmt"
	"github.com/keroserene/go-webrtc"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
)

type MockDataChannel struct {
	destination bytes.Buffer
	done        chan bool
}

func (m *MockDataChannel) Send(data []byte) {
	m.destination.Write(data)
	m.done <- true
}

func (*MockDataChannel) Close() error {
	return nil
}

type MockResponse struct{}

func (m *MockResponse) Read(p []byte) (int, error) {
	p = []byte(`{"type":"answer","sdp":"fake"}`)
	return 0, nil
}
func (m *MockResponse) Close() error {
	return nil
}

type MockTransport struct {
	statusOverride int
}

// Just returns a response with fake SDP answer.
func (m *MockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s := ioutil.NopCloser(strings.NewReader(`{"type":"answer","sdp":"fake"}`))
	r := &http.Response{
		StatusCode: m.statusOverride,
		Body:       s,
	}
	return r, nil
}

type FakeDialer struct{}

func (w FakeDialer) Catch() (*webRTCConn, error) {
	fmt.Println("Caught a dummy snowflake.")
	return &webRTCConn{}, nil
}

func TestSnowflakeClient(t *testing.T) {
	Convey("Snowflake", t, func() {

		Convey("Peers", func() {

			Convey("WebRTC ConnectLoop continues until capacity of 1.\n", func() {
				peers := NewPeers(1)
				peers.Tongue = FakeDialer{}

				go ConnectLoop(peers)
				<-peers.maxedChan

				So(peers.Count(), ShouldEqual, 1)
				r := <-peers.snowflakeChan
				So(r, ShouldNotBeNil)
				So(peers.Count(), ShouldEqual, 0)
			})

			Convey("WebRTC ConnectLoop continues until capacity of 3.\n", func() {
				peers := NewPeers(3)
				peers.Tongue = FakeDialer{}

				go ConnectLoop(peers)
				<-peers.maxedChan
				So(peers.Count(), ShouldEqual, 3)
				<-peers.snowflakeChan
				<-peers.snowflakeChan
				<-peers.snowflakeChan
				So(peers.Count(), ShouldEqual, 0)
			})

			Convey("WebRTC ConnectLoop continues filling when Snowflakes disconnect.\n", func() {
				peers := NewPeers(3)
				peers.Tongue = FakeDialer{}

				go ConnectLoop(peers)
				<-peers.maxedChan
				So(peers.Count(), ShouldEqual, 3)

				r := <-peers.snowflakeChan
				So(peers.Count(), ShouldEqual, 2)
				r.Close()
				<-peers.maxedChan
				So(peers.Count(), ShouldEqual, 3)

				<-peers.snowflakeChan
				<-peers.snowflakeChan
				<-peers.snowflakeChan
				So(peers.Count(), ShouldEqual, 0)
			})
		})

		Convey("WebRTC Connection", func() {
			c := new(webRTCConn)
			c.BytesInfo = &BytesInfo{
				inboundChan: make(chan int), outboundChan: make(chan int),
				inbound: 0, outbound: 0, inEvents: 0, outEvents: 0,
			}
			So(c.buffer.Bytes(), ShouldEqual, nil)

			Convey("Can construct a WebRTCConn", func() {
				s := NewWebRTCConnection(nil, nil)
				So(s, ShouldNotBeNil)
				So(s.index, ShouldEqual, 0)
				So(s.offerChannel, ShouldNotBeNil)
				So(s.answerChannel, ShouldNotBeNil)
				s.Close()
			})

			Convey("Write buffers when datachannel is nil", func() {
				c.Write([]byte("test"))
				c.snowflake = nil
				So(c.buffer.Bytes(), ShouldResemble, []byte("test"))
			})

			Convey("Write sends to datachannel when not nil", func() {
				mock := new(MockDataChannel)
				c.snowflake = mock
				mock.done = make(chan bool, 1)
				c.Write([]byte("test"))
				<-mock.done
				So(c.buffer.Bytes(), ShouldEqual, nil)
				So(mock.destination.Bytes(), ShouldResemble, []byte("test"))
			})

			Convey("Exchange SDP sets remote description", func() {
				c.offerChannel = make(chan *webrtc.SessionDescription, 1)
				c.answerChannel = make(chan *webrtc.SessionDescription, 1)

				c.config = webrtc.NewConfiguration()
				c.preparePeerConnection()

				c.offerChannel <- nil
				answer := webrtc.DeserializeSessionDescription(
					`{"type":"answer","sdp":""}`)
				c.answerChannel <- answer
				c.exchangeSDP()
			})

			SkipConvey("Exchange SDP fails on nil answer", func() {
				c.reset = make(chan struct{})
				c.offerChannel = make(chan *webrtc.SessionDescription, 1)
				c.answerChannel = make(chan *webrtc.SessionDescription, 1)
				c.offerChannel <- nil
				c.answerChannel <- nil
				c.exchangeSDP()
				<-c.reset
			})

		})
	})

	Convey("Rendezvous", t, func() {
		webrtc.SetLoggingVerbosity(0)
		transport := &MockTransport{http.StatusOK}
		fakeOffer := webrtc.DeserializeSessionDescription("test")

		Convey("Construct BrokerChannel with no front domain", func() {
			b := NewBrokerChannel("test.broker", "", transport)
			So(b.url, ShouldNotBeNil)
			So(b.url.Path, ShouldResemble, "test.broker")
			So(b.transport, ShouldNotBeNil)
		})

		Convey("Construct BrokerChannel *with* front domain", func() {
			b := NewBrokerChannel("test.broker", "front", transport)
			So(b.url, ShouldNotBeNil)
			So(b.url.Path, ShouldResemble, "test.broker")
			So(b.url.Host, ShouldResemble, "front")
			So(b.transport, ShouldNotBeNil)
		})

		Convey("BrokerChannel.Negotiate responds with answer", func() {
			b := NewBrokerChannel("test.broker", "", transport)
			answer, err := b.Negotiate(fakeOffer)
			So(err, ShouldBeNil)
			So(answer, ShouldNotBeNil)
			So(answer.Sdp, ShouldResemble, "fake")
		})

		Convey("BrokerChannel.Negotiate fails with 503", func() {
			b := NewBrokerChannel("test.broker", "",
				&MockTransport{http.StatusServiceUnavailable})
			answer, err := b.Negotiate(fakeOffer)
			So(err, ShouldNotBeNil)
			So(answer, ShouldBeNil)
			So(err.Error(), ShouldResemble, BrokerError503)
		})

		Convey("BrokerChannel.Negotiate fails with 400", func() {
			b := NewBrokerChannel("test.broker", "",
				&MockTransport{http.StatusBadRequest})
			answer, err := b.Negotiate(fakeOffer)
			So(err, ShouldNotBeNil)
			So(answer, ShouldBeNil)
			So(err.Error(), ShouldResemble, BrokerError400)
		})

		Convey("BrokerChannel.Negotiate fails with unexpected error", func() {
			b := NewBrokerChannel("test.broker", "",
				&MockTransport{123})
			answer, err := b.Negotiate(fakeOffer)
			So(err, ShouldNotBeNil)
			So(answer, ShouldBeNil)
			So(err.Error(), ShouldResemble, BrokerErrorUnexpected)
		})
	})
}
