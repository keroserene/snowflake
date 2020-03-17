package util

import (
	"encoding/json"
	"log"

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
