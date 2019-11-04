package main

import (
	"net"
	"strings"
	"testing"

	"github.com/pion/webrtc"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRemoteIPFromSDP(t *testing.T) {
	tests := []struct {
		sdp      string
		expected net.IP
	}{
		// https://tools.ietf.org/html/rfc4566#section-5
		{`v=0
o=jdoe 2890844526 2890842807 IN IP4 10.47.16.5
s=SDP Seminar
i=A Seminar on the session description protocol
u=http://www.example.com/seminars/sdp.pdf
e=j.doe@example.com (Jane Doe)
c=IN IP4 224.2.17.12/127
t=2873397496 2873404696
a=recvonly
m=audio 49170 RTP/AVP 0
m=video 51372 RTP/AVP 99
a=rtpmap:99 h263-1998/90000
`, net.ParseIP("224.2.17.12")},
		// Missing c= line
		{`v=0
o=jdoe 2890844526 2890842807 IN IP4 10.47.16.5
s=SDP Seminar
i=A Seminar on the session description protocol
u=http://www.example.com/seminars/sdp.pdf
e=j.doe@example.com (Jane Doe)
t=2873397496 2873404696
a=recvonly
m=audio 49170 RTP/AVP 0
m=video 51372 RTP/AVP 99
a=rtpmap:99 h263-1998/90000
`, nil},
		// Single line, IP address only
		{`c=IN IP4 224.2.1.1
`, net.ParseIP("224.2.1.1")},
		// Same, with TTL
		{`c=IN IP4 224.2.1.1/127
`, net.ParseIP("224.2.1.1")},
		// Same, with TTL and multicast addresses
		{`c=IN IP4 224.2.1.1/127/3
`, net.ParseIP("224.2.1.1")},
		// IPv6, address only
		{`c=IN IP6 FF15::101
`, net.ParseIP("ff15::101")},
		// Same, with multicast addresses
		{`c=IN IP6 FF15::101/3
`, net.ParseIP("ff15::101")},
		// Multiple c= lines
		{`c=IN IP4 1.2.3.4
c=IN IP4 5.6.7.8
`, net.ParseIP("1.2.3.4")},
		// Modified from SDP sent by snowflake-client.
		{`v=0
o=- 7860378660295630295 2 IN IP4 127.0.0.1
s=-
t=0 0
a=group:BUNDLE data
a=msid-semantic: WMS
m=application 54653 DTLS/SCTP 5000
c=IN IP4 1.2.3.4
a=candidate:3581707038 1 udp 2122260223 192.168.0.1 54653 typ host generation 0 network-id 1 network-cost 50
a=candidate:2617212910 1 tcp 1518280447 192.168.0.1 59673 typ host tcptype passive generation 0 network-id 1 network-cost 50
a=candidate:2082671819 1 udp 1686052607 1.2.3.4 54653 typ srflx raddr 192.168.0.1 rport 54653 generation 0 network-id 1 network-cost 50
a=ice-ufrag:IBdf
a=ice-pwd:G3lTrrC9gmhQx481AowtkhYz
a=fingerprint:sha-256 53:F8:84:D9:3C:1F:A0:44:AA:D6:3C:65:80:D3:CB:6F:23:90:17:41:06:F9:9C:10:D8:48:4A:A8:B6:FA:14:A1
a=setup:actpass
a=mid:data
a=sctpmap:5000 webrtc-datachannel 1024
`, net.ParseIP("1.2.3.4")},
		// Improper character within IPv4
		{`c=IN IP4 224.2z.1.1
`, nil},
		// Improper character within IPv6
		{`c=IN IP6 ff15:g::101
`, nil},
		// Bogus "IP7" addrtype
		{`c=IN IP7 1.2.3.4
`, nil},
	}

	for _, test := range tests {
		// https://tools.ietf.org/html/rfc4566#section-5: "The sequence
		// CRLF (0x0d0a) is used to end a record, although parsers
		// SHOULD be tolerant and also accept records terminated with a
		// single newline character." We represent the test cases with
		// LF line endings for convenience, and test them both that way
		// and with CRLF line endings.
		lfSDP := test.sdp
		crlfSDP := strings.Replace(lfSDP, "\n", "\r\n", -1)

		ip := remoteIPFromSDP(lfSDP)
		if !ip.Equal(test.expected) {
			t.Errorf("expected %q, got %q from %q", test.expected, ip, lfSDP)
		}
		ip = remoteIPFromSDP(crlfSDP)
		if !ip.Equal(test.expected) {
			t.Errorf("expected %q, got %q from %q", test.expected, ip, crlfSDP)
		}
	}
}

func TestSessionDescriptions(t *testing.T) {
	Convey("Session description deserialization", t, func() {
		for _, test := range []struct {
			msg string
			ret *webrtc.SessionDescription
		}{
			{
				"test",
				nil,
			},
			{
				`{"type":"answer"}`,
				nil,
			},
			{
				`{"sdp":"test"}`,
				nil,
			},
			{
				`{"type":"test", "sdp":"test"}`,
				nil,
			},
			{
				`{"type":"answer", "sdp":"test"}`,
				&webrtc.SessionDescription{
					Type: webrtc.SDPTypeAnswer,
					SDP:  "test",
				},
			},
			{
				`{"type":"pranswer", "sdp":"test"}`,
				&webrtc.SessionDescription{
					Type: webrtc.SDPTypePranswer,
					SDP:  "test",
				},
			},
			{
				`{"type":"rollback", "sdp":"test"}`,
				&webrtc.SessionDescription{
					Type: webrtc.SDPTypeRollback,
					SDP:  "test",
				},
			},
			{
				`{"type":"offer", "sdp":"test"}`,
				&webrtc.SessionDescription{
					Type: webrtc.SDPTypeOffer,
					SDP:  "test",
				},
			},
		} {
			desc := deserializeSessionDescription(test.msg)
			So(desc, ShouldResemble, test.ret)
		}
	})
	Convey("Session description serialization", t, func() {
		for _, test := range []struct {
			desc *webrtc.SessionDescription
			ret  string
		}{
			{
				&webrtc.SessionDescription{
					Type: webrtc.SDPTypeOffer,
					SDP:  "test",
				},
				`{"type":"offer","sdp":"test"}`,
			},
		} {
			msg := serializeSessionDescription(test.desc)
			So(msg, ShouldResemble, test.ret)
		}
	})
}

func TestUtilityFuncs(t *testing.T) {
	Convey("LimitedRead", t, func() {
	})
}
