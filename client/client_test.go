package main

import (
	"bytes"
	"github.com/keroserene/go-webrtc"
	. "github.com/smartystreets/goconvey/convey"
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

			Convey("Receive answer sets remote description", func() {
				c.answerChannel = make(chan *webrtc.SessionDescription)
				c.config = webrtc.NewConfiguration()
				c.preparePeerConnection()
				c.receiveAnswer()
				sdp := webrtc.DeserializeSessionDescription("test")
				c.answerChannel <- sdp
				So(c.pc.RemoteDescription(), ShouldEqual, sdp)

			})

			Convey("Receive answer fails on nil answer", func() {
				c.reset = make(chan struct{})
				c.answerChannel = make(chan *webrtc.SessionDescription)
				c.receiveAnswer()
				c.answerChannel <- nil
				<-c.reset
			})

			Convey("Connect Loop", func() {
				// TODO
			})
		})

	})
}
