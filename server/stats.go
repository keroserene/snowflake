package main

// This code handled periodic statistics logging.
//
// The only thing it keeps track of is how many connections had the client_ip
// parameter. Write true to statsChannel to record a connection with client_ip;
// write false for without.

import (
	"log"
	"time"
)

const (
	statsInterval = 24 * time.Hour
)

var (
	statsChannel = make(chan bool)
)

func statsThread() {
	var numClientIP, numConnections uint64
	prevTime := time.Now()
	deadline := time.After(statsInterval)
	for {
		select {
		case v := <-statsChannel:
			if v {
				numClientIP++
			}
			numConnections++
		case <-deadline:
			now := time.Now()
			log.Printf("in the past %.f s, %d/%d connections had client_ip",
				(now.Sub(prevTime)).Seconds(),
				numClientIP, numConnections)
			numClientIP = 0
			numConnections = 0
			prevTime = now
			deadline = time.After(statsInterval)
		}
	}
}
