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

	"github.com/keroserene/go-webrtc"
	"golang.org/x/net/websocket"
)

const defaultBrokerURL = "https://snowflake-broker.bamsoftware.com/"
const defaultRelayURL = "wss://snowflake.bamsoftware.com/"
const defaultSTUNURL = "stun:stun.l.google.com:19302"
const pollInterval = 5 * time.Second
//amount of time after sending an SDP answer before the proxy assumes the
//client is not going to connect
const dataChannelTimeout = time.Minute

var brokerURL *url.URL
var relayURL string

const (
	sessionIDLength = 16
)

var (
	tokens chan bool
	config *webrtc.Configuration
	client http.Client
)

var remoteIPPatterns = []*regexp.Regexp{
	/* IPv4 */
	regexp.MustCompile(`(?m)^c=IN IP4 ([\d.]+)(?:(?:\/\d+)?\/\d+)?(:? |\r?\n)`),
	/* IPv6 */
	regexp.MustCompile(`(?m)^c=IN IP6 ([0-9A-Fa-f:.]+)(?:\/\d+)?(:? |\r?\n)`),
}

// https://tools.ietf.org/html/rfc4566#section-5.7
func remoteIPFromSDP(sdp string) net.IP {
	for _, pattern := range remoteIPPatterns {
		m := pattern.FindStringSubmatch(sdp)
		if m != nil {
			// Ignore parsing errors, ParseIP returns nil.
			return net.ParseIP(m[1])
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
}

func (c *webRTCConn) Read(b []byte) (int, error) {
	return c.pr.Read(b)
}

func (c *webRTCConn) Write(b []byte) (int, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	// log.Printf("webrtc Write %d %+q", len(b), string(b))
	log.Printf("Write %d bytes --> WebRTC", len(b))
	if c.dc != nil {
		c.dc.Send(b)
	}
	return len(b), nil
}

func (c *webRTCConn) Close() (err error) {
	c.once.Do(func() {
		err = c.pc.Destroy()
	})
	return
}

func (c *webRTCConn) LocalAddr() net.Addr {
	return nil
}

func (c *webRTCConn) RemoteAddr() net.Addr {
	//Parse Remote SDP offer and extract client IP
	clientIP := remoteIPFromSDP(c.pc.RemoteDescription().Sdp)
	if clientIP == nil {
		return nil
	}
	return &net.IPAddr{IP: clientIP, Zone: ""}
}

func (c *webRTCConn) SetDeadline(t time.Time) error {
	return fmt.Errorf("SetDeadline not implemented")
}

func (c *webRTCConn) SetReadDeadline(t time.Time) error {
	return fmt.Errorf("SetReadDeadline not implemented")
}

func (c *webRTCConn) SetWriteDeadline(t time.Time) error {
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

func pollOffer(sid string) *webrtc.SessionDescription {
	broker := brokerURL.ResolveReference(&url.URL{Path: "proxy"})
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

		req, _ := http.NewRequest("POST", broker.String(), bytes.NewBuffer([]byte(sid)))
		req.Header.Set("X-Session-ID", sid)
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("error polling broker: %s", err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				log.Printf("broker returns: %d", resp.StatusCode)
			} else {
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					log.Printf("error reading broker response: %s", err)
				} else {
					return webrtc.DeserializeSessionDescription(string(body))
				}
			}
		}
	}
}

func sendAnswer(sid string, pc *webrtc.PeerConnection) error {
	broker := brokerURL.ResolveReference(&url.URL{Path: "answer"})
	body := bytes.NewBuffer([]byte(pc.LocalDescription().Serialize()))
	req, _ := http.NewRequest("POST", broker.String(), body)
	req.Header.Set("X-Session-ID", sid)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("broker returned %d", resp.StatusCode)
	}
	return nil
}

type timeoutConn struct {
	c net.Conn
	t time.Duration
}

func (tc timeoutConn) Read(buf []byte) (int, error) {
	tc.c.SetDeadline(time.Now().Add(tc.t))
	return tc.c.Read(buf)
}

func (tc timeoutConn) Write(buf []byte) (int, error) {
	tc.c.SetDeadline(time.Now().Add(tc.t))
	return tc.c.Write(buf)
}

func (tc timeoutConn) Close() error {
	return tc.c.Close()
}

