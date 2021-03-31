/*
Implements some additional prometheus metrics that we need for privacy preserving
counts of users and proxies
*/

package main

import (
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"
)

// New Prometheus counter type that produces rounded counts of metrics
// for privacy preserving reasons
type RoundedCounter interface {
	prometheus.Metric

	Inc()
}

type roundedCounter struct {
	total uint64 //reflects the true count
	value uint64 //reflects the rounded count

	desc       *prometheus.Desc
	labelPairs []*dto.LabelPair
}

// Implements the RoundedCounter interface
func (c *roundedCounter) Inc() {
	atomic.AddUint64(&c.total, 1)
	if c.total > c.value {
		atomic.AddUint64(&c.value, 8)
	}
}

// Implements the prometheus.Metric interface
func (c *roundedCounter) Desc() *prometheus.Desc {
	return c.desc
}

// Implements the prometheus.Metric interface
func (c *roundedCounter) Write(m *dto.Metric) error {
	m.Label = c.labelPairs

	m.Counter = &dto.Counter{Value: proto.Float64(float64(c.value))}
	return nil
}

// New prometheus vector type that will track RoundedCounter metrics
// accross multiple labels
type RoundedCounterVec struct {
	*prometheus.MetricVec
}

func NewRoundedCounterVec(opts prometheus.CounterOpts, labelNames []string) *RoundedCounterVec {
	desc := prometheus.NewDesc(
		prometheus.BuildFQName(opts.Namespace, opts.Subsystem, opts.Name),
		opts.Help,
		labelNames,
		opts.ConstLabels,
	)
	return &RoundedCounterVec{
		MetricVec: prometheus.NewMetricVec(desc, func(lvs ...string) prometheus.Metric {
			if len(lvs) != len(labelNames) {
				panic("inconsistent cardinality")
			}
			return &roundedCounter{desc: desc, labelPairs: prometheus.MakeLabelPairs(desc, lvs)}
		}),
	}
}

// Helper function to return the underlying RoundedCounter metric from MetricVec
func (v *RoundedCounterVec) With(labels prometheus.Labels) RoundedCounter {
	metric, err := v.GetMetricWith(labels)
	if err != nil {
		panic(err)
	}
	return metric.(RoundedCounter)
}
