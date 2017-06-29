package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/keroserene/go-webrtc"
	"golang.org/x/net/websocket"
)

type snowflakeOptions struct {
	capacity  int
	broker    string
	brokerURL *url.URL
	stun      string
	stunURL   *url.URL
	relay     string
	relayURL  *url.URL
}

const (
	sessionIDLength = 16
)

var (
	tokens chan bool
	opt    *snowflakeOptions
	config *webrtc.Configuration
	client http.Client
)

type webRTCConn struct {
	dc *webrtc.DataChannel
	pc *webrtc.PeerConnection
	pr *io.PipeReader
}

func (c *webRTCConn) Read(b []byte) (int, error) {
	return c.pr.Read(b)
}

func (c *webRTCConn) Write(b []byte) (int, error) {
	// log.Printf("webrtc Write %d %+q", len(b), string(b))
	log.Printf("Write %d bytes --> WebRTC", len(b))
	c.dc.Send(b)
	return len(b), nil
}

func (c *webRTCConn) Close() error {
	return c.pc.Close()
}

func (c *webRTCConn) LocalAddr() net.Addr {
	return nil
}

func (c *webRTCConn) RemoteAddr() net.Addr {
	return nil
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

func pollOffer(sid string, broker *url.URL) *webrtc.SessionDescription {
	broker.Path = "/proxy"
	for {
		req, _ := http.NewRequest("POST", broker.String(), bytes.NewBuffer([]byte(sid)))
		req.Header.Set("X-Session-ID", sid)
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("error to poll broker: %s", err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				log.Printf("broker returns: %d", resp.StatusCode)
			} else {
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					log.Printf("error to read broker responses: %s", err)
				} else {
					return webrtc.DeserializeSessionDescription(string(body))
				}
			}
		}
	}
}

func sendAnswer(sid string, broker *url.URL, pc *webrtc.PeerConnection) error {
	broker.Path = "/answer"
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

func datachannelHandler(conn *webRTCConn) {
	defer conn.Close()
	defer retToken()

	wsConn, err := websocket.Dial(opt.relay, "", opt.relay)
	if err != nil {
		log.Printf("error to dial relay: %s", err)
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
	errChan := make(chan error)
	answerChan := make(chan struct{})

	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("accept: NewPeerConnection: %s", err)
	}
	pc.OnNegotiationNeeded = func() {
		panic("OnNegotiationNeeded")
	}
	pc.OnIceComplete = func() {
		answerChan <- struct{}{}
	}
	pc.OnDataChannel = func(dc *webrtc.DataChannel) {
		log.Println("OnDataChannel")

		pr, pw := io.Pipe()

		dc.OnOpen = func() {
			log.Println("OnOpen channel")
		}
		dc.OnClose = func() {
			log.Println("OnClose channel")
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
		conn := &webRTCConn{pc: pc, dc: dc, pr: pr}
		go datachannelHandler(conn)
	}

	err = pc.SetRemoteDescription(sdp)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("accept: SetRemoteDescription: %s", err)
	}
	log.Println("sdp offer successfully received.")

	go func() {
		log.Println("Generating answer...")
		answer, err := pc.CreateAnswer() // blocking
		if err != nil {
			errChan <- err
			return
		}
		err = pc.SetLocalDescription(answer)
		if err != nil {
			errChan <- err
			return
		}
	}()

	// Wait until answer is ready.
	select {
	case err = <-errChan:
		pc.Close()
		return nil, err
	case _, ok := <-answerChan:
		if !ok {
			pc.Close()
			return nil, fmt.Errorf("Failed gathering ICE candidates.")
		}
	}
	return pc, nil
}

func runSession(sid string) {
	offer := pollOffer(sid, opt.brokerURL)
	if offer == nil {
		log.Printf("bad offer from broker")
		retToken()
		return
	}
	pc, err := makePeerConnectionFromOffer(offer, config)
	if err != nil {
		log.Printf("error to make WebRTC connection: %s", err)
		retToken()
		return
	}
	err = sendAnswer(sid, opt.brokerURL, pc)
	if err != nil {
		log.Printf("error to send answer to client through broker: %s", err)
		pc.Close()
		retToken()
		return
	}
}

func main() {
	var logFilename string
	opt = new(snowflakeOptions)
	flag.IntVar(&opt.capacity, "capacity", 10, "maximum concurrent clients")
	flag.StringVar(&opt.broker, "broker", "https://snowflake-reg.appspot.com/", "broker URL")
	flag.StringVar(&opt.relay, "relay", "wss://snowflake.bamsoftware.com/", "websocket relay URL")
	flag.StringVar(&opt.stun, "stun", "stun:stun.l.google.com:19302", "stun URL")
	flag.StringVar(&logFilename, "log", "", "log filename")
	flag.Parse()

	if logFilename != "" {
		f, err := os.OpenFile(logFilename, os.O_WRONLY | os.O_APPEND, 0600)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	}

	var err error
	opt.brokerURL, err = url.Parse(opt.broker)
	if err != nil {
		log.Fatalf("invalid broker url: %s", err)
	}
	opt.stunURL, err = url.Parse(opt.stun)
	if err != nil {
		log.Fatalf("invalid stun url: %s", err)
	}
	opt.relayURL, err = url.Parse(opt.relay)
	if err != nil {
		log.Fatalf("invalid relay url: %s", err)
	}

	config = webrtc.NewConfiguration(webrtc.OptionIceServer(opt.stun))
	tokens = make(chan bool, opt.capacity)
	for i := 0; i < opt.capacity; i++ {
		tokens <- true
	}

	for {
		getToken()
		sessionID := genSessionID()
		runSession(sessionID)
	}
}
