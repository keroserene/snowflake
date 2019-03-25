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

//Check to make sure that addresses split across calls to write are still scrubbed
func TestLogScrubberSplit(t *testing.T) {
	input := []byte("test\nhttp2: panic serving [2620:101:f000:780:9097:75b1:519f:dbb8]:58344: interface conversion: *http2.responseWriter is not http.Hijacker: missing method Hijack\n")

	expected := "test\nhttp2: panic serving [scrubbed]: interface conversion: *http2.responseWriter is not http.Hijacker: missing method Hijack\n"

	var buff bytes.Buffer
	scrubber := &logScrubber{output: &buff}
	n, err := scrubber.Write(input[:12]) //test\nhttp2:
	if n != 12 {
		t.Errorf("wrong number of bytes %d", n)
	}
	if err != nil {
		t.Errorf("%q", err)
	}
	if buff.String() != "test\n" {
		t.Errorf("Got %q, expected %q", buff.String(), "test\n")
	}

	n, err = scrubber.Write(input[12:30]) //panic serving [2620:101:f
	if n != 18 {
		t.Errorf("wrong number of bytes %d", n)
	}
	if err != nil {
		t.Errorf("%q", err)
	}
	if buff.String() != "test\n" {
		t.Errorf("Got %q, expected %q", buff.String(), "test\n")
	}

	n, err = scrubber.Write(input[30:]) //000:780:9097:75b1:519f:dbb8]:58344: interface conversion: *http2.responseWriter is not http.Hijacker: missing method Hijack\n
	if n != (len(input) - 30) {
		t.Errorf("wrong number of bytes %d", n)
	}
	if err != nil {
		t.Errorf("%q", err)
	}
	if buff.String() != expected {
		t.Errorf("Got %q, expected %q", buff.String(), expected)
	}

}

//Test the log scrubber on known problematic log messages
func TestLogScrubberMessages(t *testing.T) {
	for _, test := range []struct {
		input, expected string
	}{
		{
			"http: TLS handshake error from 129.97.208.23:38310: ",
			"http: TLS handshake error from [scrubbed]: \n",
		},
		{
			"http2: panic serving [2620:101:f000:780:9097:75b1:519f:dbb8]:58344: interface conversion: *http2.responseWriter is not http.Hijacker: missing method Hijack",
			"http2: panic serving [scrubbed]: interface conversion: *http2.responseWriter is not http.Hijacker: missing method Hijack\n",
		},
		{
			//Make sure it doesn't scrub fingerprint
			"a=fingerprint:sha-256 33:B6:FA:F6:94:CA:74:61:45:4A:D2:1F:2C:2F:75:8A:D9:EB:23:34:B2:30:E9:1B:2A:A6:A9:E0:44:72:CC:74",
			"a=fingerprint:sha-256 33:B6:FA:F6:94:CA:74:61:45:4A:D2:1F:2C:2F:75:8A:D9:EB:23:34:B2:30:E9:1B:2A:A6:A9:E0:44:72:CC:74\n",
		},
		{
			//try with enclosing parens
			"(1:2:3:4:c:d:e:f) {1:2:3:4:c:d:e:f}",
			"([scrubbed]) {[scrubbed]}\n",
		},
	} {
		var buff bytes.Buffer
		log.SetFlags(0) //remove all extra log output for test comparisons
		log.SetOutput(&logScrubber{output: &buff})
		log.Print(test.input)
		if buff.String() != test.expected {
			t.Errorf("%q: got %q, expected %q", test.input, buff.String(), test.expected)
		}
	}

}

func TestLogScrubberGoodFormats(t *testing.T) {
	for _, addr := range []string{
		// IPv4
		"1.2.3.4",
		"255.255.255.255",
		// IPv4 with port
		"1.2.3.4:55",
		"255.255.255.255:65535",
		// IPv6
		"1:2:3:4:c:d:e:f",
		"1111:2222:3333:4444:CCCC:DDDD:EEEE:FFFF",
		// IPv6 with brackets
		"[1:2:3:4:c:d:e:f]",
		"[1111:2222:3333:4444:CCCC:DDDD:EEEE:FFFF]",
		// IPv6 with brackets and port
		"[1:2:3:4:c:d:e:f]:55",
		"[1111:2222:3333:4444:CCCC:DDDD:EEEE:FFFF]:65535",
		// compressed IPv6
		"::d:e:f",
		"1:2:3::",
		"1:2:3::d:e:f",
		"1111:2222:3333::DDDD:EEEE:FFFF",
		// compressed IPv6 with brackets
		"[::d:e:f]",
		"[1:2:3::]",
		"[1:2:3::d:e:f]",
		"[1111:2222:3333::DDDD:EEEE:FFFF]",
		"[1:2:3:4:5:6::8]",
		"[1::7:8]",
		// compressed IPv6 with brackets and port
		"[1::]:58344",
		"[::d:e:f]:55",
		"[1:2:3::]:55",
		"[1:2:3::d:e:f]:55",
		"[1111:2222:3333::DDDD:EEEE:FFFF]:65535",
		// IPv4-compatible and IPv4-mapped
		"::255.255.255.255",
		"::ffff:255.255.255.255",
		"[::255.255.255.255]",
		"[::ffff:255.255.255.255]",
		"[::255.255.255.255]:65535",
		"[::ffff:255.255.255.255]:65535",
		"[::ffff:0:255.255.255.255]",
		"[2001:db8:3:4::192.0.2.33]",
	} {
		var buff bytes.Buffer
		log.SetFlags(0) //remove all extra log output for test comparisons
		log.SetOutput(&logScrubber{output: &buff})
		log.Print(addr)
		if buff.String() != "[scrubbed]\n" {
			t.Errorf("%q: Got %q, expected %q", addr, buff.String(), "[scrubbed]\n")
		}
	}
}
