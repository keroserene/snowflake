package main

import "sync"

type bridgeListHolder struct {
	bridgeInfo       map[[20]byte]BridgeInfo
	accessBridgeInfo sync.RWMutex
}

type BridgeListHolder interface {
	GetBridgeInfo(fingerprint [20]byte) (BridgeInfo, error)
}

type BridgeInfo struct {
	DisplayName      string `json:"displayName"`
	WebSocketAddress string `json:"webSocketAddress"`
	Fingerprint      string `json:"fingerprint"`
}
