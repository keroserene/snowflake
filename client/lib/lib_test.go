package lib

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/pion/webrtc/v2"
	. "github.com/smartystreets/goconvey/convey"
)

type MockDataChannel struct {
	destination bytes.Buffer
	done        chan bool
}

func (m *MockDataChannel) Send(data []byte) error {
	m.destination.Write(data)
	m.done <- true
	return nil
}

func (*MockDataChannel) Close() error { return nil }

type MockResponse struct{}

func (m *MockResponse) Read(p []byte) (int, error) {
	p = []byte(`{"type":"answer","sdp":"fake"}`)
	return 0, nil
}
func (m *MockResponse) Close() error { return nil }

type MockTransport struct {
	statusOverride int
	body           []byte
}

// Just returns a response with fake SDP answer.
func (m *MockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s := ioutil.NopCloser(bytes.NewReader(m.body))
	r := &http.Response{
		StatusCode: m.statusOverride,
		Body:       s,
	}
	return r, nil
}

type FakeDialer struct{}

func (w FakeDialer) Catch() (Snowflake, error) {
	fmt.Println("Caught a dummy snowflake.")
	return &WebRTCPeer{}, nil
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

type FakePeers struct{ toRelease *WebRTCPeer }

func (f FakePeers) Collect() (Snowflake, error) { return &WebRTCPeer{}, nil }
func (f FakePeers) Pop() Snowflake              { return nil }
func (f FakePeers) Melted() <-chan struct{}     { return nil }

const sampleSDP = `"v=0\r\no=- 4358805017720277108 2 IN IP4 8.8.8.8\r\ns=-\r\nt=0 0\r\na=group:BUNDLE data\r\na=msid-semantic: WMS\r\nm=application 56688 DTLS/SCTP 5000\r\nc=IN IP4 8.8.8.8\r\na=candidate:3769337065 1 udp 2122260223 8.8.8.8 56688 typ host generation 0 network-id 1 network-cost 50\r\na=candidate:2921887769 1 tcp 1518280447 8.8.8.8 35441 typ host tcptype passive generation 0 network-id 1 network-cost 50\r\na=ice-ufrag:aMAZ\r\na=ice-pwd:jcHb08Jjgrazp2dzjdrvPPvV\r\na=ice-options:trickle\r\na=fingerprint:sha-256 C8:88:EE:B9:E7:02:2E:21:37:ED:7A:D1:EB:2B:A3:15:A2:3B:5B:1C:3D:D4:D5:1F:06:CF:52:40:03:F8:DD:66\r\na=setup:actpass\r\na=mid:data\r\na=sctpmap:5000 webrtc-datachannel 1024\r\n"`

const sampleAnswer = `{"type":"answer","sdp":` + sampleSDP + `}`

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
			_, err := p.Collect()
			So(err, ShouldNotBeNil)
			So(p.Count(), ShouldEqual, 0)
			// Set the dialer so that collection is possible.
			p.Tongue = FakeDialer{}
			_, err = p.Collect()
			So(err, ShouldBeNil)
			So(p.Count(), ShouldEqual, 1)
			// S
			_, err = p.Collect()
		})

		Convey("Collection continues until capacity.", func() {
			c := 5
			p := NewPeers(c)
			p.Tongue = FakeDialer{}
			// Fill up to capacity.
			for i := 0; i < c; i++ {
				fmt.Println("Adding snowflake ", i)
				_, err := p.Collect()
				So(err, ShouldBeNil)
				So(p.Count(), ShouldEqual, i+1)
			}
			// But adding another gives an error.
			So(p.Count(), ShouldEqual, c)
			_, err := p.Collect()
			So(err, ShouldNotBeNil)
			So(p.Count(), ShouldEqual, c)

			// But popping and closing allows it to continue.
			s := p.Pop()
			s.Close()
			So(s, ShouldNotBeNil)
			So(p.Count(), ShouldEqual, c-1)

			_, err = p.Collect()
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
				p.activePeers.PushBack(&WebRTCPeer{})
			}
			So(p.Count(), ShouldEqual, cnt)
			p.End()
			<-p.Melted()
			So(p.Count(), ShouldEqual, 0)
		})

		Convey("Pop skips over closed peers.", func() {
			p := NewPeers(4)
			p.Tongue = FakeDialer{}
			wc1, _ := p.Collect()
			wc2, _ := p.Collect()
			wc3, _ := p.Collect()
			So(wc1, ShouldNotBeNil)
			So(wc2, ShouldNotBeNil)
			So(wc3, ShouldNotBeNil)
			wc1.Close()
			r := p.Pop()
			So(p.Count(), ShouldEqual, 2)
			So(r, ShouldEqual, wc2)
			wc4, _ := p.Collect()
			wc2.Close()
			wc3.Close()
			r = p.Pop()
			So(r, ShouldEqual, wc4)
		})

	})

	Convey("Snowflake", t, func() {

		SkipConvey("Handler Grants correctly", func() {
			socks := &FakeSocksConn{}
			snowflakes := &FakePeers{}

			So(socks.rejected, ShouldEqual, false)
			snowflakes.toRelease = nil
			Handler(socks, snowflakes)
			So(socks.rejected, ShouldEqual, true)
		})

		Convey("WebRTC Connection", func() {
			c := NewWebRTCPeer(nil, nil)
			So(c.buffer.Bytes(), ShouldEqual, nil)

			Convey("Can construct a WebRTCConn", func() {
				s := NewWebRTCPeer(nil, nil)
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

				c.config = &webrtc.Configuration{}
				c.pc, _ = webrtc.NewPeerConnection(*c.config)
				offer, _ := c.pc.CreateOffer(nil)
				err := c.pc.SetLocalDescription(offer)
				So(err, ShouldBeNil)

				c.offerChannel <- nil
				answer := deserializeSessionDescription(sampleAnswer)
				So(answer, ShouldNotBeNil)
				c.answerChannel <- answer
				err = c.exchangeSDP()
				So(err, ShouldBeNil)
			})

			Convey("Exchange SDP keeps trying on nil answer", func(ctx C) {
				var wg sync.WaitGroup
				wg.Add(1)

				c.offerChannel = make(chan *webrtc.SessionDescription, 1)
				c.answerChannel = make(chan *webrtc.SessionDescription, 1)
				c.config = &webrtc.Configuration{}
				c.pc, _ = webrtc.NewPeerConnection(*c.config)
				offer, _ := c.pc.CreateOffer(nil)
				c.pc.SetLocalDescription(offer)

				c.offerChannel <- nil
				c.answerChannel <- nil
				go func() {
					err := c.exchangeSDP()
					ctx.So(err, ShouldBeNil)
					wg.Done()
				}()
				answer := deserializeSessionDescription(sampleAnswer)
				c.answerChannel <- answer
				wg.Wait()
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
		SkipConvey("WebRTCDialer can Catch a snowflake.", func() {
			broker := &BrokerChannel{Host: "test"}
			d := NewWebRTCDialer(broker, nil)
			conn, err := d.Catch()
			So(conn, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Rendezvous", t, func() {
		transport := &MockTransport{
			http.StatusOK,
			[]byte(`{"type":"answer","sdp":"fake"}`),
		}
		fakeOffer := deserializeSessionDescription(`{"type":"offer","sdp":"test"}`)

		Convey("Construct BrokerChannel with no front domain", func() {
			b, err := NewBrokerChannel("test.broker", "", transport, false)
			So(b.url, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(b.url.Path, ShouldResemble, "test.broker")
			So(b.transport, ShouldNotBeNil)
		})

		Convey("Construct BrokerChannel *with* front domain", func() {
			b, err := NewBrokerChannel("test.broker", "front", transport, false)
			So(b.url, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(b.url.Path, ShouldResemble, "test.broker")
			So(b.url.Host, ShouldResemble, "front")
			So(b.transport, ShouldNotBeNil)
		})

		Convey("BrokerChannel.Negotiate responds with answer", func() {
			b, err := NewBrokerChannel("test.broker", "", transport, false)
			So(err, ShouldBeNil)
			answer, err := b.Negotiate(fakeOffer)
			So(err, ShouldBeNil)
			So(answer, ShouldNotBeNil)
			So(answer.SDP, ShouldResemble, "fake")
		})

		Convey("BrokerChannel.Negotiate fails with 503", func() {
			b, err := NewBrokerChannel("test.broker", "",
				&MockTransport{http.StatusServiceUnavailable, []byte("\n")},
				false)
			So(err, ShouldBeNil)
			answer, err := b.Negotiate(fakeOffer)
			So(err, ShouldNotBeNil)
			So(answer, ShouldBeNil)
			So(err.Error(), ShouldResemble, BrokerError503)
		})

		Convey("BrokerChannel.Negotiate fails with 400", func() {
			b, err := NewBrokerChannel("test.broker", "",
				&MockTransport{http.StatusBadRequest, []byte("\n")},
				false)
			So(err, ShouldBeNil)
			answer, err := b.Negotiate(fakeOffer)
			So(err, ShouldNotBeNil)
			So(answer, ShouldBeNil)
			So(err.Error(), ShouldResemble, BrokerError400)
		})

		Convey("BrokerChannel.Negotiate fails with large read", func() {
			b, err := NewBrokerChannel("test.broker", "",
				&MockTransport{http.StatusOK, make([]byte, 100001, 100001)},
				false)
			So(err, ShouldBeNil)
			answer, err := b.Negotiate(fakeOffer)
			So(err, ShouldNotBeNil)
			So(answer, ShouldBeNil)
			So(err.Error(), ShouldResemble, "unexpected EOF")
		})

		Convey("BrokerChannel.Negotiate fails with unexpected error", func() {
			b, err := NewBrokerChannel("test.broker", "",
				&MockTransport{123, []byte("")}, false)
			So(err, ShouldBeNil)
			answer, err := b.Negotiate(fakeOffer)
			So(err, ShouldNotBeNil)
			So(answer, ShouldBeNil)
			So(err.Error(), ShouldResemble, BrokerErrorUnexpected)
		})
	})

	Convey("Strip", t, func() {
		const offerStart = "v=0\r\no=- 4358805017720277108 2 IN IP4 8.8.8.8\r\ns=-\r\nt=0 0\r\na=group:BUNDLE data\r\na=msid-semantic: WMS\r\nm=application 56688 DTLS/SCTP 5000\r\nc=IN IP4 8.8.8.8\r\n"
		const goodCandidate = "a=candidate:3769337065 1 udp 2122260223 8.8.8.8 56688 typ host generation 0 network-id 1 network-cost 50\r\n"
		const offerEnd = "a=ice-ufrag:aMAZ\r\na=ice-pwd:jcHb08Jjgrazp2dzjdrvPPvV\r\na=ice-options:trickle\r\na=fingerprint:sha-256 C8:88:EE:B9:E7:02:2E:21:37:ED:7A:D1:EB:2B:A3:15:A2:3B:5B:1C:3D:D4:D5:1F:06:CF:52:40:03:F8:DD:66\r\na=setup:actpass\r\na=mid:data\r\na=sctpmap:5000 webrtc-datachannel 1024\r\n"

		offer := offerStart + goodCandidate +
			"a=candidate:3769337065 1 udp 2122260223 192.168.0.100 56688 typ host generation 0 network-id 1 network-cost 50\r\n" + // IsLocal IPv4
			"a=candidate:3769337065 1 udp 2122260223 fdf8:f53b:82e4::53 56688 typ host generation 0 network-id 1 network-cost 50\r\n" + // IsLocal IPv6
			"a=candidate:3769337065 1 udp 2122260223 0.0.0.0 56688 typ host generation 0 network-id 1 network-cost 50\r\n" + // IsUnspecified IPv4
			"a=candidate:3769337065 1 udp 2122260223 :: 56688 typ host generation 0 network-id 1 network-cost 50\r\n" + // IsUnspecified IPv6
			"a=candidate:3769337065 1 udp 2122260223 127.0.0.1 56688 typ host generation 0 network-id 1 network-cost 50\r\n" + // IsLoopback IPv4
			"a=candidate:3769337065 1 udp 2122260223 ::1 56688 typ host generation 0 network-id 1 network-cost 50\r\n" + // IsLoopback IPv6
			offerEnd

		So(stripLocalAddresses(offer), ShouldEqual, offerStart+goodCandidate+offerEnd)
	})

}
