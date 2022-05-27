/*
Broker acts as the HTTP signaling channel.
It matches clients and snowflake proxies by passing corresponding
SessionDescriptions in order to negotiate a WebRTC connection.
*/
package main

import (
	"bytes"
	"container/heap"
	"crypto/tls"
	"flag"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/bridgefingerprint"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/ipsetsink"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/ipsetsink/sinkcluster"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/namematcher"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/safelog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/crypto/acme/autocert"
)

type BrokerContext struct {
	snowflakes           *SnowflakeHeap
	restrictedSnowflakes *SnowflakeHeap
	// Maps keeping track of snowflakeIDs required to match SDP answers from
	// the second http POST. Restricted snowflakes can only be matched up with
	// clients behind an unrestricted NAT.
	idToSnowflake map[string]*Snowflake
	// Synchronization for the snowflake map and heap
	snowflakeLock sync.Mutex
	proxyPolls    chan *ProxyPoll
	metrics       *Metrics

	bridgeList                     BridgeListHolderFileBased
	allowedRelayPattern            string
	presumedPatternForLegacyClient string
}

func (ctx *BrokerContext) GetBridgeInfo(fingerprint bridgefingerprint.Fingerprint) (BridgeInfo, error) {
	return ctx.bridgeList.GetBridgeInfo(fingerprint)
}

func NewBrokerContext(metricsLogger *log.Logger) *BrokerContext {
	snowflakes := new(SnowflakeHeap)
	heap.Init(snowflakes)
	rSnowflakes := new(SnowflakeHeap)
	heap.Init(rSnowflakes)
	metrics, err := NewMetrics(metricsLogger)

	if err != nil {
		panic(err.Error())
	}

	if metrics == nil {
		panic("Failed to create metrics")
	}

	bridgeListHolder := NewBridgeListHolder()

	const DefaultBridges = `{"displayName":"default", "webSocketAddress":"wss://snowflake.torproject.net/", "fingerprint":"2B280B23E1107BB62ABFC40DDCC8824814F80A72"}
`
	bridgeListHolder.LoadBridgeInfo(bytes.NewReader([]byte(DefaultBridges)))

	return &BrokerContext{
		snowflakes:           snowflakes,
		restrictedSnowflakes: rSnowflakes,
		idToSnowflake:        make(map[string]*Snowflake),
		proxyPolls:           make(chan *ProxyPoll),
		metrics:              metrics,
		bridgeList:           bridgeListHolder,
	}
}

// Proxies may poll for client offers concurrently.
type ProxyPoll struct {
	id           string
	proxyType    string
	natType      string
	clients      int
	offerChannel chan *ClientOffer
}

// Registers a Snowflake and waits for some Client to send an offer,
// as part of the polling logic of the proxy handler.
func (ctx *BrokerContext) RequestOffer(id string, proxyType string, natType string, clients int) *ClientOffer {
	request := new(ProxyPoll)
	request.id = id
	request.proxyType = proxyType
	request.natType = natType
	request.clients = clients
	request.offerChannel = make(chan *ClientOffer)
	ctx.proxyPolls <- request
	// Block until an offer is available, or timeout which sends a nil offer.
	offer := <-request.offerChannel
	return offer
}

// goroutine which matches clients to proxies and sends SDP offers along.
// Safely processes proxy requests, responding to them with either an available
// client offer or nil on timeout / none are available.
func (ctx *BrokerContext) Broker() {
	for request := range ctx.proxyPolls {
		snowflake := ctx.AddSnowflake(request.id, request.proxyType, request.natType, request.clients)
		// Wait for a client to avail an offer to the snowflake.
		go func(request *ProxyPoll) {
			select {
			case offer := <-snowflake.offerChannel:
				request.offerChannel <- offer
			case <-time.After(time.Second * ProxyTimeout):
				// This snowflake is no longer available to serve clients.
				ctx.snowflakeLock.Lock()
				defer ctx.snowflakeLock.Unlock()
				if snowflake.index != -1 {
					if request.natType == NATUnrestricted {
						heap.Remove(ctx.snowflakes, snowflake.index)
					} else {
						heap.Remove(ctx.restrictedSnowflakes, snowflake.index)
					}
					ctx.metrics.promMetrics.AvailableProxies.With(prometheus.Labels{"nat": request.natType, "type": request.proxyType}).Dec()
					delete(ctx.idToSnowflake, snowflake.id)
					close(request.offerChannel)
				}
			}
		}(request)
	}
}

