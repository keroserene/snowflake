package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/common/messages"
	"git.torproject.org/pluggable-transports/snowflake.git/common/safelog"
	"git.torproject.org/pluggable-transports/snowflake.git/common/util"
	"git.torproject.org/pluggable-transports/snowflake.git/common/websocketconn"
	"github.com/gorilla/websocket"
	"github.com/pion/ice/v2"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

const defaultBrokerURL = "https://snowflake-broker.bamsoftware.com/"
const defaultProbeURL = "https://snowflake-broker.torproject.net:8443/probe"
const defaultRelayURL = "wss://snowflake.bamsoftware.com/"
const defaultSTUNURL = "stun:stun.stunprotocol.org:3478"
const pollInterval = 5 * time.Second
const (
	NATUnknown      = "unknown"
	NATRestricted   = "restricted"
	NATUnrestricted = "unrestricted"
)

//amount of time after sending an SDP answer before the proxy assumes the
//client is not going to connect
const dataChannelTimeout = 20 * time.Second

const readLimit = 100000 //Maximum number of bytes to be read from an HTTP request

var broker *SignalingServer
var relayURL string

var currentNATType = NATUnknown

const (
	sessionIDLength = 16
)

var (
	tokens chan bool
	config webrtc.Configuration
	client http.Client
)

var remoteIPPatterns = []*regexp.Regexp{
	/* IPv4 */
	regexp.MustCompile(`(?m)^c=IN IP4 ([\d.]+)(?:(?:\/\d+)?\/\d+)?(:? |\r?\n)`),
	/* IPv6 */
	regexp.MustCompile(`(?m)^c=IN IP6 ([0-9A-Fa-f:.]+)(?:\/\d+)?(:? |\r?\n)`),
}

// Checks whether an IP address is a remote address for the client
func isRemoteAddress(ip net.IP) bool {
	return !(util.IsLocal(ip) || ip.IsUnspecified() || ip.IsLoopback())
}

func remoteIPFromSDP(str string) net.IP {
	// Look for remote IP in "a=candidate" attribute fields
	// https://tools.ietf.org/html/rfc5245#section-15.1
	var desc sdp.SessionDescription
	err := desc.Unmarshal([]byte(str))
	if err != nil {
		log.Println("Error parsing SDP: ", err.Error())
		return nil
	}
	for _, m := range desc.MediaDescriptions {
		for _, a := range m.Attributes {
			if a.IsICECandidate() {
				c, err := ice.UnmarshalCandidate(a.Value)
				if err == nil {
					ip := net.ParseIP(c.Address())
					if ip != nil && isRemoteAddress(ip) {
						return ip
					}
				}
			}
		}
	}
	// Finally look for remote IP in "c=" Connection Data field
	// https://tools.ietf.org/html/rfc4566#section-5.7
	for _, pattern := range remoteIPPatterns {
		m := pattern.FindStringSubmatch(str)
		if m != nil {
			// Ignore parsing errors, ParseIP returns nil.
			ip := net.ParseIP(m[1])
			if ip != nil && isRemoteAddress(ip) {
				return ip
			}

		}
	}

	return nil
}

type webRTCConn struct {
	dc *webrtc.DataChannel
	pc *webrtc.PeerConnection
	pr *io.PipeReader

	lock sync.Mutex // Synchronization for DataChannel destruction
	once sync.Once  // Synchronization for PeerConnection destruction

	bytesLogger BytesLogger
}

func (c *webRTCConn) Read(b []byte) (int, error) {
	return c.pr.Read(b)
}

func (c *webRTCConn) Write(b []byte) (int, error) {
	c.bytesLogger.AddInbound(len(b))
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.dc != nil {
		c.dc.Send(b)
	}
	return len(b), nil
}

func (c *webRTCConn) Close() (err error) {
	c.once.Do(func() {
		err = c.pc.Close()
	})
	return
}

func (c *webRTCConn) LocalAddr() net.Addr {
	return nil
}

