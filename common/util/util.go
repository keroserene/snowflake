package util

import (
	"encoding/json"
	"log"
	"net"

	"github.com/pion/sdp/v2"
	"github.com/pion/webrtc/v2"
)

func SerializeSessionDescription(desc *webrtc.SessionDescription) string {
	bytes, err := json.Marshal(*desc)
	if nil != err {
		log.Println(err)
		return ""
	}
	return string(bytes)
}

func DeserializeSessionDescription(msg string) *webrtc.SessionDescription {
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(msg), &parsed)
	if nil != err {
		log.Println(err)
		return nil
	}
	if _, ok := parsed["type"]; !ok {
		log.Println("Cannot deserialize SessionDescription without type field.")
		return nil
	}
	if _, ok := parsed["sdp"]; !ok {
		log.Println("Cannot deserialize SessionDescription without sdp field.")
		return nil
	}

	var stype webrtc.SDPType
	switch parsed["type"].(string) {
	default:
		log.Println("Unknown SDP type")
		return nil
	case "offer":
		stype = webrtc.SDPTypeOffer
	case "pranswer":
		stype = webrtc.SDPTypePranswer
	case "answer":
		stype = webrtc.SDPTypeAnswer
	case "rollback":
		stype = webrtc.SDPTypeRollback
	}

	if err != nil {
		log.Println(err)
		return nil
	}
	return &webrtc.SessionDescription{
		Type: stype,
		SDP:  parsed["sdp"].(string),
	}
}

// Stolen from https://github.com/golang/go/pull/30278
func IsLocal(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		// Local IPv4 addresses are defined in https://tools.ietf.org/html/rfc1918
		return ip4[0] == 10 ||
			(ip4[0] == 172 && ip4[1]&0xf0 == 16) ||
			(ip4[0] == 192 && ip4[1] == 168)
	}
	// Local IPv6 addresses are defined in https://tools.ietf.org/html/rfc4193
	return len(ip) == net.IPv6len && ip[0]&0xfe == 0xfc
}

// Removes local LAN address ICE candidates
func StripLocalAddresses(str string) string {
	var desc sdp.SessionDescription
	err := desc.Unmarshal([]byte(str))
	if err != nil {
		return str
	}
	for _, m := range desc.MediaDescriptions {
		attrs := make([]sdp.Attribute, 0)
		for _, a := range m.Attributes {
			if a.IsICECandidate() {
				ice, err := a.ToICECandidate()
				if err == nil && ice.Typ == "host" {
					ip := net.ParseIP(ice.Address)
					if ip != nil && (IsLocal(ip) || ip.IsUnspecified() || ip.IsLoopback()) {
						/* no append in this case */
						continue
					}
				}
			}
			attrs = append(attrs, a)
		}
		m.Attributes = attrs
	}
	bts, err := desc.Marshal()
	if err != nil {
		return str
	}
	return string(bts)
}
