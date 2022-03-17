//Package for communication with the snowflake broker

//import "git.torproject.org/pluggable-transports/snowflake.git/v2/common/messages"
package messages

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/nat"
)

const ClientVersion = "1.0"

/* Client--Broker protocol v1.x specification:

All messages contain the version number
followed by a new line and then the message body
<message> := <version>\n<body>
<version> := <digit>.<digit>
<body> := <poll request>|<poll response>

There are two different types of body messages,
each encoded in JSON format

== ClientPollRequest ==
<poll request> :=
{
  offer: <sdp offer>
  [nat: (unknown|restricted|unrestricted)]
  [fingerprint: <fingerprint string>]
}

The NAT field is optional, and if it is missing a
value of "unknown" will be assumed.  The fingerprint
is also optional and, if absent, will be assigned the
fingerprint of the default bridge.

== ClientPollResponse ==
<poll response> :=
{
  [answer: <sdp answer>]
  [error: <error string>]
}

If the broker succeeded in matching the client with a proxy,
the answer field MUST contain a valid SDP answer, and the
error field MUST be empty. If the answer field is empty, the
error field MUST contain a string explaining with a reason
for the error.

*/

// The bridge fingerprint to assume, for client poll requests that do not
// specify a fingerprint.  Before #28651, there was only one bridge with one
// fingerprint, which all clients expected to be connected to implicitly.
// If a client is old enough that it does not specify a fingerprint, this is
// the fingerprint it expects.  Clients that do set a fingerprint in the
// SOCKS params will also be assumed to want to connect to the default bridge.
const defaultBridgeFingerprint = "2B280B23E1107BB62ABFC40DDCC8824814F80A72"

type ClientPollRequest struct {
	Offer       string `json:"offer"`
	NAT         string `json:"nat"`
	Fingerprint string `json:"fingerprint"`
}

// Encodes a poll message from a snowflake client
func (req *ClientPollRequest) EncodeClientPollRequest() ([]byte, error) {
	if req.Fingerprint == "" {
		req.Fingerprint = defaultBridgeFingerprint
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return append([]byte(ClientVersion+"\n"), body...), nil
}

// Decodes a poll message from a snowflake client
func DecodeClientPollRequest(data []byte) (*ClientPollRequest, error) {
	parts := bytes.SplitN(data, []byte("\n"), 2)

	if len(parts) < 2 {
		// no version number found
		return nil, fmt.Errorf("unsupported message version")
	}

	var message ClientPollRequest

	if string(parts[0]) != ClientVersion {
		return nil, fmt.Errorf("unsupported message version")
	}

	err := json.Unmarshal(parts[1], &message)
	if err != nil {
		return nil, err
	}

	if message.Offer == "" {
		return nil, fmt.Errorf("no supplied offer")
	}

	if message.Fingerprint == "" {
		message.Fingerprint = defaultBridgeFingerprint
	}
	if hex.DecodedLen(len(message.Fingerprint)) != 20 {
		return nil, fmt.Errorf("cannot decode fingerprint")
	}

	switch message.NAT {
	case "":
		message.NAT = nat.NATUnknown
	case nat.NATUnknown:
	case nat.NATRestricted:
	case nat.NATUnrestricted:
	default:
		return nil, fmt.Errorf("invalid NAT type")
	}

	return &message, nil
}

type ClientPollResponse struct {
	Answer string `json:"answer,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Encodes a poll response for a snowflake client
func (resp *ClientPollResponse) EncodePollResponse() ([]byte, error) {
	return json.Marshal(resp)
}

// Decodes a poll response for a snowflake client
// If the Error field is empty, the Answer should be non-empty
func DecodeClientPollResponse(data []byte) (*ClientPollResponse, error) {
	var message ClientPollResponse

	err := json.Unmarshal(data, &message)
	if err != nil {
		return nil, err
	}
	if message.Error == "" && message.Answer == "" {
		return nil, fmt.Errorf("received empty broker response")
	}

	return &message, nil
}