// Create and add a Snowflake to the heap.
// Required to keep track of proxies between providing them
// with an offer and awaiting their second POST with an answer.
func (ctx *BrokerContext) AddSnowflake(id string, proxyType string, natType string, clients int) *Snowflake {
	snowflake := new(Snowflake)
	snowflake.id = id
	snowflake.clients = clients
	snowflake.proxyType = proxyType
	snowflake.natType = natType
	snowflake.offerChannel = make(chan *ClientOffer)
	snowflake.answerChannel = make(chan string)
	ctx.snowflakeLock.Lock()
	if natType == NATUnrestricted {
		heap.Push(ctx.snowflakes, snowflake)
	} else {
		heap.Push(ctx.restrictedSnowflakes, snowflake)
	}
	ctx.metrics.promMetrics.AvailableProxies.With(prometheus.Labels{"nat": natType, "type": proxyType}).Inc()
	ctx.idToSnowflake[id] = snowflake
	ctx.snowflakeLock.Unlock()
	return snowflake
}

func (ctx *BrokerContext) InstallBridgeListProfile(reader io.Reader, relayPattern, presumedPatternForLegacyClient string) error {
	if err := ctx.bridgeList.LoadBridgeInfo(reader); err != nil {
		return err
	}
	ctx.allowedRelayPattern = relayPattern
	ctx.presumedPatternForLegacyClient = presumedPatternForLegacyClient
	return nil
}

func (ctx *BrokerContext) CheckProxyRelayPattern(pattern string, nonSupported bool) bool {
	if nonSupported {
		pattern = ctx.presumedPatternForLegacyClient
	}
	proxyPattern := namematcher.NewNameMatcher(pattern)
	brokerPattern := namematcher.NewNameMatcher(ctx.allowedRelayPattern)
	return proxyPattern.IsSupersetOf(brokerPattern)
}

// Client offer contains an SDP, bridge fingerprint and the NAT type of the client
type ClientOffer struct {
	natType     string
	sdp         []byte
	fingerprint []byte
}

