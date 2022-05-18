/* (*BridgeListHolderFileBased).LoadBridgeInfo loads a Snowflake Server bridge info description file,
   its format is as follows:

   This file should be in newline-delimited JSON format(https://jsonlines.org/).
   For each line, the format of json data should be in the format of:
   {"displayName":"default", "webSocketAddress":"wss://snowflake.torproject.net/", "fingerprint":"2B280B23E1107BB62ABFC40DDCC8824814F80A72"}

   displayName:string is the name of this bridge. This value is not currently used programmatically.

   webSocketAddress:string is the WebSocket URL of this bridge.
   This will be the address proxy used to connect to this snowflake server.

   fingerprint:string is the identifier of the bridge.
   This will be used by a client to identify the bridge it wishes to connect to.

   The existence of ANY other fields is NOT permitted.

   The file will be considered invalid if there is at least one invalid json record.
   In this case, an error will be returned, and none of the records will be loaded.
*/

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/bridgefingerprint"
)

var ErrBridgeNotFound = errors.New("bridge not found")

func NewBridgeListHolder() BridgeListHolderFileBased {
	return &bridgeListHolder{}
}

type bridgeListHolder struct {
	bridgeInfo       map[bridgefingerprint.Fingerprint]BridgeInfo
	accessBridgeInfo sync.RWMutex
}

type BridgeListHolder interface {
	GetBridgeInfo(bridgefingerprint.Fingerprint) (BridgeInfo, error)
}

type BridgeListHolderFileBased interface {
	BridgeListHolder
	LoadBridgeInfo(reader io.Reader) error
}

type BridgeInfo struct {
	DisplayName      string `json:"displayName"`
	WebSocketAddress string `json:"webSocketAddress"`
	Fingerprint      string `json:"fingerprint"`
}

func (h *bridgeListHolder) GetBridgeInfo(fingerprint bridgefingerprint.Fingerprint) (BridgeInfo, error) {
	h.accessBridgeInfo.RLock()
	defer h.accessBridgeInfo.RUnlock()
	if bridgeInfo, ok := h.bridgeInfo[fingerprint]; ok {
		return bridgeInfo, nil
	}
	return BridgeInfo{}, ErrBridgeNotFound
}

func (h *bridgeListHolder) LoadBridgeInfo(reader io.Reader) error {
	bridgeInfoMap := map[bridgefingerprint.Fingerprint]BridgeInfo{}
	inputScanner := bufio.NewScanner(reader)
	for inputScanner.Scan() {
		inputLine := inputScanner.Bytes()
		bridgeInfo := BridgeInfo{}
		decoder := json.NewDecoder(bytes.NewReader(inputLine))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&bridgeInfo); err != nil {
			return err
		}

		var bridgeFingerprint bridgefingerprint.Fingerprint
		var err error
		if bridgeFingerprint, err = bridgefingerprint.FingerprintFromHexString(bridgeInfo.Fingerprint); err != nil {
			return err
		}

		bridgeInfoMap[bridgeFingerprint] = bridgeInfo
	}
	h.accessBridgeInfo.Lock()
	defer h.accessBridgeInfo.Unlock()
	h.bridgeInfo = bridgeInfoMap
	return nil
}
