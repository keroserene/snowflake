package snowflake_client

import (
	"fmt"

	pt "git.torproject.org/pluggable-transports/goptlib.git"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/event"
)

func NewPTEventLogger() event.SnowflakeEventReceiver {
	return &ptEventLogger{}
}

type ptEventLogger struct {
}

func (p ptEventLogger) OnNewSnowflakeEvent(e event.SnowflakeEvent) {
	switch e.(type) {
	case event.EventOnOfferCreated:
		e := e.(event.EventOnOfferCreated)
		if e.Error != nil {
			pt.Log(pt.LogSeverityError, fmt.Sprintf("offer creation failure %v", e.Error.Error()))
		} else {
			pt.Log(pt.LogSeverityNotice, fmt.Sprintf("offer created"))
		}

	case event.EventOnBrokerRendezvous:
		e := e.(event.EventOnBrokerRendezvous)
		if e.Error != nil {
			pt.Log(pt.LogSeverityError, fmt.Sprintf("broker failure %v", e.Error.Error()))
		} else {
			pt.Log(pt.LogSeverityNotice, fmt.Sprintf("broker rendezvous peer received"))
		}

	case event.EventOnSnowflakeConnected:
		pt.Log(pt.LogSeverityNotice, fmt.Sprintf("connected"))

	case event.EventOnSnowflakeConnectionFailed:
		e := e.(event.EventOnSnowflakeConnectionFailed)
		pt.Log(pt.LogSeverityError, fmt.Sprintf("connection failed %v", e.Error.Error()))
	}

}
