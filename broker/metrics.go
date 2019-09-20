/*
We export metrics in the following format:

    "snowflake-stats-end" YYYY-MM-DD HH:MM:SS (NSEC s) NL
        [At most once.]

        YYYY-MM-DD HH:MM:SS defines the end of the included measurement
        interval of length NSEC seconds (86400 seconds by default).

    "snowflake-ips" CC=NUM,CC=NUM,... NL
        [At most once.]

        List of mappings from two-letter country codes to the number of
        unique IP addresses of snowflake proxies that have polled.

    "snowflake-ips-total" NUM NL
        [At most once.]

        A count of the total number of unique IP addresses of snowflake
        proxies that have polled.

    "snowflake-idle-count" NUM NL
        [At most once.]

        A count of the number of times a proxy has polled but received
        no client offer, rounded up to the nearest multiple of 8.

    "client-denied-count" NUM NL
        [At most once.]

        A count of the number of times a client has requested a proxy
        from the broker but no proxies were available, rounded up to
        the nearest multiple of 8.

    "client-snowflake-match-count" NUM NL
        [At most once.]

        A count of the number of times a client successfully received a
        proxy from the broker, rounded up to the nearest multiple of 8.
*/

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

const metricsResolution = 60 * 60 * 24 * time.Second //86400 seconds

type CountryStats struct {
	addrs  map[string]bool
	counts map[string]int
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

func (m *Metrics) UpdateCountryStats(addr string) {

	var country string
	var ok bool

	if m.countryStats.addrs[addr] {
		return
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
	m.countryStats.addrs[addr] = true

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
		counts: make(map[string]int),
		addrs:  make(map[string]bool),
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
	m.logger.Println("snowflake-ips-total", len(m.countryStats.addrs))
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
	m.countryStats.addrs = make(map[string]bool)
}

// Rounds up a count to the nearest multiple of 8.
func binCount(count uint) uint {
	return uint((math.Ceil(float64(count) / 8)) * 8)
}