func CopyLoopTimeout(c1 net.Conn, c2 net.Conn, timeout time.Duration) {
	tc1 := timeoutConn{c: c1, t: timeout}
	tc2 := timeoutConn{c: c2, t: timeout}
	var wg sync.WaitGroup
	copyer := func(dst io.ReadWriteCloser, src io.ReadWriteCloser) {
		defer wg.Done()
		io.Copy(dst, src)
		dst.Close()
		src.Close()
	}
	wg.Add(2)
	go copyer(tc1, tc2)
	go copyer(tc2, tc1)
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

	// Retrieve client IP address
	if remoteAddr != nil {
		// Encode client IP address in relay URL
		q := u.Query()
		clientIP := remoteAddr.String()
		q.Set("client_ip", clientIP)
		u.RawQuery = q.Encode()
	} else {
		log.Printf("no remote address given in websocket")
	}

	wsConn, err := websocket.Dial(u.String(), "", relayURL)
	if err != nil {
		log.Printf("error dialing relay: %s", err)
		return
	}
	log.Printf("connected to relay")
	defer wsConn.Close()
	wsConn.PayloadType = websocket.BinaryFrame
	CopyLoopTimeout(conn, wsConn, time.Minute)
	log.Printf("datachannelHandler ends")
}

// Create a PeerConnection from an SDP offer. Blocks until the gathering of ICE
// candidates is complete and the answer is available in LocalDescription.
// Installs an OnDataChannel callback that creates a webRTCConn and passes it to
// datachannelHandler.
func makePeerConnectionFromOffer(sdp *webrtc.SessionDescription, config *webrtc.Configuration) (*webrtc.PeerConnection, error) {

	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("accept: NewPeerConnection: %s", err)
	}
	pc.OnNegotiationNeeded = func() {
		panic("OnNegotiationNeeded")
	}
	pc.OnDataChannel = func(dc *webrtc.DataChannel) {
		log.Println("OnDataChannel")

		pr, pw := io.Pipe()
		conn := &webRTCConn{pc: pc, dc: dc, pr: pr}

		dc.OnOpen = func() {
			log.Println("OnOpen channel")
		}
		dc.OnClose = func() {
			conn.lock.Lock()
			defer conn.lock.Unlock()
			log.Println("OnClose channel")
			conn.dc = nil
			pc.DeleteDataChannel(dc)
			pw.Close()
		}
		dc.OnMessage = func(msg []byte) {
			log.Printf("OnMessage <--- %d bytes", len(msg))
			n, err := pw.Write(msg)
			if err != nil {
				pw.CloseWithError(err)
			}
			if n != len(msg) {
				panic("short write")
			}
		}

		go datachannelHandler(conn, conn.RemoteAddr())
	}

	err = pc.SetRemoteDescription(sdp)
	if err != nil {
		pc.Destroy()
		return nil, fmt.Errorf("accept: SetRemoteDescription: %s", err)
	}
	log.Println("sdp offer successfully received.")

	log.Println("Generating answer...")
	answer, err := pc.CreateAnswer()
	// blocks on ICE gathering. we need to add a timeout if needed
	// not putting this in a separate go routine, because we need
	// SetLocalDescription(answer) to be called before sendAnswer
	if err != nil {
		pc.Destroy()
		return nil, err
	}

	if answer == nil {
		pc.Destroy()
		return nil, fmt.Errorf("Failed gathering ICE candidates.")
	}

	err = pc.SetLocalDescription(answer)
	if err != nil {
		pc.Destroy()
		return nil, err
	}

	// Set a timeout on peerconnection. If the connection state has not
	// advanced to PeerConnectionStateConnected in this time,
	// destroy the peer connection and return the token.
	go func() {
		<-time.After(dataChannelTimeout)
		if pc.ConnectionState() != webrtc.PeerConnectionStateConnected {
			log.Println("Timed out waiting for client to open data cannel.")
			pc.Destroy()
			retToken()
		}

	}()

	return pc, nil
}

func runSession(sid string) {
	offer := pollOffer(sid)
	if offer == nil {
		log.Printf("bad offer from broker")
		retToken()
		return
	}
	pc, err := makePeerConnectionFromOffer(offer, config)
	if err != nil {
		log.Printf("error making WebRTC connection: %s", err)
		retToken()
		return
	}
	err = sendAnswer(sid, pc)
	if err != nil {
		log.Printf("error sending answer to client through broker: %s", err)
		pc.Destroy()
		retToken()
		return
	}
}

func main() {
	var capacity uint
	var stunURL string
	var logFilename string
	var rawBrokerURL string

	flag.UintVar(&capacity, "capacity", 10, "maximum concurrent clients")
	flag.StringVar(&rawBrokerURL, "broker", defaultBrokerURL, "broker URL")
	flag.StringVar(&relayURL, "relay", defaultRelayURL, "websocket relay URL")
	flag.StringVar(&stunURL, "stun", defaultSTUNURL, "stun URL")
	flag.StringVar(&logFilename, "log", "", "log filename")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.LUTC)
	if logFilename != "" {
		f, err := os.OpenFile(logFilename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	}

	log.Println("starting")

	var err error
	brokerURL, err = url.Parse(rawBrokerURL)
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

	config = webrtc.NewConfiguration(webrtc.OptionIceServer(stunURL))
	tokens = make(chan bool, capacity)
	for i := uint(0); i < capacity; i++ {
		tokens <- true
	}

	for {
		getToken()
		sessionID := genSessionID()
		runSession(sessionID)
	}
}
