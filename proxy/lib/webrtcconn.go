package snowflake_proxy

import (
	"fmt"
	"io"
	"log"
	"net"
	"regexp"
	"sync"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/event"
	"github.com/pion/ice/v2"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

var remoteIPPatterns = []*regexp.Regexp{
	/* IPv4 */
	regexp.MustCompile(`(?m)^c=IN IP4 ([\d.]+)(?:(?:\/\d+)?\/\d+)?(:? |\r?\n)`),
	/* IPv6 */
	regexp.MustCompile(`(?m)^c=IN IP6 ([0-9A-Fa-f:.]+)(?:\/\d+)?(:? |\r?\n)`),
}

type webRTCConn struct {
	dc *webrtc.DataChannel
	pc *webrtc.PeerConnection
	pr *io.PipeReader

	lock sync.Mutex // Synchronization for DataChannel destruction
	once sync.Once  // Synchronization for PeerConnection destruction

	bytesLogger bytesLogger
	eventLogger event.SnowflakeEventReceiver
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
