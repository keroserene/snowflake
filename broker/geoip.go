/*
This code is for loading database data that maps ip addresses to countries
for collecting and presenting statistics on snowflake use that might alert us
to censorship events.

The functions here are heavily based off of how tor maintains and searches their
geoip database

The tables used for geoip data must be structured as follows:

Recognized line format for IPv4 is:
    INTIPLOW,INTIPHIGH,CC
        where INTIPLOW and INTIPHIGH are IPv4 addresses encoded as big-endian 4-byte unsigned
        integers, and CC is a country code.

Note that the IPv4 line format
    "INTIPLOW","INTIPHIGH","CC","CC3","COUNTRY NAME"
is not currently supported.

Recognized line format for IPv6 is:
    IPV6LOW,IPV6HIGH,CC
        where IPV6LOW and IPV6HIGH are IPv6 addresses and CC is a country code.

It also recognizes, and skips over, blank lines and lines that start
with '#' (comments).

*/
package main

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type GeoIPTable interface {
	parseEntry(string) (*GeoIPEntry, error)
	Len() int
	Append(GeoIPEntry)
	ElementAt(int) GeoIPEntry
	Lock()
	Unlock()
}

type GeoIPEntry struct {
	ipLow   net.IP
	ipHigh  net.IP
	country string
}

type GeoIPv4Table struct {
	table []GeoIPEntry

	lock sync.Mutex // synchronization for geoip table accesses and reloads
}

type GeoIPv6Table struct {
	table []GeoIPEntry

	lock sync.Mutex // synchronization for geoip table accesses and reloads
}

func (table *GeoIPv4Table) Len() int { return len(table.table) }
func (table *GeoIPv6Table) Len() int { return len(table.table) }

func (table *GeoIPv4Table) Append(entry GeoIPEntry) {
	(*table).table = append(table.table, entry)
}
func (table *GeoIPv6Table) Append(entry GeoIPEntry) {
	(*table).table = append(table.table, entry)
}

func (table *GeoIPv4Table) ElementAt(i int) GeoIPEntry { return table.table[i] }
func (table *GeoIPv6Table) ElementAt(i int) GeoIPEntry { return table.table[i] }

func (table *GeoIPv4Table) Lock() { (*table).lock.Lock() }
func (table *GeoIPv6Table) Lock() { (*table).lock.Lock() }

func (table *GeoIPv4Table) Unlock() { (*table).lock.Unlock() }
func (table *GeoIPv6Table) Unlock() { (*table).lock.Unlock() }

// Convert a geoip IP address represented as a big-endian unsigned integer to net.IP
func geoipStringToIP(ipStr string) (net.IP, error) {
	ip, err := strconv.ParseUint(ipStr, 10, 32)
	if err != nil {
		return net.IPv4(0, 0, 0, 0), fmt.Errorf("error parsing IP %s", ipStr)
	}
	var bytes [4]byte
	bytes[0] = byte(ip & 0xFF)
	bytes[1] = byte((ip >> 8) & 0xFF)
	bytes[2] = byte((ip >> 16) & 0xFF)
	bytes[3] = byte((ip >> 24) & 0xFF)

	return net.IPv4(bytes[3], bytes[2], bytes[1], bytes[0]), nil
}

//Parses a line in the provided geoip file that corresponds
//to an address range and a two character country code
func (table *GeoIPv4Table) parseEntry(candidate string) (*GeoIPEntry, error) {

	if candidate[0] == '#' {
		return nil, nil
	}

	parsedCandidate := strings.Split(candidate, ",")

	if len(parsedCandidate) != 3 {
		return nil, fmt.Errorf("provided geoip file is incorrectly formatted. Could not parse line:\n%s", parsedCandidate)
	}

	low, err := geoipStringToIP(parsedCandidate[0])
	if err != nil {
		return nil, err
	}
	high, err := geoipStringToIP(parsedCandidate[1])
	if err != nil {
		return nil, err
	}

	geoipEntry := &GeoIPEntry{
		ipLow:   low,
		ipHigh:  high,
		country: parsedCandidate[2],
	}

	return geoipEntry, nil
}

//Parses a line in the provided geoip file that corresponds
//to an address range and a two character country code
func (table *GeoIPv6Table) parseEntry(candidate string) (*GeoIPEntry, error) {

	if candidate[0] == '#' {
		return nil, nil
	}

	parsedCandidate := strings.Split(candidate, ",")

	if len(parsedCandidate) != 3 {
		return nil, fmt.Errorf("")
	}

	low := net.ParseIP(parsedCandidate[0])
	if low == nil {
		return nil, fmt.Errorf("")
	}
	high := net.ParseIP(parsedCandidate[1])
	if high == nil {
		return nil, fmt.Errorf("")
	}

	geoipEntry := &GeoIPEntry{
		ipLow:   low,
		ipHigh:  high,
		country: parsedCandidate[2],
	}

	return geoipEntry, nil
}

//Loads provided geoip file into our tables
//Entries are stored in a table
func GeoIPLoadFile(table GeoIPTable, pathname string) error {
	//open file
	geoipFile, err := os.Open(pathname)
	if err != nil {
		return err
	}
	defer geoipFile.Close()

	hash := sha1.New()

	table.Lock()
	defer table.Unlock()

	hashedFile := io.TeeReader(geoipFile, hash)

	//read in strings and call parse function
	scanner := bufio.NewScanner(hashedFile)
	for scanner.Scan() {
		entry, err := table.parseEntry(scanner.Text())
		if err != nil {
			return fmt.Errorf("provided geoip file is incorrectly formatted. Line is: %+q", scanner.Text())
		}

		if entry != nil {
			table.Append(*entry)
		}

	}
	if err := scanner.Err(); err != nil {
		return err
	}

	sha1Hash := hex.EncodeToString(hash.Sum(nil))
	log.Println("Using geoip file ", pathname, " with checksum", sha1Hash)
	log.Println("Loaded ", table.Len(), " entries into table")

	return nil
}

//Returns the country location of an IPv4 or IPv6 address, and a boolean value
//that indicates whether the IP address was present in the geoip database
func GetCountryByAddr(table GeoIPTable, ip net.IP) (string, bool) {

	table.Lock()
	defer table.Unlock()

	//look IP up in database
	index := sort.Search(table.Len(), func(i int) bool {
		entry := table.ElementAt(i)
		return (bytes.Compare(ip.To16(), entry.ipHigh.To16()) <= 0)
	})

	if index == table.Len() {
		return "", false
	}

	// check to see if addr is in the range specified by the returned index
	// search on IPs in invalid ranges (e.g., 127.0.0.0/8) will return the
	//country code of the next highest range
	entry := table.ElementAt(index)
	if !(bytes.Compare(ip.To16(), entry.ipLow.To16()) >= 0 &&
		bytes.Compare(ip.To16(), entry.ipHigh.To16()) <= 0) {
		return "", false
	}

	return table.ElementAt(index).country, true

}
