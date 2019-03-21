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

	var buff bytes.Buffer
	scrubber := &logScrubber{&buff}
	log.SetFlags(0) //remove all extra log output for test comparisons
	log.SetOutput(scrubber)

	log.Printf("%s", "http: TLS handshake error from 129.97.208.23:38310:")

        //Example IPv4 address that ended up in log
	if bytes.Compare(buff.Bytes(), []byte("http: TLS handshake error from X.X.X.X:38310:\n")) != 0 {
		t.Errorf("log scrubber didn't scrub IPv4 address. Output: %s", string(buff.Bytes()))
	}
	buff.Reset()

	log.Printf("%s", "http2: panic serving [2620:101:f000:780:9097:75b1:519f:dbb8]:58344: interface conversion: *http2.responseWriter is not http.Hijacker: missing method Hijack")

        //Example IPv6 address that ended up in log
	if bytes.Compare(buff.Bytes(), []byte("http2: panic serving [X:X:X:X:X:X:X:X]:58344: interface conversion: *http2.responseWriter is not http.Hijacker: missing method Hijack\n")) != 0 {
		t.Errorf("log scrubber didn't scrub IPv6 address. Output: %s", string(buff.Bytes()))
	}
	buff.Reset()

        //Testing IPv6 edge cases
	log.Printf("%s", "[1::]:58344")
	log.Printf("%s", "[1:2:3:4:5:6::8]:58344")
	log.Printf("%s", "[1::7:8]:58344")
	log.Printf("%s", "[::4:5:6:7:8]:58344")
	log.Printf("%s", "[::255.255.255.255]:58344")
	log.Printf("%s", "[::ffff:0:255.255.255.255]:58344")
	log.Printf("%s", "[2001:db8:3:4::192.0.2.33]:58344")

	if bytes.Compare(buff.Bytes(), []byte("[X:X:X:X:X:X:X:X]:58344\n[X:X:X:X:X:X:X:X]:58344\n[X:X:X:X:X:X:X:X]:58344\n[X:X:X:X:X:X:X:X]:58344\n[X:X:X:X:X:X:X:XX.X.X.X]:58344\n[X:X:X:X:X:X:X:XX.X.X.X]:58344\n[X:X:X:X:X:X:X:XX.X.X.X]:58344\n")) != 0 {
		t.Errorf("log scrubber didn't scrub IPv6 address. Output: %s", string(buff.Bytes()))
	}
	buff.Reset()
}
