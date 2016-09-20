package snowflake_broker

import (
	// "golang.org/x/net/internal/timeseries"
	"time"
)

// Implements Observable
type Metrics struct {
	// snowflakes	timeseries.Float
	clientRoundtripEstimate time.Duration
}

func NewMetrics() *Metrics {
	m := new(Metrics)
	return m
}
