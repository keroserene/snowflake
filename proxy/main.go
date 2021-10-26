package main

import (
	"flag"
	"io"
	"log"
	"os"

	"git.torproject.org/pluggable-transports/snowflake.git/common/safelog"
	"git.torproject.org/pluggable-transports/snowflake.git/proxy/lib"
)

func main() {
	capacity := flag.Int("capacity", 10, "maximum concurrent clients")
	stunURL := flag.String("stun", snowflake.DefaultSTUNURL, "broker URL")
	logFilename := flag.String("log", "", "log filename")
	rawBrokerURL := flag.String("broker", snowflake.DefaultBrokerURL, "broker URL")
	unsafeLogging := flag.Bool("unsafe-logging", false, "prevent logs from being scrubbed")
	keepLocalAddresses := flag.Bool("keep-local-addresses", false, "keep local LAN address ICE candidates")
	relayURL := flag.String("relay", snowflake.DefaultRelayURL, "websocket relay URL")

	flag.Parse()

	sf := snowflake.SnowflakeProxy{
		Capacity:           uint(*capacity),
		StunURL:            *stunURL,
		RawBrokerURL:       *rawBrokerURL,
		KeepLocalAddresses: *keepLocalAddresses,
		RelayURL:           *relayURL,
		LogOutput:          os.Stderr,
	}

	if *logFilename != "" {
		f, err := os.OpenFile(*logFilename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		sf.LogOutput = io.MultiWriter(os.Stderr, f)
	}
	if *unsafeLogging {
		log.SetOutput(sf.LogOutput)
	} else {
		log.SetOutput(&safelog.LogScrubber{Output: sf.LogOutput})
	}

	sf.Start()
}
