package ipsetsink

import (
	"crypto/hmac"
	"hash"
	"hash/crc64"

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
	s.countDistinct.Add(crc64FromBytes{hashValue(s.maskIPAddress(ipAddress))})
}

func (s *IPSetSink) Dump() ([]byte, error) {
	return s.countDistinct.GobEncode()
}

func (s *IPSetSink) Reset() {
	s.countDistinct.Clear()
}

type hashValue []byte
type crc64FromBytes struct {
	hashValue
}

func (c crc64FromBytes) Sum64() uint64 {
	return crc64.Checksum(c.hashValue, crc64.MakeTable(crc64.ECMA))
}
