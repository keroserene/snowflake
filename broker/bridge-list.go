package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

var ErrBridgeNotFound = errors.New("bridge not found")
var ErrBridgeFingerprintInvalid = errors.New("bridge fingerprint invalid")

func NewBridgeListHolder() BridgeListHolderFileBased {
	return &bridgeListHolder{}
}

type bridgeListHolder struct {
	bridgeInfo       map[[20]byte]BridgeInfo
	accessBridgeInfo sync.RWMutex
}

type BridgeListHolder interface {
	GetBridgeInfo(fingerprint [20]byte) (BridgeInfo, error)
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

func (h *bridgeListHolder) GetBridgeInfo(fingerprint [20]byte) (BridgeInfo, error) {
	h.accessBridgeInfo.RLock()
	defer h.accessBridgeInfo.RUnlock()
	if bridgeInfo, ok := h.bridgeInfo[fingerprint]; ok {
		return bridgeInfo, nil
	}
	return BridgeInfo{}, ErrBridgeNotFound
}

func (h *bridgeListHolder) LoadBridgeInfo(reader io.Reader) error {
	bridgeInfoMap := map[[20]byte]BridgeInfo{}
	inputScanner := bufio.NewScanner(reader)
	for inputScanner.Scan() {
		inputLine := inputScanner.Bytes()
		bridgeInfo := BridgeInfo{}
		if err := json.Unmarshal(inputLine, &bridgeInfo); err != nil {
			return err
		}
		var bridgeHash [20]byte
		if n, err := hex.Decode(bridgeHash[:], []byte(bridgeInfo.Fingerprint)); err != nil {
			return err
		} else if n != 20 {
			return ErrBridgeFingerprintInvalid
		}
		bridgeInfoMap[bridgeHash] = bridgeInfo
	}
	h.accessBridgeInfo.Lock()
	defer h.accessBridgeInfo.Unlock()
	h.bridgeInfo = bridgeInfoMap
	return nil
}
