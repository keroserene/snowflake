package ipsetsink

import (
	"bytes"
	"crypto/hmac"
	"encoding/binary"
	"hash"

	"github.com/clarkduvall/hyperloglog"
	"golang.org/x/crypto/sha3"
)

func NewIPSetSink(maskingKey string) *IPSetSink {
	countDistinct, _ := hyperloglog.NewPlus(18)
	return &IPSetSink{
		ipMaskingKey:  maskingKey,
		countDistinct: countDistinct,
	}
}

type IPSetSink struct {
	ipMaskingKey  string
	countDistinct *hyperloglog.HyperLogLogPlus
}

func (s *IPSetSink) maskIPAddress(ipAddress string) []byte {
	hmacIPMasker := hmac.New(func() hash.Hash {
		return sha3.New256()
	}, []byte(s.ipMaskingKey))
	hmacIPMasker.Write([]byte(ipAddress))
	return hmacIPMasker.Sum(nil)
}

func (s *IPSetSink) AddIPToSet(ipAddress string) {
	s.countDistinct.Add(truncatedHash64FromBytes{hashValue(s.maskIPAddress(ipAddress))})
}

func (s *IPSetSink) Dump() ([]byte, error) {
	return s.countDistinct.GobEncode()
}

func (s *IPSetSink) Reset() {
	s.countDistinct.Clear()
}

type hashValue []byte
type truncatedHash64FromBytes struct {
	hashValue
}

func (c truncatedHash64FromBytes) Sum64() uint64 {
	var value uint64
	binary.Read(bytes.NewReader(c.hashValue), binary.BigEndian, &value)
	return value
}
