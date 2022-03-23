package main

import (
	"bytes"
	"fmt"
	pt "git.torproject.org/pluggable-transports/goptlib.git"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/event"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/safelog"
	"strings"
)

func NewPTEventLogger() event.SnowflakeEventReceiver {
	return &ptEventLogger{}
}

type ptEventLogger struct {
}

type logSeverity int

const (
	Debug logSeverity = iota
	Info
	Notice
	Warning
	Error
)

func safePTLog(severity logSeverity, format string, a ...interface{}) {
	var buff bytes.Buffer
	scrubber := &safelog.LogScrubber{Output: &buff}

	// make sure logString ends with exactly one "\n" so it's not stuck in scrubber.Write()'s internal buffer
	logString := strings.TrimRight(fmt.Sprintf(format, a...), "\n") + "\n"
	scrubber.Write([]byte(logString))

	// remove newline before calling pt.Log because it adds a newline
	msg := strings.TrimRight(buff.String(), "\n")

	switch severity {
	case Error:
		pt.Log(pt.LogSeverityError, msg)
	case Warning:
		pt.Log(pt.LogSeverityWarning, msg)
	case Notice:
		pt.Log(pt.LogSeverityWarning, msg)
	case Info:
		pt.Log(pt.LogSeverityInfo, msg)
	case Debug:
		pt.Log(pt.LogSeverityDebug, msg)
	default:
		pt.Log(pt.LogSeverityNotice, msg)
	}
}

func (p ptEventLogger) OnNewSnowflakeEvent(e event.SnowflakeEvent) {
	switch e.(type) {
	case event.EventOnOfferCreated:
		e := e.(event.EventOnOfferCreated)
		if e.Error != nil {
			safePTLog(Notice, "offer creation failure %v", e.Error.Error())
		} else {
			safePTLog(Notice, "offer created")
		}

	case event.EventOnBrokerRendezvous:
		e := e.(event.EventOnBrokerRendezvous)
		if e.Error != nil {
			safePTLog(Notice, "broker failure %v", e.Error.Error())
		} else {
			safePTLog(Notice, "broker rendezvous peer received")
		}

	case event.EventOnSnowflakeConnected:
		safePTLog(Notice, "connected")

	case event.EventOnSnowflakeConnectionFailed:
		e := e.(event.EventOnSnowflakeConnectionFailed)
		safePTLog(Notice, "trying a new proxy: %v", e.Error.Error())
	}

}
