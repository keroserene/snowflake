package main

import (
	"flag"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/event"
	"io"
	"io/ioutil"
	"log"
	"os"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/safelog"
	sf "git.torproject.org/pluggable-transports/snowflake.git/v2/proxy/lib"
)

func main() {
	capacity := flag.Uint("capacity", 0, "maximum concurrent clients")
	stunURL := flag.String("stun", sf.DefaultSTUNURL, "broker URL")
	logFilename := flag.String("log", "", "log filename")
	rawBrokerURL := flag.String("broker", sf.DefaultBrokerURL, "broker URL")
	unsafeLogging := flag.Bool("unsafe-logging", false, "prevent logs from being scrubbed")
	keepLocalAddresses := flag.Bool("keep-local-addresses", false, "keep local LAN address ICE candidates")
	relayURL := flag.String("relay", sf.DefaultRelayURL, "websocket relay URL")
	NATTypeMeasurementInterval := flag.Duration("nat-retest-interval", time.Hour*24,
		"the time interval in second before NAT type is retested, 0s disables retest. Valid time units are \"s\", \"m\", \"h\". ")
	SummaryInterval := flag.Duration("summary-interval", time.Hour,
		"the time interval to output summary, 0s disables retest. Valid time units are \"s\", \"m\", \"h\". ")
	verboseLogging := flag.Bool("verbose", false, "increase log verbosity")

	flag.Parse()

	eventLogger := event.NewSnowflakeEventDispatcher()

	proxy := sf.SnowflakeProxy{
		Capacity:           uint(*capacity),
		STUNURL:            *stunURL,
		BrokerURL:          *rawBrokerURL,
		KeepLocalAddresses: *keepLocalAddresses,
		RelayURL:           *relayURL,

		NATTypeMeasurementInterval: *NATTypeMeasurementInterval,
		EventDispatcher:            eventLogger,
	}

	var logOutput io.Writer = os.Stderr
	var eventlogOutput io.Writer = os.Stderr
	log.SetFlags(log.LstdFlags | log.LUTC)

	if !*verboseLogging {
		logOutput = ioutil.Discard
	}

	if *logFilename != "" {
		f, err := os.OpenFile(*logFilename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		logOutput = io.MultiWriter(logOutput, f)
		eventlogOutput = io.MultiWriter(eventlogOutput, f)
	}
	if *unsafeLogging {
		log.SetOutput(logOutput)
	} else {
		log.SetOutput(&safelog.LogScrubber{Output: logOutput})
	}

	periodicEventLogger := sf.NewProxyEventLogger(*SummaryInterval, eventlogOutput)
	eventLogger.AddSnowflakeEventListener(periodicEventLogger)

	err := proxy.Start()
	if err != nil {
		log.Fatal(err)
	}
}
