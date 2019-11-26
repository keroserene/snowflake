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
	"sync"
	"time"
)

var (
	once sync.Once
)

const metricsResolution = 60 * 60 * 24 * time.Second //86400 seconds

type CountryStats struct {
	standalone map[string]bool
	badge      map[string]bool
	webext     map[string]bool
	unknown    map[string]bool
	counts     map[string]int
}

// Implements Observable
type Metrics struct {
	logger  *log.Logger
	tablev4 *GeoIPv4Table
	tablev6 *GeoIPv6Table

	countryStats            CountryStats
	clientRoundtripEstimate time.Duration
	proxyIdleCount          uint
	clientDeniedCount       uint
	clientProxyMatchCount   uint
}

func (s CountryStats) Display() string {
	output := ""
	for cc, count := range s.counts {
		output += fmt.Sprintf("%s=%d,", cc, count)
	}

	// cut off trailing ","
	if len(output) > 0 {
		return output[:len(output)-1]
	}

	return output
}

func (m *Metrics) UpdateCountryStats(addr string, proxyType string) {

	var country string
	var ok bool

	if proxyType == "standalone" {
		if m.countryStats.standalone[addr] {
			return
		}
	} else if proxyType == "badge" {
		if m.countryStats.badge[addr] {
			return
		}
	} else if proxyType == "webext" {
		if m.countryStats.webext[addr] {
			return
		}
	} else {
		if m.countryStats.unknown[addr] {
			return
		}
	}

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
	}

	//update map of unique ips and counts
	m.countryStats.counts[country]++
	if proxyType == "standalone" {
		m.countryStats.standalone[addr] = true
	} else if proxyType == "badge" {
		m.countryStats.badge[addr] = true
	} else if proxyType == "webext" {
		m.countryStats.webext[addr] = true
	} else {
		m.countryStats.unknown[addr] = true
	}

}

func (m *Metrics) LoadGeoipDatabases(geoipDB string, geoip6DB string) error {

	// Load geoip databases
	log.Println("Loading geoip databases")
	tablev4 := new(GeoIPv4Table)
	err := GeoIPLoadFile(tablev4, geoipDB)
	if err != nil {
		m.tablev4 = nil
		return err
	}
	m.tablev4 = tablev4

	tablev6 := new(GeoIPv6Table)
	err = GeoIPLoadFile(tablev6, geoip6DB)
	if err != nil {
		m.tablev6 = nil
		return err
	}
	m.tablev6 = tablev6
	return nil
}

func NewMetrics(metricsLogger *log.Logger) (*Metrics, error) {
	m := new(Metrics)

	m.countryStats = CountryStats{
		counts:     make(map[string]int),
		standalone: make(map[string]bool),
		badge:      make(map[string]bool),
		webext:     make(map[string]bool),
		unknown:    make(map[string]bool),
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
	m.logger.Println("snowflake-stats-end", time.Now().UTC().Format("2006-01-02 15:04:05"), fmt.Sprintf("(%d s)", int(metricsResolution.Seconds())))
	m.logger.Println("snowflake-ips", m.countryStats.Display())
	m.logger.Println("snowflake-ips-total", len(m.countryStats.standalone)+
		len(m.countryStats.badge)+len(m.countryStats.webext)+len(m.countryStats.unknown))
	m.logger.Println("snowflake-ips-standalone", len(m.countryStats.standalone))
	m.logger.Println("snowflake-ips-badge", len(m.countryStats.badge))
	m.logger.Println("snowflake-ips-webext", len(m.countryStats.webext))
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
	m.countryStats.standalone = make(map[string]bool)
	m.countryStats.badge = make(map[string]bool)
	m.countryStats.webext = make(map[string]bool)
	m.countryStats.unknown = make(map[string]bool)
}

// Rounds up a count to the nearest multiple of 8.
func binCount(count uint) uint {
	return uint((math.Ceil(float64(count) / 8)) * 8)
}