func (c *webRTCConn) RemoteAddr() net.Addr {
	//Parse Remote SDP offer and extract client IP
	clientIP := remoteIPFromSDP(c.pc.RemoteDescription().SDP)
	if clientIP == nil {
		return nil
	}
	return &net.IPAddr{IP: clientIP, Zone: ""}
}

func (c *webRTCConn) SetDeadline(t time.Time) error {
	// nolint: golint
	return fmt.Errorf("SetDeadline not implemented")
}

func (c *webRTCConn) SetReadDeadline(t time.Time) error {
	// nolint: golint
	return fmt.Errorf("SetReadDeadline not implemented")
}

func (c *webRTCConn) SetWriteDeadline(t time.Time) error {
	// nolint: golint
	return fmt.Errorf("SetWriteDeadline not implemented")
}

func getToken() {
	<-tokens
}

func retToken() {
	tokens <- true
}

func genSessionID() string {
	buf := make([]byte, sessionIDLength)
	_, err := rand.Read(buf)
	if err != nil {
		panic(err.Error())
	}
	return strings.TrimRight(base64.StdEncoding.EncodeToString(buf), "=")
}

func limitedRead(r io.Reader, limit int64) ([]byte, error) {
	p, err := ioutil.ReadAll(&io.LimitedReader{R: r, N: limit + 1})
	if err != nil {
		return p, err
	} else if int64(len(p)) == limit+1 {
		return p[0:limit], io.ErrUnexpectedEOF
	}
	return p, err
}

type SignalingServer struct {
	url                *url.URL
	transport          http.RoundTripper
	keepLocalAddresses bool
}

func (s *SignalingServer) Post(path string, payload io.Reader) ([]byte, error) {

	req, err := http.NewRequest("POST", path, payload)
	if err != nil {
		return nil, err
	}
	resp, err := s.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote returned status code %d", resp.StatusCode)
	}

	defer resp.Body.Close()
	return limitedRead(resp.Body, readLimit)
}

func (s *SignalingServer) pollOffer(sid string) *webrtc.SessionDescription {
	brokerPath := s.url.ResolveReference(&url.URL{Path: "proxy"})
	timeOfNextPoll := time.Now()
	for {
		// Sleep until we're scheduled to poll again.
		now := time.Now()
		time.Sleep(timeOfNextPoll.Sub(now))
		// Compute the next time to poll -- if it's in the past, that
		// means that the POST took longer than pollInterval, so we're
		// allowed to do another one immediately.
		timeOfNextPoll = timeOfNextPoll.Add(pollInterval)
		if timeOfNextPoll.Before(now) {
			timeOfNextPoll = now
		}

		body, err := messages.EncodePollRequest(sid, "standalone", currentNATType)
		if err != nil {
			log.Printf("Error encoding poll message: %s", err.Error())
			return nil
		}
		resp, err := s.Post(brokerPath.String(), bytes.NewBuffer(body))
		if err != nil {
			log.Printf("error polling broker: %s", err.Error())
		}

		offer, _, err := messages.DecodePollResponse(resp)
		if err != nil {
			log.Printf("Error reading broker response: %s", err.Error())
			log.Printf("body: %s", resp)
			return nil
		}
		if offer != "" {
			offer, err := util.DeserializeSessionDescription(offer)
			if err != nil {
				log.Printf("Error processing session description: %s", err.Error())
				return nil
			}
			return offer

		}
	}
}

func (s *SignalingServer) sendAnswer(sid string, pc *webrtc.PeerConnection) error {
	brokerPath := s.url.ResolveReference(&url.URL{Path: "answer"})
	ld := pc.LocalDescription()
	if !s.keepLocalAddresses {
		ld = &webrtc.SessionDescription{
			Type: ld.Type,
			SDP:  util.StripLocalAddresses(ld.SDP),
		}
	}
	answer, err := util.SerializeSessionDescription(ld)
	if err != nil {
		return err
	}
	body, err := messages.EncodeAnswerRequest(answer, sid)
	if err != nil {
		return err
	}
	resp, err := s.Post(brokerPath.String(), bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("error sending answer to broker: %s", err.Error())
	}

	success, err := messages.DecodeAnswerResponse(resp)
	if err != nil {
		return err
	}
	if !success {
		return fmt.Errorf("broker returned client timeout")
	}

	return nil
}

