/*
Probe test server to check the reachability of Snowflake proxies from
clients with symmetric NATs.

The probe server receives an offer from a proxy, returns an answer, and then
attempts to establish a datachannel connection to that proxy. The proxy will
self-determine whether the connection opened successfully.
*/
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/messages"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/safelog"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/util"

	"github.com/pion/webrtc/v3"
	"golang.org/x/crypto/acme/autocert"
)

const (
	readLimit          = 100000                         //Maximum number of bytes to be read from an HTTP request
	dataChannelTimeout = 20 * time.Second               //time after which we assume proxy data channel will not open
	stunUrl            = "stun:stun.l.google.com:19302" //default STUN URL
)

// Create a PeerConnection from an SDP offer. Blocks until the gathering of ICE
// candidates is complete and the answer is available in LocalDescription.
func makePeerConnectionFromOffer(sdp *webrtc.SessionDescription,
	dataChan chan struct{}) (*webrtc.PeerConnection, error) {

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{stunUrl},
			},
		},
	}
	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("accept: NewPeerConnection: %s", err)
	}
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnOpen(func() {
			close(dataChan)
		})
		dc.OnClose(func() {
			dc.Close()
		})
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

	answer, err := pc.CreateAnswer(nil)
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

func probeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	resp, err := ioutil.ReadAll(http.MaxBytesReader(w, r.Body, readLimit))
	if nil != err {
		log.Println("Invalid data.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	offer, _, err := messages.DecodePollResponse(resp)
	if err != nil {
		log.Printf("Error reading offer: %s", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if offer == "" {
		log.Printf("Error processing session description: %s", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	sdp, err := util.DeserializeSessionDescription(offer)
	if err != nil {
		log.Printf("Error processing session description: %s", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	dataChan := make(chan struct{})
	pc, err := makePeerConnectionFromOffer(sdp, dataChan)
	if err != nil {
		log.Printf("Error making WebRTC connection: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	sdp = &webrtc.SessionDescription{
		Type: pc.LocalDescription().Type,
		SDP:  util.StripLocalAddresses(pc.LocalDescription().SDP),
	}
	answer, err := util.SerializeSessionDescription(sdp)
	if err != nil {
		log.Printf("Error making WebRTC connection: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	body, err := messages.EncodeAnswerRequest(answer, "stub-sid")
	if err != nil {
		log.Printf("Error making WebRTC connection: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(body)
	// Set a timeout on peerconnection. If the connection state has not
	// advanced to PeerConnectionStateConnected in this time,
	// destroy the peer connection and return the token.
	go func() {
		timer := time.NewTimer(dataChannelTimeout)
		defer timer.Stop()

		select {
		case <-dataChan:
		case <-timer.C:
		}

		if err := pc.Close(); err != nil {
			log.Printf("Error calling pc.Close: %v", err)
		}
	}()
	return

}

func main() {
	var acmeEmail string
	var acmeHostnamesCommas string
	var acmeCertCacheDir string
	var addr string
	var disableTLS bool
	var certFilename, keyFilename string
	var unsafeLogging bool

	flag.StringVar(&acmeEmail, "acme-email", "", "optional contact email for Let's Encrypt notifications")
	flag.StringVar(&acmeHostnamesCommas, "acme-hostnames", "", "comma-separated hostnames for TLS certificate")
	flag.StringVar(&acmeCertCacheDir, "acme-cert-cache", "acme-cert-cache", "directory in which certificates should be cached")
	flag.StringVar(&certFilename, "cert", "", "TLS certificate file")
	flag.StringVar(&keyFilename, "key", "", "TLS private key file")
	flag.StringVar(&addr, "addr", ":8443", "address to listen on")
	flag.BoolVar(&disableTLS, "disable-tls", false, "don't use HTTPS")
	flag.BoolVar(&unsafeLogging, "unsafe-logging", false, "prevent logs from being scrubbed")
	flag.Parse()

	var logOutput io.Writer = os.Stderr
	if unsafeLogging {
		log.SetOutput(logOutput)
	} else {
		// Scrub log output just in case an address ends up there
		log.SetOutput(&safelog.LogScrubber{Output: logOutput})
	}

	log.SetFlags(log.LstdFlags | log.LUTC)

	http.HandleFunc("/probe", probeHandler)

	server := http.Server{
		Addr: addr,
	}

	var err error
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
		// start certificate manager handler
		go func() {
			log.Printf("Starting HTTP-01 listener")
			log.Fatal(http.ListenAndServe(":80", certManager.HTTPHandler(nil)))
		}()

		server.TLSConfig = &tls.Config{GetCertificate: certManager.GetCertificate}
		err = server.ListenAndServeTLS("", "")
	} else if certFilename != "" && keyFilename != "" {
		err = server.ListenAndServeTLS(certFilename, keyFilename)
	} else if disableTLS {
		err = server.ListenAndServe()
	} else {
		log.Fatal("the --cert and --key, --acme-hostnames, or --disable-tls option is required")
	}

	if err != nil {
		log.Println(err)
	}
}
