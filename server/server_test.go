package main

import (
	"bytes"
	"log"
	"net"
	"strconv"
	"testing"
)

func TestClientAddr(t *testing.T) {
	// good tests
	for _, test := range []struct {
		input    string
		expected net.IP
	}{
		{"1.2.3.4", net.ParseIP("1.2.3.4")},
		{"1:2::3:4", net.ParseIP("1:2::3:4")},
	} {
		useraddr := clientAddr(test.input)
		host, port, err := net.SplitHostPort(useraddr)
		if err != nil {
			t.Errorf("clientAddr(%q) → SplitHostPort error %v", test.input, err)
			continue
		}
		if !test.expected.Equal(net.ParseIP(host)) {
			t.Errorf("clientAddr(%q) → host %q, not %v", test.input, host, test.expected)
		}
		portNo, err := strconv.Atoi(port)
		if err != nil {
			t.Errorf("clientAddr(%q) → port %q", test.input, port)
			continue
		}
		if portNo == 0 {
			t.Errorf("clientAddr(%q) → port %d", test.input, portNo)
		}
	}

	// bad tests
	for _, input := range []string{
		"",
		"abc",
		"1.2.3.4.5",
		"[12::34]",
	} {
		useraddr := clientAddr(input)
		if useraddr != "" {
			t.Errorf("clientAddr(%q) → %q, not %q", input, useraddr, "")
		}
	}
}

func TestLogScrubber(t *testing.T) {
	for _, test := range []struct {
		input, expected string
	}{
		{
			"http: TLS handshake error from 129.97.208.23:38310:",
			"http: TLS handshake error from [scrubbed]:38310:\n",
		},
		{
			"http2: panic serving [2620:101:f000:780:9097:75b1:519f:dbb8]:58344: interface conversion: *http2.responseWriter is not http.Hijacker: missing method Hijack",
			"http2: panic serving [scrubbed]:58344: interface conversion: *http2.responseWriter is not http.Hijacker: missing method Hijack\n",
		},
		{
			//Make sure it doesn't scrub fingerprint
			"a=fingerprint:sha-256 33:B6:FA:F6:94:CA:74:61:45:4A:D2:1F:2C:2F:75:8A:D9:EB:23:34:B2:30:E9:1B:2A:A6:A9:E0:44:72:CC:74",
			"a=fingerprint:sha-256 33:B6:FA:F6:94:CA:74:61:45:4A:D2:1F:2C:2F:75:8A:D9:EB:23:34:B2:30:E9:1B:2A:A6:A9:E0:44:72:CC:74\n",
		},
		{
			"[1::]:58344",
			"[scrubbed]:58344\n",
		},
		{
			"[1:2:3:4:5:6::8]",
			"[scrubbed]\n",
		},
		{
			"[1::7:8]",
			"[scrubbed]\n",
		},
		{
			"[::4:5:6:7:8]",
			"[scrubbed]\n",
		},
		{
			"[::255.255.255.255]",
			"[scrubbed]\n",
		},
		{
			"[::ffff:0:255.255.255.255]",
			"[scrubbed]\n",
		},
		{
			"[2001:db8:3:4::192.0.2.33]",
			"[scrubbed]\n",
		},
	} {
		var buff bytes.Buffer
		log.SetFlags(0) //remove all extra log output for test comparisons
		log.SetOutput(&logScrubber{&buff})
		log.Print(test.input)
		if buff.String() != test.expected {
			t.Errorf("%q: got %q, expected %q", test.input, buff.String(), test.expected)
		}
	}

}
