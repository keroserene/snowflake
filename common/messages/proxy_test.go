package messages

import (
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDecodeProxyPollRequest(t *testing.T) {
	Convey("Context", t, func() {
		for _, test := range []struct {
			sid       string
			proxyType string
			natType   string
			data      string
			err       error
		}{
			{
				//Version 1.0 proxy message
				"ymbcCMto7KHNGYlp",
				"",
				"unknown",
				`{"Sid":"ymbcCMto7KHNGYlp","Version":"1.0"}`,
				nil,
			},
			{
				//Version 1.1 proxy message
				"ymbcCMto7KHNGYlp",
				"standalone",
				"unknown",
				`{"Sid":"ymbcCMto7KHNGYlp","Version":"1.1","Type":"standalone"}`,
				nil,
			},
			{
				//Version 1.2 proxy message
				"ymbcCMto7KHNGYlp",
				"standalone",
				"restricted",
				`{"Sid":"ymbcCMto7KHNGYlp","Version":"1.2","Type":"standalone", "NAT":"restricted"}`,
				nil,
			},
			{
				//Version 0.X proxy message:
				"",
				"",
				"",
				"",
				&json.SyntaxError{},
			},
			{
				"",
				"",
				"",
				`{"Sid":"ymbcCMto7KHNGYlp"}`,
				fmt.Errorf(""),
			},
			{
				"",
				"",
				"",
				"{}",
				fmt.Errorf(""),
			},
			{
				"",
				"",
				"",
				`{"Version":"1.0"}`,
				fmt.Errorf(""),
			},
			{
				"",
				"",
				"",
				`{"Version":"2.0"}`,
				fmt.Errorf(""),
			},
		} {
			sid, proxyType, natType, err := DecodePollRequest([]byte(test.data))
			So(sid, ShouldResemble, test.sid)
			So(proxyType, ShouldResemble, test.proxyType)
			So(natType, ShouldResemble, test.natType)
			So(err, ShouldHaveSameTypeAs, test.err)
		}

	})
}

func TestEncodeProxyPollRequests(t *testing.T) {
	Convey("Context", t, func() {
		b, err := EncodePollRequest("ymbcCMto7KHNGYlp", "standalone", "unknown")
		So(err, ShouldEqual, nil)
		sid, proxyType, natType, err := DecodePollRequest(b)
		So(sid, ShouldEqual, "ymbcCMto7KHNGYlp")
		So(proxyType, ShouldEqual, "standalone")
		So(natType, ShouldEqual, "unknown")
		So(err, ShouldEqual, nil)
	})
}

func TestDecodeProxyPollResponse(t *testing.T) {
	Convey("Context", t, func() {
		for _, test := range []struct {
			offer string
			data  string
			err   error
		}{
			{
				"fake offer",
				`{"Status":"client match","Offer":"fake offer"}`,
				nil,
			},
			{
				"",
				`{"Status":"no match"}`,
				nil,
			},
			{
				"",
				`{"Status":"client match"}`,
				fmt.Errorf("no supplied offer"),
			},
			{
				"",
				`{"Test":"test"}`,
				fmt.Errorf(""),
			},
		} {
			offer, err := DecodePollResponse([]byte(test.data))
			So(offer, ShouldResemble, test.offer)
			So(err, ShouldHaveSameTypeAs, test.err)
		}

	})
}

func TestEncodeProxyPollResponse(t *testing.T) {
	Convey("Context", t, func() {
		b, err := EncodePollResponse("fake offer", true)
		So(err, ShouldEqual, nil)
		offer, err := DecodePollResponse(b)
		So(offer, ShouldEqual, "fake offer")
		So(err, ShouldEqual, nil)

		b, err = EncodePollResponse("", false)
		So(err, ShouldEqual, nil)
		offer, err = DecodePollResponse(b)
		So(offer, ShouldEqual, "")
		So(err, ShouldEqual, nil)
	})
}
func TestDecodeProxyAnswerRequest(t *testing.T) {
	Convey("Context", t, func() {
		for _, test := range []struct {
			answer string
			sid    string
			data   string
			err    error
		}{
			{
				"test",
				"test",
				`{"Version":"1.0","Sid":"test","Answer":"test"}`,
				nil,
			},
			{
				"",
				"",
				`{"type":"offer","sdp":"v=0\r\no=- 4358805017720277108 2 IN IP4 [scrubbed]\r\ns=-\r\nt=0 0\r\na=group:BUNDLE data\r\na=msid-semantic: WMS\r\nm=application 56688 DTLS/SCTP 5000\r\nc=IN IP4 [scrubbed]\r\na=candidate:3769337065 1 udp 2122260223 [scrubbed] 56688 typ host generation 0 network-id 1 network-cost 50\r\na=candidate:2921887769 1 tcp 1518280447 [scrubbed] 35441 typ host tcptype passive generation 0 network-id 1 network-cost 50\r\na=ice-ufrag:aMAZ\r\na=ice-pwd:jcHb08Jjgrazp2dzjdrvPPvV\r\na=ice-options:trickle\r\na=fingerprint:sha-256 C8:88:EE:B9:E7:02:2E:21:37:ED:7A:D1:EB:2B:A3:15:A2:3B:5B:1C:3D:D4:D5:1F:06:CF:52:40:03:F8:DD:66\r\na=setup:actpass\r\na=mid:data\r\na=sctpmap:5000 webrtc-datachannel 1024\r\n"}`,
				fmt.Errorf(""),
			},
			{
				"",
				"",
				`{"Version":"1.0","Answer":"test"}`,
				fmt.Errorf(""),
			},
			{
				"",
				"",
				`{"Version":"1.0","Sid":"test"}`,
				fmt.Errorf(""),
			},
		} {
			answer, sid, err := DecodeAnswerRequest([]byte(test.data))
			So(answer, ShouldResemble, test.answer)
			So(sid, ShouldResemble, test.sid)
			So(err, ShouldHaveSameTypeAs, test.err)
		}

	})
}

func TestEncodeProxyAnswerRequest(t *testing.T) {
	Convey("Context", t, func() {
		b, err := EncodeAnswerRequest("test answer", "test sid")
		So(err, ShouldEqual, nil)
		answer, sid, err := DecodeAnswerRequest(b)
		So(answer, ShouldEqual, "test answer")
		So(sid, ShouldEqual, "test sid")
		So(err, ShouldEqual, nil)
	})
}

func TestDecodeProxyAnswerResponse(t *testing.T) {
	Convey("Context", t, func() {
		for _, test := range []struct {
			success bool
			data    string
			err     error
		}{
			{
				true,
				`{"Status":"success"}`,
				nil,
			},
			{
				false,
				`{"Status":"client gone"}`,
				nil,
			},
			{
				false,
				`{"Test":"test"}`,
				fmt.Errorf(""),
			},
		} {
			success, err := DecodeAnswerResponse([]byte(test.data))
			So(success, ShouldResemble, test.success)
			So(err, ShouldHaveSameTypeAs, test.err)
		}

	})
}

func TestEncodeProxyAnswerResponse(t *testing.T) {
	Convey("Context", t, func() {
		b, err := EncodeAnswerResponse(true)
		So(err, ShouldEqual, nil)
		success, err := DecodeAnswerResponse(b)
		So(success, ShouldEqual, true)
		So(err, ShouldEqual, nil)

		b, err = EncodeAnswerResponse(false)
		So(err, ShouldEqual, nil)
		success, err = DecodeAnswerResponse(b)
		So(success, ShouldEqual, false)
		So(err, ShouldEqual, nil)
	})
}
