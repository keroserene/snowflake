package snowflake_broker

import (
	"bytes"
	"container/heap"
	. "github.com/smartystreets/goconvey/convey"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBroker(t *testing.T) {

	Convey("Context", t, func() {
		ctx := NewBrokerContext()

		Convey("Adds Snowflake", func() {
			ctx := NewBrokerContext()
			So(ctx.snowflakes.Len(), ShouldEqual, 0)
			So(len(ctx.snowflakeMap), ShouldEqual, 0)
			ctx.AddSnowflake("foo")
			So(ctx.snowflakes.Len(), ShouldEqual, 1)
			So(len(ctx.snowflakeMap), ShouldEqual, 1)
		})

		Convey("Responds to client offers...", func() {

			w := httptest.NewRecorder()
			data := bytes.NewReader([]byte("test"))
			r, err := http.NewRequest("POST", "broker.com/client", data)
			So(err, ShouldBeNil)

			Convey("with 503 when no snowflakes are available.", func() {
				clientHandler(ctx, w, r)
				h := w.Header()
				So(h["Access-Control-Allow-Headers"], ShouldNotBeNil)
				So(w.Code, ShouldEqual, http.StatusServiceUnavailable)
				So(w.Body.String(), ShouldEqual, "")
			})

			Convey("with a proxy answer if available.", func() {
				done := make(chan bool)
				// Prepare a fake proxy to respond with.
				snowflake := ctx.AddSnowflake("fake")
				go func() {
					clientHandler(ctx, w, r)
					done <- true
				}()
				offer := <-snowflake.offerChannel
				So(offer, ShouldResemble, []byte("test"))
				snowflake.answerChannel <- []byte("fake answer")
				<-done
				So(w.Body.String(), ShouldEqual, "fake answer")
				So(w.Code, ShouldEqual, http.StatusOK)
			})

			Convey("Times out when no proxy responds.", func() {
				done := make(chan bool)
				snowflake := ctx.AddSnowflake("fake")
				go func() {
					clientHandler(ctx, w, r)
					done <- true
				}()
				offer := <-snowflake.offerChannel
				So(offer, ShouldResemble, []byte("test"))
				<-done
				So(w.Code, ShouldEqual, http.StatusGatewayTimeout)
			})

		})
	})
}

func TestSnowflakeHeap(t *testing.T) {
	Convey("SnowflakeHeap", t, func() {
		h := new(SnowflakeHeap)
		heap.Init(h)
		So(h.Len(), ShouldEqual, 0)
		s1 := new(Snowflake)
		s2 := new(Snowflake)
		s3 := new(Snowflake)
		s4 := new(Snowflake)
		s1.clients = 4
		s2.clients = 5
		s3.clients = 3
		s4.clients = 1

		heap.Push(h, s1)
		So(h.Len(), ShouldEqual, 1)
		heap.Push(h, s2)
		So(h.Len(), ShouldEqual, 2)
		heap.Push(h, s3)
		So(h.Len(), ShouldEqual, 3)
		heap.Push(h, s4)
		So(h.Len(), ShouldEqual, 4)

		heap.Remove(h, 0)
		So(h.Len(), ShouldEqual, 3)

		r := heap.Pop(h).(*Snowflake)
		So(h.Len(), ShouldEqual, 2)
		So(r.clients, ShouldEqual, 3)
		So(r.index, ShouldEqual, -1)

		r = heap.Pop(h).(*Snowflake)
		So(h.Len(), ShouldEqual, 1)
		So(r.clients, ShouldEqual, 4)
		So(r.index, ShouldEqual, -1)

		r = heap.Pop(h).(*Snowflake)
		So(h.Len(), ShouldEqual, 0)
		So(r.clients, ShouldEqual, 5)
		So(r.index, ShouldEqual, -1)
	})
}