func CopyLoop(c1 io.ReadWriteCloser, c2 io.ReadWriteCloser) {
	var wg sync.WaitGroup
	copyer := func(dst io.ReadWriteCloser, src io.ReadWriteCloser) {
		defer wg.Done()
		// Ignore io.ErrClosedPipe because it is likely caused by the
		// termination of copyer in the other direction.
		if _, err := io.Copy(dst, src); err != nil && err != io.ErrClosedPipe {
			log.Printf("io.Copy inside CopyLoop generated an error: %v", err)
		}
		dst.Close()
		src.Close()
	}
	wg.Add(2)
	go copyer(c1, c2)
	go copyer(c2, c1)
	wg.Wait()
}

// We pass conn.RemoteAddr() as an additional parameter, rather than calling
// conn.RemoteAddr() inside this function, as a workaround for a hang that
// otherwise occurs inside of conn.pc.RemoteDescription() (called by
// RemoteAddr). https://bugs.torproject.org/18628#comment:8
func datachannelHandler(conn *webRTCConn, remoteAddr net.Addr) {
	defer conn.Close()
	defer retToken()

	u, err := url.Parse(relayURL)
	if err != nil {
		log.Fatalf("invalid relay url: %s", err)
	}

	if remoteAddr != nil {
		// Encode client IP address in relay URL
		q := u.Query()
		clientIP := remoteAddr.String()
		q.Set("client_ip", clientIP)
		u.RawQuery = q.Encode()
	} else {
		log.Printf("no remote address given in websocket")
	}

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Printf("error dialing relay: %s", err)
		return
	}
	wsConn := websocketconn.New(ws)
	log.Printf("connected to relay")
	defer wsConn.Close()
	CopyLoop(conn, wsConn)
	log.Printf("datachannelHandler ends")
}

// Create a PeerConnection from an SDP offer. Blocks until the gathering of ICE
// candidates is complete and the answer is available in LocalDescription.
// Installs an OnDataChannel callback that creates a webRTCConn and passes it to
// datachannelHandler.
func makePeerConnectionFromOffer(sdp *webrtc.SessionDescription,
	config webrtc.Configuration,
	dataChan chan struct{},
	handler func(conn *webRTCConn, remoteAddr net.Addr)) (*webrtc.PeerConnection, error) {

	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("accept: NewPeerConnection: %s", err)
	}
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		log.Println("OnDataChannel")
		close(dataChan)

		pr, pw := io.Pipe()
		conn := &webRTCConn{pc: pc, dc: dc, pr: pr}
		conn.bytesLogger = NewBytesSyncLogger()

		dc.OnOpen(func() {
			log.Println("OnOpen channel")
		})
		dc.OnClose(func() {
			conn.lock.Lock()
			defer conn.lock.Unlock()
			log.Println("OnClose channel")
			log.Println(conn.bytesLogger.ThroughputSummary())
			conn.dc = nil
			dc.Close()
			pw.Close()
		})
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			var n int
			n, err = pw.Write(msg.Data)
			if err != nil {
				if inerr := pw.CloseWithError(err); inerr != nil {
					log.Printf("close with error generated an error: %v", inerr)
				}
			}
			conn.bytesLogger.AddOutbound(n)
			if n != len(msg.Data) {
				panic("short write")
			}
		})

		go handler(conn, conn.RemoteAddr())
	})
	// As of v3.0.0, pion-webrtc uses trickle ICE by default.
	// We have to wait for candidate gathering to complete
	// before we send the offer
	done := webrtc.GatheringCompletePromise(pc)
	err = pc.SetRemoteDescription(*sdp)
	if err != nil {
		if inerr := pc.Close(); inerr != nil {
			log.Printf("unable to call pc.Close after pc.SetRemoteDescription with error: %v", inerr)
		}
		return nil, fmt.Errorf("accept: SetRemoteDescription: %s", err)
	}
	log.Println("sdp offer successfully received.")

	log.Println("Generating answer...")
	answer, err := pc.CreateAnswer(nil)
	// blocks on ICE gathering. we need to add a timeout if needed
	// not putting this in a separate go routine, because we need
	// SetLocalDescription(answer) to be called before sendAnswer
	if err != nil {
		if inerr := pc.Close(); inerr != nil {
			log.Printf("ICE gathering has generated an error when calling pc.Close: %v", inerr)
		}
		return nil, err
	}

	err = pc.SetLocalDescription(answer)
	if err != nil {
		if err = pc.Close(); err != nil {
			log.Printf("pc.Close after setting local description returned : %v", err)
		}
		return nil, err
	}
	// Wait for ICE candidate gathering to complete
	<-done
	return pc, nil
}

