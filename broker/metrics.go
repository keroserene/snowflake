/*
We export metrics in the format specified in our broker spec:
https://gitweb.torproject.org/pluggable-transports/snowflake.git/tree/doc/broker-spec.txt
*/

package main

import (
	"fmt"
	"log"
	"math"
	"net"
	"sort"
	"sync"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/ipsetsink/sinkcluster"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/messages"
	"github.com/prometheus/client_golang/prometheus"
	"gitlab.torproject.org/tpo/anti-censorship/geoip"
)

const (
	prometheusNamespace = "snowflake"
	metricsResolution   = 60 * 60 * 24 * time.Second //86400 seconds
)

type CountryStats struct {
	// map[proxyType][address]bool
	proxies map[string]map[string]bool
	unknown map[string]bool

	natRestricted   map[string]bool
	natUnrestricted map[string]bool
	natUnknown      map[string]bool

	counts map[string]int
}

// Implements Observable
type Metrics struct {
	logger  *log.Logger
	geoipdb *geoip.Geoip

	distinctIPWriter *sinkcluster.ClusterWriter

	countryStats                  CountryStats
	clientRoundtripEstimate       time.Duration
	proxyIdleCount                uint
	clientDeniedCount             uint
	clientRestrictedDeniedCount   uint
	clientUnrestrictedDeniedCount uint
	clientProxyMatchCount         uint

	proxyPollWithRelayURLExtension         uint
	proxyPollWithoutRelayURLExtension      uint
	proxyPollRejectedWithRelayURLExtension uint

	// synchronization for access to snowflake metrics
	lock sync.Mutex

	promMetrics *PromMetrics
}

type record struct {
	cc    string
	count int
}
type records []record

func (r records) Len() int      { return len(r) }
func (r records) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r records) Less(i, j int) bool {
	if r[i].count == r[j].count {
		return r[i].cc > r[j].cc
	}
	return r[i].count < r[j].count
}

func (s CountryStats) Display() string {
	output := ""

	// Use the records struct to sort our counts map by value.
	rs := records{}
	for cc, count := range s.counts {
		rs = append(rs, record{cc: cc, count: count})
	}
	sort.Sort(sort.Reverse(rs))
	for _, r := range rs {
		output += fmt.Sprintf("%s=%d,", r.cc, r.count)
	}

	// cut off trailing ","
	if len(output) > 0 {
		return output[:len(output)-1]
	}

	return output
}

func (m *Metrics) UpdateCountryStats(addr string, proxyType string, natType string) {

	var country string
	var ok bool

	addresses, ok := m.countryStats.proxies[proxyType]
	if !ok {
		if m.countryStats.unknown[addr] {
			return
		}
		m.countryStats.unknown[addr] = true
	} else {
		if addresses[addr] {
			return
		}
		addresses[addr] = true
	}

	ip := net.ParseIP(addr)
	if m.geoipdb == nil {
		return
	}
	country, ok = m.geoipdb.GetCountryByAddr(ip)
	if !ok {
		country = "??"
	}
	m.countryStats.counts[country]++

	m.promMetrics.ProxyTotal.With(prometheus.Labels{
		"nat":  natType,
		"type": proxyType,
		"cc":   country,
	}).Inc()

	switch natType {
	case NATRestricted:
		m.countryStats.natRestricted[addr] = true
	case NATUnrestricted:
		m.countryStats.natUnrestricted[addr] = true
	default:
		m.countryStats.natUnknown[addr] = true
	}

}

func (m *Metrics) LoadGeoipDatabases(geoipDB string, geoip6DB string) error {

	// Load geoip databases
	var err error
	log.Println("Loading geoip databases")
	m.geoipdb, err = geoip.New(geoipDB, geoip6DB)
	return err
}

func NewMetrics(metricsLogger *log.Logger) (*Metrics, error) {
	m := new(Metrics)

	m.countryStats = CountryStats{
		counts:          make(map[string]int),
		proxies:         make(map[string]map[string]bool),
		unknown:         make(map[string]bool),
		natRestricted:   make(map[string]bool),
		natUnrestricted: make(map[string]bool),
		natUnknown:      make(map[string]bool),
	}
	for pType := range messages.KnownProxyTypes {
		m.countryStats.proxies[pType] = make(map[string]bool)
	}

	m.logger = metricsLogger
	m.promMetrics = initPrometheus()

	// Write to log file every day with updated metrics
	go m.logMetrics()

	return m, nil
}

// Logs metrics in intervals specified by metricsResolution
func (m *Metrics) logMetrics() {
	heartbeat := time.Tick(metricsResolution)
	for range heartbeat {
		m.printMetrics()
		m.zeroMetrics()
	}
}

func (m *Metrics) printMetrics() {
	m.lock.Lock()
	m.logger.Println("snowflake-stats-end", time.Now().UTC().Format("2006-01-02 15:04:05"), fmt.Sprintf("(%d s)", int(metricsResolution.Seconds())))
	m.logger.Println("snowflake-ips", m.countryStats.Display())
	total := len(m.countryStats.unknown)
	for pType, addresses := range m.countryStats.proxies {
		m.logger.Printf("snowflake-ips-%s %d\n", pType, len(addresses))
		total += len(addresses)
	}
	m.logger.Println("snowflake-ips-total", total)
	m.logger.Println("snowflake-idle-count", binCount(m.proxyIdleCount))
	m.logger.Println("snowflake-proxy-poll-with-relay-url-count", binCount(m.proxyPollWithRelayURLExtension))
	m.logger.Println("snowflake-proxy-poll-without-relay-url-count", binCount(m.proxyPollWithoutRelayURLExtension))
	m.logger.Println("snowflake-proxy-rejected-for-relay-url-count", binCount(m.proxyPollRejectedWithRelayURLExtension))
	m.logger.Println("client-denied-count", binCount(m.clientDeniedCount))
	m.logger.Println("client-restricted-denied-count", binCount(m.clientRestrictedDeniedCount))
	m.logger.Println("client-unrestricted-denied-count", binCount(m.clientUnrestrictedDeniedCount))
	m.logger.Println("client-snowflake-match-count", binCount(m.clientProxyMatchCount))
	m.logger.Println("snowflake-ips-nat-restricted", len(m.countryStats.natRestricted))
	m.logger.Println("snowflake-ips-nat-unrestricted", len(m.countryStats.natUnrestricted))
	m.logger.Println("snowflake-ips-nat-unknown", len(m.countryStats.natUnknown))
	m.lock.Unlock()
}

