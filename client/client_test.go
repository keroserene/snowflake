package main

import (
	"bytes"
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
}

// Just returns a response with fake SDP answer.
func (m *MockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s := ioutil.NopCloser(strings.NewReader(`{"type":"answer","sdp":"fake"}`))
	r := &http.Response{
		StatusCode: http.StatusOK,
		Body:       s,
	}
	return r, nil
}

func TestConnect(t *testing.T) {
	Convey("Snowflake", t, func() {

		Convey("WebRTC Connection", func() {
			c := new(webRTCConn)
			c.BytesInfo = &BytesInfo{
				inboundChan: make(chan int), outboundChan: make(chan int),
				inbound: 0, outbound: 0, inEvents: 0, outEvents: 0,
			}
			So(c.buffer.Bytes(), ShouldEqual, nil)

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

				// offer := webrtc.DeserializeSessionDescription(
				// `{"type":"offer","sdp":"test offer"}`)
				// c.pc.SetLocalDescription(offer)
				c.offerChannel <- nil
				answer := webrtc.DeserializeSessionDescription(
					`{"type":"answer","sdp":""}`)
				c.answerChannel <- answer
				c.exchangeSDP()
				// So(c.pc.RemoteDescription(), ShouldEqual, answer)
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

			Convey("Connect Loop", func() {
				// TODO
			})
		})
	})

	Convey("Rendezvous", t, func() {

		Convey("BrokerChannel with no front domain", func() {
			b := NewBrokerChannel("test.broker", "")
			So(b.url, ShouldNotBeNil)
			So(b.url.Path, ShouldResemble, "test.broker")
			So(b.transport, ShouldNotBeNil)
		})

		Convey("BrokerChannel with front domain", func() {
			b := NewBrokerChannel("test.broker", "front")
			So(b.url, ShouldNotBeNil)
			So(b.url.Path, ShouldResemble, "test.broker")
			So(b.url.Host, ShouldResemble, "front")
			So(b.transport, ShouldNotBeNil)
		})

		Convey("BrokerChannel Negotiate responds with answer", func() {
			b := NewBrokerChannel("test.broker", "")
			sdp := webrtc.DeserializeSessionDescription("test")
			// Replace transport with a mock.
			b.transport = &MockTransport{}
			answer, err := b.Negotiate(sdp)
			So(err, ShouldBeNil)
			So(answer, ShouldNotBeNil)
			So(answer.Sdp, ShouldResemble, "fake")
		})

	})
}
