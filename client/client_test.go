package main

import (
	"bytes"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

type MockDataChannel struct {
	destination bytes.Buffer
}

func (m *MockDataChannel) Send(data []byte) {
	m.destination.Write(data)
}

func (*MockDataChannel) Close() error {
	return nil
}

func TestConnect(t *testing.T) {
	Convey("Snowflake", t, func() {

		Convey("WebRTC Connection", func() {
			c := new(webRTCConn)
			So(c.buffer.Bytes(), ShouldEqual, nil)

			Convey("SendData buffers when datachannel is nil", func() {
				c.sendData([]byte("test"))
				c.snowflake = nil
				So(c.buffer.Bytes(), ShouldResemble, []byte("test"))
			})

			Convey("SendData sends to datachannel when not nil", func() {
				mock := new(MockDataChannel)
				c.snowflake = mock
				c.sendData([]byte("test"))
				So(c.buffer.Bytes(), ShouldEqual, nil)
				So(mock.destination.Bytes(), ShouldResemble, []byte("test"))
			})

			Convey("Receive answer fails on nil answer", func() {
				c.reset = make(chan struct{})
				c.ReceiveAnswer()
				answerChannel <- nil
				<-c.reset
			})

			Convey("Connect Loop", func() {
				// TODO
			})
		})

	})
}
