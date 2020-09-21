package util

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUtil(t *testing.T) {
	Convey("Strip", t, func() {
		const offerStart = "v=0\r\no=- 4358805017720277108 2 IN IP4 8.8.8.8\r\ns=-\r\nt=0 0\r\na=group:BUNDLE data\r\na=msid-semantic: WMS\r\nm=application 56688 DTLS/SCTP 5000\r\nc=IN IP4 8.8.8.8\r\n"
		const goodCandidate = "a=candidate:3769337065 1 udp 2122260223 8.8.8.8 56688 typ host generation 0 network-id 1 network-cost 50\r\n"
		const offerEnd = "a=ice-ufrag:aMAZ\r\na=ice-pwd:jcHb08Jjgrazp2dzjdrvPPvV\r\na=ice-options:trickle\r\na=fingerprint:sha-256 C8:88:EE:B9:E7:02:2E:21:37:ED:7A:D1:EB:2B:A3:15:A2:3B:5B:1C:3D:D4:D5:1F:06:CF:52:40:03:F8:DD:66\r\na=setup:actpass\r\na=mid:data\r\na=sctpmap:5000 webrtc-datachannel 1024\r\n"

		offer := offerStart + goodCandidate +
			"a=candidate:3769337065 1 udp 2122260223 192.168.0.100 56688 typ host generation 0 network-id 1 network-cost 50\r\n" + // IsLocal IPv4
			"a=candidate:3769337065 1 udp 2122260223 100.127.50.5 56688 typ host generation 0 network-id 1 network-cost 50\r\n" + // IsLocal IPv4
			"a=candidate:3769337065 1 udp 2122260223 169.254.250.88 56688 typ host generation 0 network-id 1 network-cost 50\r\n" + // IsLocal IPv4
			"a=candidate:3769337065 1 udp 2122260223 fdf8:f53b:82e4::53 56688 typ host generation 0 network-id 1 network-cost 50\r\n" + // IsLocal IPv6
			"a=candidate:3769337065 1 udp 2122260223 0.0.0.0 56688 typ host generation 0 network-id 1 network-cost 50\r\n" + // IsUnspecified IPv4
			"a=candidate:3769337065 1 udp 2122260223 :: 56688 typ host generation 0 network-id 1 network-cost 50\r\n" + // IsUnspecified IPv6
			"a=candidate:3769337065 1 udp 2122260223 127.0.0.1 56688 typ host generation 0 network-id 1 network-cost 50\r\n" + // IsLoopback IPv4
			"a=candidate:3769337065 1 udp 2122260223 ::1 56688 typ host generation 0 network-id 1 network-cost 50\r\n" + // IsLoopback IPv6
			offerEnd

		So(StripLocalAddresses(offer), ShouldEqual, offerStart+goodCandidate+offerEnd)
	})
}
