package snowflake_server

import (
	"encoding/binary"
	"net"
	"testing"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/turbotunnel"
)

func TestClientIDMap(t *testing.T) {
	// Convert a uint64 into a ClientID.
	id := func(n uint64) turbotunnel.ClientID {
		var clientID turbotunnel.ClientID
		binary.PutUvarint(clientID[:], n)
		return clientID
	}

	// Does m.Get(key) and checks that the output matches what is expected.
	expectGet := func(m *clientIDMap, clientID turbotunnel.ClientID, expectedAddr string, expectedOK bool) {
		t.Helper()
		addr, ok := m.Get(clientID)
		if (ok && addr.String() != expectedAddr) || ok != expectedOK {
			t.Errorf("expected (%+q, %v), got (%+q, %v)", expectedAddr, expectedOK, addr, ok)
		}
	}

	// Checks that the len of m.current is as expected.
	expectSize := func(m *clientIDMap, expectedLen int) {
		t.Helper()
		if len(m.current) != expectedLen {
			t.Errorf("expected map len %d, got %d %+v", expectedLen, len(m.current), m.current)
		}
	}

	// Convert a string to a net.Addr
	ip := func(addr string) net.Addr {
		ret, err := net.ResolveIPAddr("ip", addr)
		if err != nil {
			t.Errorf("received error: %s", err.Error())
		}
		return ret
	}

	// Zero-capacity map can't remember anything.
	{
		m := newClientIDMap(0)
		expectSize(m, 0)
		expectGet(m, id(0), "", false)
		expectGet(m, id(1234), "", false)

		m.Set(id(0), ip("1.1.1.1"))
		expectSize(m, 0)
		expectGet(m, id(0), "", false)
		expectGet(m, id(1234), "", false)

		m.Set(id(1234), ip("1.1.1.1"))
		expectSize(m, 0)
		expectGet(m, id(0), "", false)
		expectGet(m, id(1234), "", false)
	}

	{
		m := newClientIDMap(1)
		expectSize(m, 0)
		expectGet(m, id(0), "", false)
		expectGet(m, id(1), "", false)

		m.Set(id(0), ip("1.1.1.1"))
		expectSize(m, 1)
		expectGet(m, id(0), "1.1.1.1", true)
		expectGet(m, id(1), "", false)

		m.Set(id(1), ip("1.1.1.2")) // forgets the (0, "1.1.1.1") entry
		expectSize(m, 1)
		expectGet(m, id(0), "", false)
		expectGet(m, id(1), "1.1.1.2", true)

		m.Set(id(1), ip("1.1.1.3")) // forgets the (1, "1.1.1.2") entry
		expectSize(m, 1)
		expectGet(m, id(0), "", false)
		expectGet(m, id(1), "1.1.1.3", true)
	}

	{
		m := newClientIDMap(5)
		m.Set(id(0), ip("1.1.1.1"))
		m.Set(id(1), ip("1.1.1.2"))
		m.Set(id(2), ip("1.1.1.3"))
		m.Set(id(0), ip("1.1.1.4")) // shadows the (0, "1.1.1.1") entry
		m.Set(id(3), ip("1.1.1.5"))
		expectSize(m, 4)
		expectGet(m, id(0), "1.1.1.4", true)
		expectGet(m, id(1), "1.1.1.2", true)
		expectGet(m, id(2), "1.1.1.3", true)
		expectGet(m, id(3), "1.1.1.5", true)
		expectGet(m, id(4), "", false)

		m.Set(id(4), ip("1.1.1.6")) // forgets the (0, "1.1.1.1") entry but should preserve (0, "1.1.1.4")
		expectSize(m, 5)
		expectGet(m, id(0), "1.1.1.4", true)
		expectGet(m, id(1), "1.1.1.2", true)
		expectGet(m, id(2), "1.1.1.3", true)
		expectGet(m, id(3), "1.1.1.5", true)
		expectGet(m, id(4), "1.1.1.6", true)

		m.Set(id(5), ip("1.1.1.7")) // forgets the (1, "1.1.1.2") entry
		m.Set(id(0), ip("1.1.1.8")) // forgets the (2, "1.1.1.3") entry and shadows (0, "1.1.1.4")
		expectSize(m, 4)
		expectGet(m, id(0), "1.1.1.8", true)
		expectGet(m, id(1), "", false)
		expectGet(m, id(2), "", false)
		expectGet(m, id(3), "1.1.1.5", true)
		expectGet(m, id(4), "1.1.1.6", true)
		expectGet(m, id(5), "1.1.1.7", true)

		m.Set(id(0), ip("1.1.1.9"))  // forgets the (0, "1.1.1.4") entry and shadows (0, "1.1.1.8")
		m.Set(id(0), ip("1.1.1.10")) // forgets the (3, "1.1.1.5") entry and shadows (0, "1.1.1.9")
		m.Set(id(0), ip("1.1.1.11")) // forgets the (4, "1.1.1.6") entry and shadows (0, "1.1.1.10")
		m.Set(id(0), ip("1.1.1.12")) // forgets the (5, "1.1.1.7") entry and shadows (0, "1.1.1.11")
		expectSize(m, 1)
		expectGet(m, id(0), "1.1.1.12", true)
		expectGet(m, id(1), "", false)
		expectGet(m, id(2), "", false)
		expectGet(m, id(3), "", false)
		expectGet(m, id(4), "", false)
		expectGet(m, id(5), "", false)
	}
}
