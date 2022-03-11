//Package for communication with the snowflake broker

//import "git.torproject.org/pluggable-transports/snowflake.git/v2/common/messages"
package messages

import (
	"encoding/json"
	"fmt"
	"strings"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/nat"
)

const (
	version      = "1.2"
	ProxyUnknown = "unknown"
)

var KnownProxyTypes = map[string]bool{
	"standalone": true,
	"webext":     true,
	"badge":      true,
	"iptproxy":   true,
}

/* Version 1.2 specification:

== ProxyPollRequest ==
{
  Sid: [generated session id of proxy],
  Version: 1.2,
  Type: ["badge"|"webext"|"standalone"],
  NAT: ["unknown"|"restricted"|"unrestricted"],
  Clients: [number of current clients, rounded down to multiples of 8]
}

== ProxyPollResponse ==
1) If a client is matched:
HTTP 200 OK
{
  Status: "client match",
  {
    type: offer,
    sdp: [WebRTC SDP]
  },
  NAT: ["unknown"|"restricted"|"unrestricted"]
}

2) If a client is not matched:
HTTP 200 OK

{
    Status: "no match"
}

3) If the request is malformed:
HTTP 400 BadRequest

== ProxyAnswerRequest ==
{
  Sid: [generated session id of proxy],
  Version: 1.2,
  Answer:
  {
    type: answer,
    sdp: [WebRTC SDP]
  }
}

== ProxyAnswerResponse ==
1) If the client retrieved the answer:
HTTP 200 OK

{
  Status: "success"
}

2) If the client left:
HTTP 200 OK

{
  Status: "client gone"
}

3) If the request is malformed:
HTTP 400 BadRequest

*/

type ProxyPollRequest struct {
	Sid     string
	Version string
	Type    string
	NAT     string
	Clients int
}

func EncodeProxyPollRequest(sid string, proxyType string, natType string, clients int) ([]byte, error) {
	return json.Marshal(ProxyPollRequest{
		Sid:     sid,
		Version: version,
		Type:    proxyType,
		NAT:     natType,
		Clients: clients,
	})
}

// Decodes a poll message from a snowflake proxy and returns the
// sid, proxy type, nat type and clients of the proxy on success
// and an error if it failed
func DecodeProxyPollRequest(data []byte) (sid string, proxyType string, natType string, clients int, err error) {
	var message ProxyPollRequest

	err = json.Unmarshal(data, &message)
	if err != nil {
		return
	}

	majorVersion := strings.Split(message.Version, ".")[0]
	if majorVersion != "1" {
		err = fmt.Errorf("using unknown version")
		return
	}

	// Version 1.x requires an Sid
	if message.Sid == "" {
		err = fmt.Errorf("no supplied session id")
		return
	}

	switch message.NAT {
	case "":
		message.NAT = nat.NATUnknown
	case nat.NATUnknown:
	case nat.NATRestricted:
	case nat.NATUnrestricted:
	default:
		err = fmt.Errorf("invalid NAT type")
		return
	}

	// we don't reject polls with an unknown proxy type because we encourage
	// projects that embed proxy code to include their own type
	if !KnownProxyTypes[message.Type] {
		message.Type = ProxyUnknown
	}

	return message.Sid, message.Type, message.NAT, message.Clients, nil
}

type ProxyPollResponse struct {
	Status string
	Offer  string
	NAT    string
}

func EncodePollResponse(offer string, success bool, natType string) ([]byte, error) {
	if success {
		return json.Marshal(ProxyPollResponse{
			Status: "client match",
			Offer:  offer,
			NAT:    natType,
		})

	}
	return json.Marshal(ProxyPollResponse{
		Status: "no match",
	})
}

// Decodes a poll response from the broker and returns an offer and the client's NAT type
// If there is a client match, the returned offer string will be non-empty
func DecodePollResponse(data []byte) (string, string, error) {
	var message ProxyPollResponse

	err := json.Unmarshal(data, &message)
	if err != nil {
		return "", "", err
	}
	if message.Status == "" {
		return "", "", fmt.Errorf("received invalid data")
	}

	if message.Status == "client match" {
		if message.Offer == "" {
			return "", "", fmt.Errorf("no supplied offer")
		}
	} else {
		message.Offer = ""
	}

	natType := message.NAT
	if natType == "" {
		natType = "unknown"
	}

	return message.Offer, natType, nil
}

type ProxyAnswerRequest struct {
	Version string
	Sid     string
	Answer  string
}

func EncodeAnswerRequest(answer string, sid string) ([]byte, error) {
	return json.Marshal(ProxyAnswerRequest{
		Version: version,
		Sid:     sid,
		Answer:  answer,
	})
}

// Returns the sdp answer and proxy sid
func DecodeAnswerRequest(data []byte) (string, string, error) {
	var message ProxyAnswerRequest

	err := json.Unmarshal(data, &message)
	if err != nil {
		return "", "", err
	}

	majorVersion := strings.Split(message.Version, ".")[0]
	if majorVersion != "1" {
		return "", "", fmt.Errorf("using unknown version")
	}

	if message.Sid == "" || message.Answer == "" {
		return "", "", fmt.Errorf("no supplied sid or answer")
	}

	return message.Answer, message.Sid, nil
}

type ProxyAnswerResponse struct {
	Status string
}

func EncodeAnswerResponse(success bool) ([]byte, error) {
	if success {
		return json.Marshal(ProxyAnswerResponse{
			Status: "success",
		})

	}
	return json.Marshal(ProxyAnswerResponse{
		Status: "client gone",
	})
}

func DecodeAnswerResponse(data []byte) (bool, error) {
	var message ProxyAnswerResponse
	var success bool

	err := json.Unmarshal(data, &message)
	if err != nil {
		return success, err
	}
	if message.Status == "" {
		return success, fmt.Errorf("received invalid data")
	}

	if message.Status == "success" {
		success = true
	}

	return success, nil
}
