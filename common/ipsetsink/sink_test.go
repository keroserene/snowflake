package ipsetsink

import (
	"fmt"
	"github.com/clarkduvall/hyperloglog"
	"testing"
)
import . "github.com/smartystreets/goconvey/convey"

func TestSinkInit(t *testing.T) {
	Convey("Context", t, func() {
		sink := NewIPSetSink("demo")
		sink.AddIPToSet("test1")
		sink.AddIPToSet("test2")
		data, err := sink.Dump()
		So(err, ShouldBeNil)
		structure, err := hyperloglog.NewPlus(18)
		So(err, ShouldBeNil)
		err = structure.GobDecode(data)
		So(err, ShouldBeNil)
		count := structure.Count()
		So(count, ShouldBeBetweenOrEqual, 1, 3)
	})
}

func TestSinkCounting(t *testing.T) {
	Convey("Context", t, func() {
		for itemCount := 300; itemCount <= 10000; itemCount += 200 {
			sink := NewIPSetSink("demo")
			for i := 0; i <= itemCount; i++ {
				sink.AddIPToSet(fmt.Sprintf("demo%v", i))
			}
			for i := 0; i <= itemCount; i++ {
				sink.AddIPToSet(fmt.Sprintf("demo%v", i))
			}
			data, err := sink.Dump()
			So(err, ShouldBeNil)
			structure, err := hyperloglog.NewPlus(18)
			So(err, ShouldBeNil)
			err = structure.GobDecode(data)
			So(err, ShouldBeNil)
			count := structure.Count()
			So((float64(count)/float64(itemCount))-1.0, ShouldAlmostEqual, 0, 0.01)
		}

	})
}
