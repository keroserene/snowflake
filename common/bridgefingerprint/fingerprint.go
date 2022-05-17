package bridgefingerprint

import (
	"encoding/hex"
	"errors"
)

type Fingerprint string

var ErrBridgeFingerprintInvalid = errors.New("bridge fingerprint invalid")

func FingerprintFromBytes(bytes []byte) (Fingerprint, error) {
	n := len(bytes)
	if n != 20 && n != 32 {
		return Fingerprint(""), ErrBridgeFingerprintInvalid
	}
	return Fingerprint(bytes), nil
}

func FingerprintFromHexString(hexString string) (Fingerprint, error) {
	decoded, err := hex.DecodeString(hexString)
	if err != nil {
		return "", err
	}
	return FingerprintFromBytes(decoded)
}

func (f Fingerprint) ToBytes() []byte {
	return []byte(f)
}
