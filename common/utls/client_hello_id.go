package utls

import (
	"errors"
	utls "github.com/refraction-networking/utls"
	"strings"
)

// ported from https://github.com/max-b/snowflake/commit/9dded063cb74c6941a16ad90b9dd0e06e618e55e
var clientHelloIDMap = map[string]utls.ClientHelloID{
	// No HelloCustom: not useful for external configuration.
	// No HelloRandomized: doesn't negotiate consistent ALPN.
	"hellorandomizedalpn":   utls.HelloRandomizedALPN,
	"hellorandomizednoalpn": utls.HelloRandomizedNoALPN,
	"hellofirefox_auto":     utls.HelloFirefox_Auto,
	"hellofirefox_55":       utls.HelloFirefox_55,
	"hellofirefox_56":       utls.HelloFirefox_56,
	"hellofirefox_63":       utls.HelloFirefox_63,
	"hellofirefox_65":       utls.HelloFirefox_65,
	"hellochrome_auto":      utls.HelloChrome_Auto,
	"hellochrome_58":        utls.HelloChrome_58,
	"hellochrome_62":        utls.HelloChrome_62,
	"hellochrome_70":        utls.HelloChrome_70,
	"hellochrome_72":        utls.HelloChrome_72,
	"helloios_auto":         utls.HelloIOS_Auto,
	"helloios_11_1":         utls.HelloIOS_11_1,
	"helloios_12_1":         utls.HelloIOS_12_1,
}

var errNameNotFound = errors.New("client hello name is unrecognized")

func NameToUTLSID(name string) (utls.ClientHelloID, error) {
	normalizedName := strings.ToLower(name)
	if id, ok := clientHelloIDMap[normalizedName]; ok {
		return id, nil
	}
	return utls.ClientHelloID{}, errNameNotFound
}
