package snowflake_proxy

import (
	"fmt"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/task"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/event"
)

func NewProxyEventLogger(logPeriod time.Duration) event.SnowflakeEventReceiver {
	el := &logEventLogger{logPeriod: logPeriod}
	el.task = &task.Periodic{Interval: logPeriod, Execute: el.logTick}
	el.task.Start()
	return el
}

type logEventLogger struct {
	inboundSum      int
	outboundSum     int
	connectionCount int
	logPeriod       time.Duration
	task            *task.Periodic
}

func (p *logEventLogger) OnNewSnowflakeEvent(e event.SnowflakeEvent) {
	switch e.(type) {
	case event.EventOnProxyConnectionOver:
		e := e.(event.EventOnProxyConnectionOver)
		p.inboundSum += e.InboundTraffic
		p.outboundSum += e.OutboundTraffic
		p.connectionCount += 1
	}
}

func (p *logEventLogger) logTick() error {
	inbound, inboundUnit := formatTraffic(p.inboundSum)
	outbound, outboundUnit := formatTraffic(p.inboundSum)
	fmt.Printf("In the last %v, there are %v connections. Traffic Relayed ↑ %v %v, ↓ %v %v.\n",
		p.logPeriod.String(), p.connectionCount, inbound, inboundUnit, outbound, outboundUnit)
	p.outboundSum = 0
	p.inboundSum = 0
	p.connectionCount = 0
	return nil
}

func (p *logEventLogger) Close() error {
	return p.task.Close()
}