// Create a new PeerConnection. Blocks until the gathering of ICE
// candidates is complete and the answer is available in LocalDescription.
func makeNewPeerConnection(config webrtc.Configuration,
	dataChan chan struct{}) (*webrtc.PeerConnection, error) {

	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("accept: NewPeerConnection: %s", err)
	}

	// Must create a data channel before creating an offer
	// https://github.com/pion/webrtc/wiki/Release-WebRTC@v3.0.0
	dc, err := pc.CreateDataChannel("test", &webrtc.DataChannelInit{})
	if err != nil {
		log.Printf("CreateDataChannel ERROR: %s", err)
		return nil, err
	}
	dc.OnOpen(func() {
		log.Println("WebRTC: DataChannel.OnOpen")
		close(dataChan)
	})
	dc.OnClose(func() {
		log.Println("WebRTC: DataChannel.OnClose")
		dc.Close()
	})

	offer, err := pc.CreateOffer(nil)
	// TODO: Potentially timeout and retry if ICE isn't working.
	if err != nil {
		log.Println("Failed to prepare offer", err)
		pc.Close()
		return nil, err
	}
	log.Println("WebRTC: Created offer")

	// As of v3.0.0, pion-webrtc uses trickle ICE by default.
	// We have to wait for candidate gathering to complete
	// before we send the offer
	done := webrtc.GatheringCompletePromise(pc)
	err = pc.SetLocalDescription(offer)
	if err != nil {
		log.Println("Failed to prepare offer", err)
		pc.Close()
		return nil, err
	}
	log.Println("WebRTC: Set local description")

	// Wait for ICE candidate gathering to complete
	<-done
	return pc, nil
}

func runSession(sid string) {
	offer := broker.pollOffer(sid)
	if offer == nil {
		log.Printf("bad offer from broker")
		retToken()
		return
	}
	dataChan := make(chan struct{})
	pc, err := makePeerConnectionFromOffer(offer, config, dataChan, datachannelHandler)
	if err != nil {
		log.Printf("error making WebRTC connection: %s", err)
		retToken()
		return
	}
	err = broker.sendAnswer(sid, pc)
	if err != nil {
		log.Printf("error sending answer to client through broker: %s", err)
		if inerr := pc.Close(); inerr != nil {
			log.Printf("error calling pc.Close: %v", inerr)
		}
		retToken()
		return
	}
	// Set a timeout on peerconnection. If the connection state has not
	// advanced to PeerConnectionStateConnected in this time,
	// destroy the peer connection and return the token.
	select {
	case <-dataChan:
		log.Println("Connection successful.")
	case <-time.After(dataChannelTimeout):
		log.Println("Timed out waiting for client to open data channel.")
		if err := pc.Close(); err != nil {
			log.Printf("error calling pc.Close: %v", err)
		}
		retToken()
	}
}

