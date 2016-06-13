package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"testing"

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

func (*MockDataChannel) Close() error { return nil }

type MockResponse struct{}

func (m *MockResponse) Read(p []byte) (int, error) {
	p = []byte(`{"type":"answer","sdp":"fake"}`)
	return 0, nil
}
func (m *MockResponse) Close() error { return nil }

type MockTransport struct{ statusOverride int }

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

func (w FakeDialer) Catch() (Snowflake, error) {
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
func (f FakeSocksConn) Grant(addr *net.TCPAddr) error { return nil }

type FakePeers struct{ toRelease *webRTCConn }

func (f FakePeers) Collect() error          { return nil }
func (f FakePeers) Pop() Snowflake          { return nil }
func (f FakePeers) Melted() <-chan struct{} { return nil }

func TestSnowflakeClient(t *testing.T) {

	Convey("Peers", t, func() {
		Convey("Can construct", func() {
			p := NewPeers(1)
			So(p.capacity, ShouldEqual, 1)
			So(p.snowflakeChan, ShouldNotBeNil)
			So(cap(p.snowflakeChan), ShouldEqual, 1)
		})

		Convey("Collecting a Snowflake requires a Tongue.", func() {
			p := NewPeers(1)
			err := p.Collect()
			So(err, ShouldNotBeNil)
			So(p.Count(), ShouldEqual, 0)
			// Set the dialer so that collection is possible.
			p.Tongue = FakeDialer{}
			err = p.Collect()
			So(err, ShouldBeNil)
			So(p.Count(), ShouldEqual, 1)
			// S
			err = p.Collect()
		})

		Convey("Collection continues until capacity.", func() {
			c := 5
			p := NewPeers(c)
			p.Tongue = FakeDialer{}
			// Fill up to capacity.
			for i := 0; i < c; i++ {
				fmt.Println("Adding snowflake ", i)
				err := p.Collect()
				So(err, ShouldBeNil)
				So(p.Count(), ShouldEqual, i+1)
			}
			// But adding another gives an error.
			So(p.Count(), ShouldEqual, c)
			err := p.Collect()
			So(err, ShouldNotBeNil)
			So(p.Count(), ShouldEqual, c)

			// But popping and closing allows it to continue.
			s := p.Pop()
			s.Close()
			So(s, ShouldNotBeNil)
			So(p.Count(), ShouldEqual, c-1)

			err = p.Collect()
			So(err, ShouldBeNil)
			So(p.Count(), ShouldEqual, c)
		})

		Convey("Count correctly purges peers marked for deletion.", func() {
			p := NewPeers(4)
			p.Tongue = FakeDialer{}
			p.Collect()
			p.Collect()
			p.Collect()
			p.Collect()
			So(p.Count(), ShouldEqual, 4)
			s := p.Pop()
			s.Close()
			So(p.Count(), ShouldEqual, 3)
			s = p.Pop()
			s.Close()
			So(p.Count(), ShouldEqual, 2)
		})

		Convey("End Closes all peers.", func() {
			cnt := 5
			p := NewPeers(cnt)
			for i := 0; i < cnt; i++ {
				p.activePeers.PushBack(&webRTCConn{})
			}
			So(p.Count(), ShouldEqual, cnt)
			p.End()
			<-p.Melted()
			So(p.Count(), ShouldEqual, 0)
		})

	})

	Convey("Snowflake", t, func() {

		SkipConvey("Handler Grants correctly", func() {
			socks := &FakeSocksConn{}
			snowflakes := &FakePeers{}

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
				So(s.offerChannel, ShouldNotBeNil)
				So(s.answerChannel, ShouldNotBeNil)
				s.Close()
			})

			Convey("Write buffers when datachannel is nil", func() {
				c.Write([]byte("test"))
				c.transport = nil
				So(c.buffer.Bytes(), ShouldResemble, []byte("test"))
			})

			Convey("Write sends to datachannel when not nil", func() {
				mock := new(MockDataChannel)
				c.transport = mock
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

	Convey("Dialers", t, func() {
		Convey("Can construct WebRTCDialer.", func() {
			broker := &BrokerChannel{Host: "test"}
			d := NewWebRTCDialer(broker, nil)
			So(d, ShouldNotBeNil)
			So(d.BrokerChannel, ShouldNotBeNil)
			So(d.BrokerChannel.Host, ShouldEqual, "test")
		})
		Convey("WebRTCDialer cannot Catch a snowflake with nil broker.", func() {
			d := NewWebRTCDialer(nil, nil)
			conn, err := d.Catch()
			So(conn, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
		SkipConvey("WebRTCDialer can Catch a snowflake.", func() {
			broker := &BrokerChannel{Host: "test"}
			d := NewWebRTCDialer(broker, nil)
			conn, err := d.Catch()
			So(conn, ShouldBeNil)
			So(err, ShouldNotBeNil)
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
