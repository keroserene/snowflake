package util

import (
	"encoding/json"
	"errors"
	"net"

	"github.com/pion/ice/v2"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

func SerializeSessionDescription(desc *webrtc.SessionDescription) (string, error) {
	bytes, err := json.Marshal(*desc)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func DeserializeSessionDescription(msg string) (*webrtc.SessionDescription, error) {
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(msg), &parsed)
	if err != nil {
		return nil, err
	}
	if _, ok := parsed["type"]; !ok {
		return nil, errors.New("cannot deserialize SessionDescription without type field")
	}
	if _, ok := parsed["sdp"]; !ok {
		return nil, errors.New("cannot deserialize SessionDescription without sdp field")
	}

	var stype webrtc.SDPType
	switch parsed["type"].(string) {
	default:
		return nil, errors.New("Unknown SDP type")
	case "offer":
		stype = webrtc.SDPTypeOffer
	case "pranswer":
		stype = webrtc.SDPTypePranswer
	case "answer":
		stype = webrtc.SDPTypeAnswer
	case "rollback":
		stype = webrtc.SDPTypeRollback
	}

	return &webrtc.SessionDescription{
		Type: stype,
		SDP:  parsed["sdp"].(string),
	}, nil
}

// Stolen from https://github.com/golang/go/pull/30278
func IsLocal(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		// Local IPv4 addresses are defined in https://tools.ietf.org/html/rfc1918
		return ip4[0] == 10 ||
			(ip4[0] == 172 && ip4[1]&0xf0 == 16) ||
			(ip4[0] == 192 && ip4[1] == 168) ||
			// Carrier-Grade NAT as per https://tools.ietf.org/htm/rfc6598
			(ip4[0] == 100 && ip4[1]&0xc0 == 64) ||
			// Dynamic Configuration as per https://tools.ietf.org/htm/rfc3927
			(ip4[0] == 169 && ip4[1] == 254)
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
				c, err := ice.UnmarshalCandidate(a.Value)
				if err == nil && c.Type() == ice.CandidateTypeHost {
					ip := net.ParseIP(c.Address())
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
