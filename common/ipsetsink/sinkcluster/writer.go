package sinkcluster

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/ipsetsink"
)

func NewClusterWriter(writer WriteSyncer, writeInterval time.Duration, sink *ipsetsink.IPSetSink) *ClusterWriter {
	c := &ClusterWriter{
		writer:        writer,
		lastWriteTime: time.Now(),
		writeInterval: writeInterval,
		current:       sink,
	}
	return c
}

type ClusterWriter struct {
	writer        WriteSyncer
	lastWriteTime time.Time
	writeInterval time.Duration
	current       *ipsetsink.IPSetSink
}

type WriteSyncer interface {
	Sync() error
	io.Writer
}

func (c *ClusterWriter) WriteIPSetToDisk() {
	currentTime := time.Now()
	data, err := c.current.Dump()
	if err != nil {
		log.Println("unable able to write ipset to file:", err)
		return
	}
	entry := &SinkEntry{
		RecordingStart: c.lastWriteTime,
		RecordingEnd:   currentTime,
		Recorded:       data,
	}
	jsonData, err := json.Marshal(entry)
	if err != nil {
		log.Println("unable able to write ipset to file:", err)
		return
	}
	jsonData = append(jsonData, byte('\n'))
	_, err = io.Copy(c.writer, bytes.NewReader(jsonData))
	if err != nil {
		log.Println("unable able to write ipset to file:", err)
		return
	}
	c.writer.Sync()
	c.lastWriteTime = currentTime
	c.current.Reset()
}

func (c *ClusterWriter) AddIPToSet(ipAddress string) {
	if c.lastWriteTime.Add(c.writeInterval).Before(time.Now()) {
		c.WriteIPSetToDisk()
	}
	c.current.AddIPToSet(ipAddress)
}
