package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/bridgefingerprint"
	"io"
	"sync"
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
		if err := json.Unmarshal(inputLine, &bridgeInfo); err != nil {
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
