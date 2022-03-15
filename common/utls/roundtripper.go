package utls

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// NewUTLSHTTPRoundTripper creates an instance of RoundTripper that dial to remote HTTPS endpoint with
// an alternative version of TLS implementation that attempts to imitate browsers' fingerprint.
// clientHelloID is the clientHello that uTLS attempts to imitate
// uTlsConfig is the TLS Configuration template
// backdropTransport is the transport that will be used for non-https traffic
// removeSNI indicates not to send Server Name Indication Extension
// returns a RoundTripper: its behaviour is documented at
// https://gitlab.torproject.org/tpo/anti-censorship/pluggable-transports/snowflake/-/merge_requests/76#note_2777161
func NewUTLSHTTPRoundTripper(clientHelloID utls.ClientHelloID, uTlsConfig *utls.Config,
	backdropTransport http.RoundTripper, removeSNI bool) http.RoundTripper {
	rtImpl := &uTLSHTTPRoundTripperImpl{
		clientHelloID:     clientHelloID,
		config:            uTlsConfig,
		connectWithH1:     map[string]bool{},
		backdropTransport: backdropTransport,
		pendingConn:       map[pendingConnKey]*unclaimedConnection{},
		removeSNI:         removeSNI,
	}
	rtImpl.init()
	return rtImpl
}

type uTLSHTTPRoundTripperImpl struct {
	clientHelloID utls.ClientHelloID
	config        *utls.Config

	accessConnectWithH1 sync.Mutex
	connectWithH1       map[string]bool

	httpsH1Transport  http.RoundTripper
	httpsH2Transport  http.RoundTripper
	backdropTransport http.RoundTripper

	accessDialingConnection sync.Mutex
	pendingConn             map[pendingConnKey]*unclaimedConnection

	removeSNI bool
}

type pendingConnKey struct {
	isH2 bool
	dest string
}

var errEAGAIN = errors.New("incorrect ALPN negotiated, try again with another ALPN")
var errEAGAINTooMany = errors.New("incorrect ALPN negotiated")
var errExpired = errors.New("connection have expired")

func (r *uTLSHTTPRoundTripperImpl) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return r.backdropTransport.RoundTrip(req)
	}
	for retryCount := 0; retryCount < 5; retryCount++ {
		if r.getShouldConnectWithH1(req.URL.Host) {
			resp, err := r.httpsH1Transport.RoundTrip(req)
			if errors.Is(err, errEAGAIN) {
				continue
			}
			return resp, err
		}
		resp, err := r.httpsH2Transport.RoundTrip(req)
		if errors.Is(err, errEAGAIN) {
			continue
		}
		return resp, err
	}
	return nil, errEAGAINTooMany
}

func (r *uTLSHTTPRoundTripperImpl) getShouldConnectWithH1(domainName string) bool {
	r.accessConnectWithH1.Lock()
	defer r.accessConnectWithH1.Unlock()
	if value, set := r.connectWithH1[domainName]; set {
		return value
	}
	return false
}

func (r *uTLSHTTPRoundTripperImpl) setShouldConnectWithH1(domainName string) {
	r.accessConnectWithH1.Lock()
	defer r.accessConnectWithH1.Unlock()
	r.connectWithH1[domainName] = true
}

func (r *uTLSHTTPRoundTripperImpl) clearShouldConnectWithH1(domainName string) {
	r.accessConnectWithH1.Lock()
	defer r.accessConnectWithH1.Unlock()
	r.connectWithH1[domainName] = false
}

func getPendingConnectionID(dest string, alpnIsH2 bool) pendingConnKey {
	return pendingConnKey{isH2: alpnIsH2, dest: dest}
}

func (r *uTLSHTTPRoundTripperImpl) putConn(addr string, alpnIsH2 bool, conn net.Conn) {
	connId := getPendingConnectionID(addr, alpnIsH2)
	r.pendingConn[connId] = NewUnclaimedConnection(conn, time.Minute)
}
func (r *uTLSHTTPRoundTripperImpl) getConn(addr string, alpnIsH2 bool) net.Conn {
	connId := getPendingConnectionID(addr, alpnIsH2)
	if conn, ok := r.pendingConn[connId]; ok {
		delete(r.pendingConn, connId)
		if claimedConnection, err := conn.claimConnection(); err == nil {
			return claimedConnection
		}
	}
	return nil
}
func (r *uTLSHTTPRoundTripperImpl) dialOrGetTLSWithExpectedALPN(ctx context.Context, addr string, expectedH2 bool) (net.Conn, error) {
	r.accessDialingConnection.Lock()
	defer r.accessDialingConnection.Unlock()

	if r.getShouldConnectWithH1(addr) == expectedH2 {
		return nil, errEAGAIN
	}

	//Get a cached connection if possible to reduce preflight connection closed without sending data
	if gconn := r.getConn(addr, expectedH2); gconn != nil {
		return gconn, nil
	}

	conn, err := r.dialTLS(ctx, addr)
	if err != nil {
		return nil, err
	}

	protocol := conn.ConnectionState().NegotiatedProtocol

	protocolIsH2 := protocol == http2.NextProtoTLS

	if protocolIsH2 == expectedH2 {
		return conn, err
	}

	r.putConn(addr, protocolIsH2, conn)

	if protocolIsH2 {
		r.clearShouldConnectWithH1(addr)
	} else {
		r.setShouldConnectWithH1(addr)
	}

	return nil, errEAGAIN
}

// based on https://repo.or.cz/dnstt.git/commitdiff/d92a791b6864901f9263f7d73d97cfd30ac53b09..98bdffa1706dfc041d1e99b86c47f29d72ad3a0c
// by dcf1
func (r *uTLSHTTPRoundTripperImpl) dialTLS(ctx context.Context, addr string) (*utls.UConn, error) {
	config := r.config.Clone()

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	config.ServerName = host

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	uconn := utls.UClient(conn, config, r.clientHelloID)
	if (net.ParseIP(config.ServerName) != nil) || r.removeSNI {
		err := uconn.RemoveSNIExtension()
		if err != nil {
			uconn.Close()
			return nil, err
		}
	}

	err = uconn.Handshake()
	if err != nil {
		return nil, err
	}
	return uconn, nil
}

func (r *uTLSHTTPRoundTripperImpl) init() {
	r.httpsH2Transport = &http2.Transport{
		DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
			return r.dialOrGetTLSWithExpectedALPN(context.Background(), addr, true)
		},
	}
	r.httpsH1Transport = &http.Transport{
		DialTLSContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
			return r.dialOrGetTLSWithExpectedALPN(ctx, addr, false)
		},
	}
}

func NewUnclaimedConnection(conn net.Conn, expireTime time.Duration) *unclaimedConnection {
	c := &unclaimedConnection{
		Conn: conn,
	}
	time.AfterFunc(expireTime, c.tick)
	return c
}

type unclaimedConnection struct {
	net.Conn
	claimed bool
	access  sync.Mutex
}

func (c *unclaimedConnection) claimConnection() (net.Conn, error) {
	c.access.Lock()
	defer c.access.Unlock()
	if !c.claimed {
		c.claimed = true
		return c.Conn, nil
	}
	return nil, errExpired
}

func (c *unclaimedConnection) tick() {
	c.access.Lock()
	defer c.access.Unlock()
	if !c.claimed {
		c.claimed = true
		c.Conn.Close()
		c.Conn = nil
	}
}