// Restores all metrics to original values
func (m *Metrics) zeroMetrics() {
	m.proxyIdleCount = 0
	m.clientDeniedCount = 0
	m.clientRestrictedDeniedCount = 0
	m.clientUnrestrictedDeniedCount = 0
	m.proxyPollRejectedWithRelayURLExtension = 0
	m.proxyPollWithRelayURLExtension = 0
	m.proxyPollWithoutRelayURLExtension = 0
	m.clientProxyMatchCount = 0
	m.countryStats.counts = make(map[string]int)
	for pType := range m.countryStats.proxies {
		m.countryStats.proxies[pType] = make(map[string]bool)
	}
	m.countryStats.unknown = make(map[string]bool)
	m.countryStats.natRestricted = make(map[string]bool)
	m.countryStats.natUnrestricted = make(map[string]bool)
	m.countryStats.natUnknown = make(map[string]bool)
}

// Rounds up a count to the nearest multiple of 8.
func binCount(count uint) uint {
	return uint((math.Ceil(float64(count) / 8)) * 8)
}

type PromMetrics struct {
	registry         *prometheus.Registry
	ProxyTotal       *prometheus.CounterVec
	ProxyPollTotal   *RoundedCounterVec
	ClientPollTotal  *RoundedCounterVec
	AvailableProxies *prometheus.GaugeVec

	ProxyPollWithRelayURLExtensionTotal    *RoundedCounterVec
	ProxyPollWithoutRelayURLExtensionTotal *RoundedCounterVec

	ProxyPollRejectedForRelayURLExtensionTotal *RoundedCounterVec
}

// Initialize metrics for prometheus exporter
func initPrometheus() *PromMetrics {
	promMetrics := &PromMetrics{}

	promMetrics.registry = prometheus.NewRegistry()

	promMetrics.ProxyTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: prometheusNamespace,
			Name:      "proxy_total",
			Help:      "The number of unique snowflake IPs",
		},
		[]string{"type", "nat", "cc"},
	)

	promMetrics.AvailableProxies = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Name:      "available_proxies",
			Help:      "The number of currently available snowflake proxies",
		},
		[]string{"type", "nat"},
	)

	promMetrics.ProxyPollTotal = NewRoundedCounterVec(
		prometheus.CounterOpts{
			Namespace: prometheusNamespace,
			Name:      "rounded_proxy_poll_total",
			Help:      "The number of snowflake proxy polls, rounded up to a multiple of 8",
		},
		[]string{"nat", "status"},
	)

	promMetrics.ProxyPollWithRelayURLExtensionTotal = NewRoundedCounterVec(
		prometheus.CounterOpts{
			Namespace: prometheusNamespace,
			Name:      "rounded_proxy_poll_with_relay_url_extension_total",
			Help:      "The number of snowflake proxy polls with Relay URL Extension, rounded up to a multiple of 8",
		},
		[]string{"nat", "type"},
	)

	promMetrics.ProxyPollWithoutRelayURLExtensionTotal = NewRoundedCounterVec(
		prometheus.CounterOpts{
			Namespace: prometheusNamespace,
			Name:      "rounded_proxy_poll_without_relay_url_extension_total",
			Help:      "The number of snowflake proxy polls without Relay URL Extension, rounded up to a multiple of 8",
		},
		[]string{"nat", "type"},
	)

	promMetrics.ProxyPollRejectedForRelayURLExtensionTotal = NewRoundedCounterVec(
		prometheus.CounterOpts{
			Namespace: prometheusNamespace,
			Name:      "rounded_proxy_poll_rejected_relay_url_extension_total",
			Help:      "The number of snowflake proxy polls rejected by Relay URL Extension, rounded up to a multiple of 8",
		},
		[]string{"nat", "type"},
	)

	promMetrics.ClientPollTotal = NewRoundedCounterVec(
		prometheus.CounterOpts{
			Namespace: prometheusNamespace,
			Name:      "rounded_client_poll_total",
			Help:      "The number of snowflake client polls, rounded up to a multiple of 8",
		},
		[]string{"nat", "status"},
	)

	// We need to register our metrics so they can be exported.
	promMetrics.registry.MustRegister(
		promMetrics.ClientPollTotal, promMetrics.ProxyPollTotal,
		promMetrics.ProxyTotal, promMetrics.AvailableProxies,
		promMetrics.ProxyPollWithRelayURLExtensionTotal,
		promMetrics.ProxyPollWithoutRelayURLExtensionTotal,
		promMetrics.ProxyPollRejectedForRelayURLExtensionTotal,
	)

	return promMetrics
}

func (m *Metrics) RecordIPAddress(ip string) {
	if m.distinctIPWriter != nil {
		m.distinctIPWriter.AddIPToSet(ip)
	}
}

func (m *Metrics) SetIPAddressRecorder(recorder *sinkcluster.ClusterWriter) {
	m.distinctIPWriter = recorder
}
