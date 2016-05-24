package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"testing"

	// "git.torproject.org/pluggable-transports/goptlib.git"
	"github.com/keroserene/go-webrtc"
	. "github.com/smartystreets/goconvey/convey"
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

type FakeSocksConn struct {
	net.Conn
	rejected bool
}

func (f FakeSocksConn) Reject() error {
	f.rejected = true
	return nil
}
func (f FakeSocksConn) Grant(addr *net.TCPAddr) error {
	return nil
}

type FakeSnowflakeJar struct {
	toRelease *webRTCConn
}

func (f FakeSnowflakeJar) Release() *webRTCConn {
	return nil
}

func (f FakeSnowflakeJar) Collect() (*webRTCConn, error) {
	return nil, nil
}

func TestSnowflakeClient(t *testing.T) {

	Convey("WebRTC ConnectLoop", t, func() {

		Convey("WebRTC ConnectLoop continues until capacity of 1.\n", func() {
			snowflakes := NewSnowflakeJar(1)
			snowflakes.Tongue = FakeDialer{}

			go ConnectLoop(snowflakes)
			<-snowflakes.maxedChan

			So(snowflakes.Count(), ShouldEqual, 1)
			r := <-snowflakes.snowflakeChan
			So(r, ShouldNotBeNil)
			So(snowflakes.Count(), ShouldEqual, 0)
		})

		Convey("WebRTC ConnectLoop continues until capacity of 3.\n", func() {
			snowflakes := NewSnowflakeJar(3)
			snowflakes.Tongue = FakeDialer{}

			go ConnectLoop(snowflakes)
			<-snowflakes.maxedChan
			So(snowflakes.Count(), ShouldEqual, 3)
			<-snowflakes.snowflakeChan
			<-snowflakes.snowflakeChan
			<-snowflakes.snowflakeChan
			So(snowflakes.Count(), ShouldEqual, 0)
		})

		Convey("WebRTC ConnectLoop continues filling when Snowflakes disconnect.\n", func() {
			snowflakes := NewSnowflakeJar(3)
			snowflakes.Tongue = FakeDialer{}

			go ConnectLoop(snowflakes)
			<-snowflakes.maxedChan
			So(snowflakes.Count(), ShouldEqual, 3)

			r := <-snowflakes.snowflakeChan
			So(snowflakes.Count(), ShouldEqual, 2)
			r.Close()
			<-snowflakes.maxedChan
			So(snowflakes.Count(), ShouldEqual, 3)

			<-snowflakes.snowflakeChan
			<-snowflakes.snowflakeChan
			<-snowflakes.snowflakeChan
			So(snowflakes.Count(), ShouldEqual, 0)
		})
	})

	Convey("Snowflake", t, func() {

		SkipConvey("Handler Grants correctly", func() {
			socks := &FakeSocksConn{}
			snowflakes := &FakeSnowflakeJar{}

			So(socks.rejected, ShouldEqual, false)
			snowflakes.toRelease = nil
			handler(socks, snowflakes)
			So(socks.rejected, ShouldEqual, true)

		})

		Convey("WebRTC Connection", func() {
			c := NewWebRTCConnection(nil, nil)
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
