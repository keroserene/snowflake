package main

import (
	pt "git.torproject.org/pluggable-transports/goptlib.git"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/event"
)

func NewPTEventLogger() event.SnowflakeEventReceiver {
	return &ptEventLogger{}
}

type ptEventLogger struct {
}

func (p ptEventLogger) OnNewSnowflakeEvent(e event.SnowflakeEvent) {
	pt.Log(pt.LogSeverityNotice, e.String())
}
