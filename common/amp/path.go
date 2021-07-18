package amp

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
)

// EncodePath encodes data in a way that is suitable for the suffix of an AMP
// cache URL.
func EncodePath(data []byte) string {
	var cacheBreaker [9]byte
	_, err := rand.Read(cacheBreaker[:])
	if err != nil {
		panic(err)
	}
	b64 := base64.RawURLEncoding.EncodeToString
	return "0" + b64(cacheBreaker[:]) + "/" + b64(data)
}

// DecodePath decodes data from a path suffix as encoded by EncodePath. The path
// must have already been trimmed of any directory prefix (as might be present
// in, e.g., an HTTP request). That is, the first character of path should be
// the "0" message format indicator.
func DecodePath(path string) ([]byte, error) {
	if len(path) < 1 {
		return nil, fmt.Errorf("missing format indicator")
	}
	version := path[0]
	rest := path[1:]
	switch version {
	case '0':
		// Ignore everything else up to and including the final slash
		// (there must be at least one slash).
		i := strings.LastIndexByte(rest, '/')
		if i == -1 {
			return nil, fmt.Errorf("missing data")
		}
		return base64.RawURLEncoding.DecodeString(rest[i+1:])
	default:
		return nil, fmt.Errorf("unknown format indicator %q", version)
	}
}