func main() {
	var capacity uint
	var stunURL string
	var logFilename string
	var rawBrokerURL string
	var unsafeLogging bool
	var keepLocalAddresses bool

	flag.UintVar(&capacity, "capacity", 10, "maximum concurrent clients")
	flag.StringVar(&rawBrokerURL, "broker", defaultBrokerURL, "broker URL")
	flag.StringVar(&relayURL, "relay", defaultRelayURL, "websocket relay URL")
	flag.StringVar(&stunURL, "stun", defaultSTUNURL, "stun URL")
	flag.StringVar(&logFilename, "log", "", "log filename")
	flag.BoolVar(&unsafeLogging, "unsafe-logging", false, "prevent logs from being scrubbed")
	flag.BoolVar(&keepLocalAddresses, "keep-local-addresses", false, "keep local LAN address ICE candidates")
	flag.Parse()

	var logOutput io.Writer = os.Stderr
	log.SetFlags(log.LstdFlags | log.LUTC)
	if logFilename != "" {
		f, err := os.OpenFile(logFilename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		logOutput = io.MultiWriter(os.Stderr, f)
	}
	if unsafeLogging {
		log.SetOutput(logOutput)
	} else {
		// We want to send the log output through our scrubber first
		log.SetOutput(&safelog.LogScrubber{Output: logOutput})
	}

	log.Println("starting")

	var err error
	broker = new(SignalingServer)
	broker.keepLocalAddresses = keepLocalAddresses
	broker.url, err = url.Parse(rawBrokerURL)
	if err != nil {
		log.Fatalf("invalid broker url: %s", err)
	}
	_, err = url.Parse(stunURL)
	if err != nil {
		log.Fatalf("invalid stun url: %s", err)
	}
	_, err = url.Parse(relayURL)
	if err != nil {
		log.Fatalf("invalid relay url: %s", err)
	}

	broker.transport = http.DefaultTransport.(*http.Transport)
	broker.transport.(*http.Transport).ResponseHeaderTimeout = 15 * time.Second
	config = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{stunURL},
			},
		},
	}
	tokens = make(chan bool, capacity)
	for i := uint(0); i < capacity; i++ {
		tokens <- true
	}

	// use probetest to determine NAT compatability
	checkNATType(config, defaultProbeURL)
	log.Printf("NAT type: %s", currentNATType)

	for {
		getToken()
		sessionID := genSessionID()
		runSession(sessionID)
	}
}

func checkNATType(config webrtc.Configuration, probeURL string) {

	var err error

	probe := new(SignalingServer)
	probe.transport = http.DefaultTransport.(*http.Transport)
	probe.transport.(*http.Transport).ResponseHeaderTimeout = 30 * time.Second
	probe.url, err = url.Parse(probeURL)
	if err != nil {
		log.Printf("Error parsing url: %s", err.Error())
	}

	// create offer
	dataChan := make(chan struct{})
	pc, err := makeNewPeerConnection(config, dataChan)
	if err != nil {
		log.Printf("error making WebRTC connection: %s", err)
		return
	}

	offer := pc.LocalDescription()
	sdp, err := util.SerializeSessionDescription(offer)
	log.Printf("Offer: %s", sdp)
	if err != nil {
		log.Printf("Error encoding probe message: %s", err.Error())
		return
	}

	// send offer
	body, err := messages.EncodePollResponse(sdp, true, "")
	if err != nil {
		log.Printf("Error encoding probe message: %s", err.Error())
		return
	}
	resp, err := probe.Post(probe.url.String(), bytes.NewBuffer(body))
	if err != nil {
		log.Printf("error polling probe: %s", err.Error())
		return
	}

	sdp, _, err = messages.DecodeAnswerRequest(resp)
	if err != nil {
		log.Printf("Error reading probe response: %s", err.Error())
		return
	}
	answer, err := util.DeserializeSessionDescription(sdp)
	if err != nil {
		log.Printf("Error setting answer: %s", err.Error())
		return
	}
	err = pc.SetRemoteDescription(*answer)
	if err != nil {
		log.Printf("Error setting answer: %s", err.Error())
		return
	}

	select {
	case <-dataChan:
		currentNATType = NATUnrestricted
	case <-time.After(dataChannelTimeout):
		currentNATType = NATRestricted
	}
	if err := pc.Close(); err != nil {
		log.Printf("error calling pc.Close: %v", err)
	}

}
