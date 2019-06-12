package main

import (
	// "golang.org/x/net/internal/timeseries"
	"fmt"
	"log"
	"math"
	"net"
	"sync"
	"time"
)

var (
	once sync.Once
)

const metricsResolution = 86400 * time.Second

type CountryStats struct {
	ips    map[string]bool
	counts map[string]int
}

// Implements Observable
type Metrics struct {
	logger  *log.Logger
	tablev4 *GeoIPv4Table
	tablev6 *GeoIPv6Table

	countryStats            CountryStats
	clientRoundtripEstimate time.Duration
	proxyIdleCount          int
	clientDeniedCount       int
	clientProxyMatchCount   int
}

func (s CountryStats) Display() string {
	output := ""
	for cc, count := range s.counts {
		output += fmt.Sprintf("%s=%d,", cc, count)
	}
	return output
}

func (m *Metrics) UpdateCountryStats(addr string) {

	var country string
	var ok bool

	ip := net.ParseIP(addr)
	if ip.To4() != nil {
		//This is an IPv4 address
		if m.tablev4 == nil {
			return
		}
		country, ok = GetCountryByAddr(m.tablev4, ip)
	} else {
		if m.tablev6 == nil {
			return
		}
		country, ok = GetCountryByAddr(m.tablev6, ip)
	}

	if !ok {
		country = "??"
		log.Println("Unknown geoip")
	}

	//update map of unique ips and counts
	if !m.countryStats.ips[addr] {
		m.countryStats.counts[country]++
		m.countryStats.ips[addr] = true
	}

	return
}

func (m *Metrics) LoadGeoipDatabases(geoipDB string, geoip6DB string) error {

	// Load geoip databases
	log.Println("Loading geoip databases")
	tablev4 := new(GeoIPv4Table)
	err := GeoIPLoadFile(tablev4, geoipDB)
	if err != nil {
		m.tablev4 = nil
		return err
	} else {
		m.tablev4 = tablev4
	}

	tablev6 := new(GeoIPv6Table)
	err = GeoIPLoadFile(tablev6, geoip6DB)
	if err != nil {
		m.tablev6 = nil
		return err
	} else {
		m.tablev6 = tablev6
	}

	return nil
}

func NewMetrics(metricsLogger *log.Logger) (*Metrics, error) {
	m := new(Metrics)

	m.countryStats = CountryStats{
		counts: make(map[string]int),
		ips:    make(map[string]bool),
	}

	m.logger = metricsLogger

	// Write to log file every hour with updated metrics
	go once.Do(m.logMetrics)

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
	m.logger.Println("snowflake-stats-end", time.Now().UTC().Format("2006-01-02 15:04:05"), "(", int(metricsResolution.Seconds()), "s)")
	m.logger.Println("snowflake-ips", m.countryStats.Display())
	m.logger.Println("snowflake-idle-count", binCount(m.proxyIdleCount))
	m.logger.Println("client-denied-count", binCount(m.clientDeniedCount))
	m.logger.Println("client-snowflake-match-count", binCount(m.clientProxyMatchCount))
}

// Restores all metrics to original values
func (m *Metrics) zeroMetrics() {
	m.proxyIdleCount = 0
	m.clientDeniedCount = 0
	m.clientProxyMatchCount = 0
	m.countryStats.counts = make(map[string]int)
	m.countryStats.ips = make(map[string]bool)
}

// Rounds up a count to the nearest multiple of 8.
func binCount(count int) int {
	return int((math.Ceil(float64(count) / 8)) * 8)
}
