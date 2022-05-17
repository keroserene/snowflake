package main

import (
	"bytes"
	"encoding/hex"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/bridgefingerprint"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

const DefaultBridges = `{"displayName":"default", "webSocketAddress":"wss://snowflake.torproject.org", "fingerprint":"2B280B23E1107BB62ABFC40DDCC8824814F80A72"}
`

const ImaginaryBridges = `{"displayName":"default", "webSocketAddress":"wss://snowflake.torproject.org", "fingerprint":"2B280B23E1107BB62ABFC40DDCC8824814F80A72"}
{"displayName":"imaginary-1", "webSocketAddress":"wss://imaginary-1-snowflake.torproject.org", "fingerprint":"2B280B23E1107BB62ABFC40DDCC8824814F80B00"}
{"displayName":"imaginary-2", "webSocketAddress":"wss://imaginary-2-snowflake.torproject.org", "fingerprint":"2B280B23E1107BB62ABFC40DDCC8824814F80B01"}
{"displayName":"imaginary-3", "webSocketAddress":"wss://imaginary-3-snowflake.torproject.org", "fingerprint":"2B280B23E1107BB62ABFC40DDCC8824814F80B02"}
{"displayName":"imaginary-4", "webSocketAddress":"wss://imaginary-4-snowflake.torproject.org", "fingerprint":"2B280B23E1107BB62ABFC40DDCC8824814F80B03"}
{"displayName":"imaginary-5", "webSocketAddress":"wss://imaginary-5-snowflake.torproject.org", "fingerprint":"2B280B23E1107BB62ABFC40DDCC8824814F80B04"}
{"displayName":"imaginary-6", "webSocketAddress":"wss://imaginary-6-snowflake.torproject.org", "fingerprint":"2B280B23E1107BB62ABFC40DDCC8824814F80B05"}
{"displayName":"imaginary-7", "webSocketAddress":"wss://imaginary-7-snowflake.torproject.org", "fingerprint":"2B280B23E1107BB62ABFC40DDCC8824814F80B06"}
{"displayName":"imaginary-8", "webSocketAddress":"wss://imaginary-8-snowflake.torproject.org", "fingerprint":"2B280B23E1107BB62ABFC40DDCC8824814F80B07"}
{"displayName":"imaginary-9", "webSocketAddress":"wss://imaginary-9-snowflake.torproject.org", "fingerprint":"2B280B23E1107BB62ABFC40DDCC8824814F80B08"}
{"displayName":"imaginary-10", "webSocketAddress":"wss://imaginary-10-snowflake.torproject.org", "fingerprint":"2B280B23E1107BB62ABFC40DDCC8824814F80B09"}
`

func TestBridgeLoad(t *testing.T) {
	Convey("load default list", t, func() {
		bridgeList := NewBridgeListHolder()
		So(bridgeList.LoadBridgeInfo(bytes.NewReader([]byte(DefaultBridges))), ShouldBeNil)
		{
			bridgeFingerprint := [20]byte{}
			{
				n, err := hex.Decode(bridgeFingerprint[:], []byte("2B280B23E1107BB62ABFC40DDCC8824814F80A72"))
				So(n, ShouldEqual, 20)
				So(err, ShouldBeNil)
			}
			Fingerprint, err := bridgefingerprint.FingerprintFromBytes(bridgeFingerprint[:])
			So(err, ShouldBeNil)
			bridgeInfo, err := bridgeList.GetBridgeInfo(Fingerprint)
			So(err, ShouldBeNil)
			So(bridgeInfo.DisplayName, ShouldEqual, "default")
			So(bridgeInfo.WebSocketAddress, ShouldEqual, "wss://snowflake.torproject.org")
		}
	})
	Convey("load imaginary list", t, func() {
		bridgeList := NewBridgeListHolder()
		So(bridgeList.LoadBridgeInfo(bytes.NewReader([]byte(ImaginaryBridges))), ShouldBeNil)
		{
			bridgeFingerprint := [20]byte{}
			{
				n, err := hex.Decode(bridgeFingerprint[:], []byte("2B280B23E1107BB62ABFC40DDCC8824814F80B07"))
				So(n, ShouldEqual, 20)
				So(err, ShouldBeNil)
			}
			Fingerprint, err := bridgefingerprint.FingerprintFromBytes(bridgeFingerprint[:])
			So(err, ShouldBeNil)
			bridgeInfo, err := bridgeList.GetBridgeInfo(Fingerprint)
			So(err, ShouldBeNil)
			So(bridgeInfo.DisplayName, ShouldEqual, "imaginary-8")
			So(bridgeInfo.WebSocketAddress, ShouldEqual, "wss://imaginary-8-snowflake.torproject.org")
		}
	})
}
