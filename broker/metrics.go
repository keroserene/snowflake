package main

import (
	// "golang.org/x/net/internal/timeseries"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

var (
	once sync.Once
)

type CountryStats struct {
	counts map[string]int
}

// Implements Observable
type Metrics struct {
	tablev4      *GeoIPv4Table
	tablev6      *GeoIPv6Table
	countryStats CountryStats
	// snowflakes	timeseries.Float
	clientRoundtripEstimate time.Duration
}

func (s CountryStats) Display() string {
	return fmt.Sprintln(s.counts)
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
	if country != "" {
		m.countryStats.counts[country]++
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

func NewMetrics() (*Metrics, error) {
	m := new(Metrics)

	f, err := os.OpenFile("metrics.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		return nil, err
	}

	metricsLogger := log.New(f, "", log.LstdFlags|log.LUTC)

	m.countryStats = CountryStats{
		counts: make(map[string]int),
	}

	// Write to log file every hour with updated metrics
	go once.Do(func() {
		heartbeat := time.Tick(time.Hour)
		for range heartbeat {
			metricsLogger.Println("Country stats: ", m.countryStats.Display())

			//restore all metrics to original values
			m.countryStats.counts = make(map[string]int)

		}
	})

	return m, nil
}
