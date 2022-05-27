package sinkcluster

import (
	"bytes"
	"io"
	"testing"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/ipsetsink"

	. "github.com/smartystreets/goconvey/convey"
)

type writerStub struct {
	io.Writer
}

func (w writerStub) Sync() error {
	return nil
}

func TestSinkWriter(t *testing.T) {

	Convey("Context", t, func() {
		buffer := bytes.NewBuffer(nil)
		writerStubInst := &writerStub{buffer}
		sink := ipsetsink.NewIPSetSink("demo")
		clusterWriter := NewClusterWriter(writerStubInst, time.Minute, sink)
		clusterWriter.AddIPToSet("1")
		clusterWriter.WriteIPSetToDisk()
		So(buffer.Bytes(), ShouldNotBeNil)
	})
}
