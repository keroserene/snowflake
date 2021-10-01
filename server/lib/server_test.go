package snowflake_server

import (
	"net"
	"strconv"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestClientAddr(t *testing.T) {
	Convey("Testing clientAddr", t, func() {
		// good tests
		for _, test := range []struct {
			input    string
			expected net.IP
		}{
			{"1.2.3.4", net.ParseIP("1.2.3.4")},
			{"1:2::3:4", net.ParseIP("1:2::3:4")},
		} {
			useraddr := clientAddr(test.input).String()
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
			"0.0.0.0",
			"[::]",
		} {
			useraddr := clientAddr(input).String()
			if useraddr != "" {
				t.Errorf("clientAddr(%q) → %q, not %q", input, useraddr, "")
			}
		}
	})
}