func main() {
	var acmeEmail string
	var acmeHostnamesCommas string
	var acmeCertCacheDir string
	var addr string
	var geoipDatabase string
	var geoip6Database string
	var bridgeListFilePath, allowedRelayPattern, presumedPatternForLegacyClient string
	var disableTLS bool
	var certFilename, keyFilename string
	var disableGeoip bool
	var metricsFilename string
	var ipCountFilename, ipCountMaskingKey string
	var ipCountInterval time.Duration
	var unsafeLogging bool

	flag.StringVar(&acmeEmail, "acme-email", "", "optional contact email for Let's Encrypt notifications")
	flag.StringVar(&acmeHostnamesCommas, "acme-hostnames", "", "comma-separated hostnames for TLS certificate")
	flag.StringVar(&certFilename, "cert", "", "TLS certificate file")
	flag.StringVar(&keyFilename, "key", "", "TLS private key file")
	flag.StringVar(&acmeCertCacheDir, "acme-cert-cache", "acme-cert-cache", "directory in which certificates should be cached")
	flag.StringVar(&addr, "addr", ":443", "address to listen on")
	flag.StringVar(&geoipDatabase, "geoipdb", "/usr/share/tor/geoip", "path to correctly formatted geoip database mapping IPv4 address ranges to country codes")
	flag.StringVar(&geoip6Database, "geoip6db", "/usr/share/tor/geoip6", "path to correctly formatted geoip database mapping IPv6 address ranges to country codes")
	flag.StringVar(&bridgeListFilePath, "bridge-list-path", "", "file path for bridgeListFile")
	flag.StringVar(&allowedRelayPattern, "allowed-relay-pattern", "", "allowed pattern for relay host name")
	flag.StringVar(&presumedPatternForLegacyClient, "default-relay-pattern", "", "presumed pattern for legacy client")
	flag.BoolVar(&disableTLS, "disable-tls", false, "don't use HTTPS")
	flag.BoolVar(&disableGeoip, "disable-geoip", false, "don't use geoip for stats collection")
	flag.StringVar(&metricsFilename, "metrics-log", "", "path to metrics logging output")
	flag.StringVar(&ipCountFilename, "ip-count-log", "", "path to ip count logging output")
	flag.StringVar(&ipCountMaskingKey, "ip-count-mask", "", "masking key for ip count logging")
	flag.DurationVar(&ipCountInterval, "ip-count-interval", time.Hour, "time interval between each chunk")
	flag.BoolVar(&unsafeLogging, "unsafe-logging", false, "prevent logs from being scrubbed")
	flag.Parse()

	var err error
	var metricsFile io.Writer
	var logOutput io.Writer = os.Stderr
	if unsafeLogging {
		log.SetOutput(logOutput)
	} else {
		// We want to send the log output through our scrubber first
		log.SetOutput(&safelog.LogScrubber{Output: logOutput})
	}

	log.SetFlags(log.LstdFlags | log.LUTC)

	if metricsFilename != "" {
		metricsFile, err = os.OpenFile(metricsFilename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

		if err != nil {
			log.Fatal(err.Error())
		}
	} else {
		metricsFile = os.Stdout
	}

	metricsLogger := log.New(metricsFile, "", 0)

	ctx := NewBrokerContext(metricsLogger)

	if bridgeListFilePath != "" {
		bridgeListFile, err := os.Open(bridgeListFilePath)
		if err != nil {
			log.Fatal(err.Error())
		}
		err = ctx.InstallBridgeListProfile(bridgeListFile, allowedRelayPattern, presumedPatternForLegacyClient)
		if err != nil {
			log.Fatal(err.Error())
		}
	}

	if !disableGeoip {
		err = ctx.metrics.LoadGeoipDatabases(geoipDatabase, geoip6Database)
		if err != nil {
			log.Fatal(err.Error())
		}
	}

	if ipCountFilename != "" {
		ipCountFile, err := os.OpenFile(ipCountFilename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

		if err != nil {
			log.Fatal(err.Error())
		}
		ipSetSink := ipsetsink.NewIPSetSink(ipCountMaskingKey)
		ctx.metrics.distinctIPWriter = sinkcluster.NewClusterWriter(ipCountFile, ipCountInterval, ipSetSink)
	}

	go ctx.Broker()

	i := &IPC{ctx}

	http.HandleFunc("/robots.txt", robotsTxtHandler)

	http.Handle("/proxy", SnowflakeHandler{i, proxyPolls})
	http.Handle("/client", SnowflakeHandler{i, clientOffers})
	http.Handle("/answer", SnowflakeHandler{i, proxyAnswers})
	http.Handle("/debug", SnowflakeHandler{i, debugHandler})
	http.Handle("/metrics", MetricsHandler{metricsFilename, metricsHandler})
	http.Handle("/prometheus", promhttp.HandlerFor(ctx.metrics.promMetrics.registry, promhttp.HandlerOpts{}))

	http.Handle("/amp/client/", SnowflakeHandler{i, ampClientOffers})

	server := http.Server{
		Addr: addr,
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)

	// go routine to handle a SIGHUP signal to allow the broker operator to send
	// a SIGHUP signal when the geoip database files are updated, without requiring
	// a restart of the broker
	go func() {
		for {
			signal := <-sigChan
			log.Printf("Received signal: %s. Reloading geoip databases.", signal)
			if err = ctx.metrics.LoadGeoipDatabases(geoipDatabase, geoip6Database); err != nil {
				log.Fatalf("reload of Geo IP databases on signal %s returned error: %v", signal, err)
			}
		}
	}()

	// Handle the various ways of setting up TLS. The legal configurations
	// are:
	//   --acme-hostnames (with optional --acme-email and/or --acme-cert-cache)
	//   --cert and --key together
	//   --disable-tls
	// The outputs of this block of code are the disableTLS,
	// needHTTP01Listener, certManager, and getCertificate variables.
	if acmeHostnamesCommas != "" {
		acmeHostnames := strings.Split(acmeHostnamesCommas, ",")
		log.Printf("ACME hostnames: %q", acmeHostnames)

		var cache autocert.Cache
		if err = os.MkdirAll(acmeCertCacheDir, 0700); err != nil {
			log.Printf("Warning: Couldn't create cache directory %q (reason: %s) so we're *not* using our certificate cache.", acmeCertCacheDir, err)
		} else {
			cache = autocert.DirCache(acmeCertCacheDir)
		}

		certManager := autocert.Manager{
			Cache:      cache,
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(acmeHostnames...),
			Email:      acmeEmail,
		}
		go func() {
			log.Printf("Starting HTTP-01 listener")
			log.Fatal(http.ListenAndServe(":80", certManager.HTTPHandler(nil)))
		}()

		server.TLSConfig = &tls.Config{GetCertificate: certManager.GetCertificate}
		err = server.ListenAndServeTLS("", "")
	} else if certFilename != "" && keyFilename != "" {
		if acmeEmail != "" || acmeHostnamesCommas != "" {
			log.Fatalf("The --cert and --key options are not allowed with --acme-email or --acme-hostnames.")
		}
		err = server.ListenAndServeTLS(certFilename, keyFilename)
	} else if disableTLS {
		err = server.ListenAndServe()
	} else {
		log.Fatal("the --acme-hostnames, --cert and --key, or --disable-tls option is required")
	}

	if err != nil {
		log.Fatal(err)
	}
}
