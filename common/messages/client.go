//Package for communication with the snowflake broker

//import "git.torproject.org/pluggable-transports/snowflake.git/v2/common/messages"
package messages

import (
	"encoding/json"
	"fmt"
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
}

The NAT field is optional, and if it is missing a
value of "unknown" will be assumed.

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

type ClientPollRequest struct {
	Offer string `json:"offer"`
	NAT   string `json:"nat"`
}

// Encodes a poll message from a snowflake client
func (req *ClientPollRequest) EncodePollRequest() ([]byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return append([]byte(ClientVersion+"\n"), body...), nil
}

// Decodes a poll message from a snowflake client
func DecodeClientPollRequest(data []byte) (*ClientPollRequest, error) {
	var message ClientPollRequest

	err := json.Unmarshal(data, &message)
	if err != nil {
		return nil, err
	}

	if message.Offer == "" {
		return nil, fmt.Errorf("no supplied offer")
	}

	if message.NAT == "" {
		message.NAT = "unknown"
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
