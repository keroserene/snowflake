package main

import (
	// "golang.org/x/net/internal/timeseries"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

var (
	once sync.Once
)

const metricsResolution = 24 * time.Hour

type CountryStats struct {
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
	return fmt.Sprint(s.counts)
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

	//update map of countries and counts
	m.countryStats.counts[country]++

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
	}

	m.logger = metricsLogger

	// Write to log file every hour with updated metrics
	go once.Do(m.logMetrics)

	return m, nil
}

func (m *Metrics) logMetrics() {

	heartbeat := time.Tick(metricsResolution)
	for range heartbeat {
		m.logger.Println("snowflake-stats-end ")
		m.logger.Println("snowflake-ips ", m.countryStats.Display())
		m.logger.Println("snowflake-idle-count ", m.proxyIdleCount)
		m.logger.Println("client-denied-count ", m.clientDeniedCount)
		m.logger.Println("client-snowflake-match-count ", m.clientProxyMatchCount)

		//restore all metrics to original values
		m.proxyIdleCount = 0
		m.clientDeniedCount = 0
		m.clientProxyMatchCount = 0
		m.countryStats.counts = make(map[string]int)
	}
}
