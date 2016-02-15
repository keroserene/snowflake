package snowflake_broker

import (
	"bytes"
	"container/heap"
	. "github.com/smartystreets/goconvey/convey"
	"net/http"
	"net/http/httptest"
	"testing"
	"fmt"
)

func TestBroker(t *testing.T) {

	Convey("Context", t, func() {
		ctx := NewBrokerContext()

		Convey("Adds Snowflake", func() {
			So(ctx.snowflakes.Len(), ShouldEqual, 0)
			So(len(ctx.snowflakeMap), ShouldEqual, 0)
			ctx.AddSnowflake("foo")
			So(ctx.snowflakes.Len(), ShouldEqual, 1)
			So(len(ctx.snowflakeMap), ShouldEqual, 1)
		})

		Convey("Responds to client offers...", func() {
			w := httptest.NewRecorder()
			data := bytes.NewReader([]byte("test"))
			r, err := http.NewRequest("POST", "snowflake.broker/client", data)
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
				if testing.Short() {
					return
				}
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

		Convey("Responds to proxy polls...", func() {
			done := make(chan bool)
			w := httptest.NewRecorder()
			data := bytes.NewReader([]byte("test"))
			r, err := http.NewRequest("POST", "snowflake.broker/proxy", data)
			r.Header.Set("X-Session-ID", "test")
			So(err, ShouldBeNil)

			Convey("with a client offer if available.", func() {
				go func(ctx *BrokerContext) {
					proxyHandler(ctx, w, r)
					done <- true
				}(ctx)
				// Pass a fake client offer to this proxy
				p := <-ctx.createChan
				So(p.id, ShouldEqual, "test")
				p.offerChan <- []byte("fake offer")
				<-done
				So(w.Code, ShouldEqual, http.StatusOK)
				So(w.Body.String(), ShouldEqual, "fake offer")
			})

			Convey("times out when no client offer is available.", func() {
				go func(ctx *BrokerContext) {
					proxyHandler(ctx, w, r)
					done <- true
				}(ctx)
				p := <-ctx.createChan
				So(p.id, ShouldEqual, "test")
				// nil means timeout
				p.offerChan <- nil
				<-done
				So(w.Body.String(), ShouldEqual, "")
				So(w.Code, ShouldEqual, http.StatusGatewayTimeout)
			})
		})

		Convey("Responds to proxy answers...", func() {	
			w := httptest.NewRecorder()
			data := bytes.NewReader([]byte("fake answer"))
			s := ctx.AddSnowflake("test")

			Convey("by passing to the client if valid.", func() {
				r, err := http.NewRequest("POST", "snowflake.broker/answer", data)
				So(err, ShouldBeNil)
				r.Header.Set("X-Session-ID", "test")
				go func(ctx *BrokerContext) {
					answerHandler(ctx, w, r)
				}(ctx)
				answer := <- s.answerChannel
				So(w.Code, ShouldEqual, http.StatusOK)
				So(answer, ShouldResemble, []byte("fake answer"))
			})

			Convey("with error if the proxy is not recognized", func() {
				r, err := http.NewRequest("POST", "snowflake.broker/answer", nil)
				So(err, ShouldBeNil)
				r.Header.Set("X-Session-ID", "invalid")
				answerHandler(ctx, w, r)
				So(w.Code, ShouldEqual, http.StatusGone)
				fmt.Println("omg")
			})

			Convey("with error if the proxy gives invalid answer", func() {
				data := bytes.NewReader(nil)
				r, err := http.NewRequest("POST", "snowflake.broker/answer", data)
				r.Header.Set("X-Session-ID", "test")
				So(err, ShouldBeNil)
				answerHandler(ctx, w, r)
				So(w.Code, ShouldEqual, http.StatusBadRequest)
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
