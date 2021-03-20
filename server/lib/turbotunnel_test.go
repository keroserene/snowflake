package lib

import (
	"encoding/binary"
	"testing"

	"git.torproject.org/pluggable-transports/snowflake.git/common/turbotunnel"
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
		if addr != expectedAddr || ok != expectedOK {
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

	// Zero-capacity map can't remember anything.
	{
		m := newClientIDMap(0)
		expectSize(m, 0)
		expectGet(m, id(0), "", false)
		expectGet(m, id(1234), "", false)

		m.Set(id(0), "A")
		expectSize(m, 0)
		expectGet(m, id(0), "", false)
		expectGet(m, id(1234), "", false)

		m.Set(id(1234), "A")
		expectSize(m, 0)
		expectGet(m, id(0), "", false)
		expectGet(m, id(1234), "", false)
	}

	{
		m := newClientIDMap(1)
		expectSize(m, 0)
		expectGet(m, id(0), "", false)
		expectGet(m, id(1), "", false)

		m.Set(id(0), "A")
		expectSize(m, 1)
		expectGet(m, id(0), "A", true)
		expectGet(m, id(1), "", false)

		m.Set(id(1), "B") // forgets the (0, "A") entry
		expectSize(m, 1)
		expectGet(m, id(0), "", false)
		expectGet(m, id(1), "B", true)

		m.Set(id(1), "C") // forgets the (1, "B") entry
		expectSize(m, 1)
		expectGet(m, id(0), "", false)
		expectGet(m, id(1), "C", true)
	}

	{
		m := newClientIDMap(5)
		m.Set(id(0), "A")
		m.Set(id(1), "B")
		m.Set(id(2), "C")
		m.Set(id(0), "D") // shadows the (0, "D") entry
		m.Set(id(3), "E")
		expectSize(m, 4)
		expectGet(m, id(0), "D", true)
		expectGet(m, id(1), "B", true)
		expectGet(m, id(2), "C", true)
		expectGet(m, id(3), "E", true)
		expectGet(m, id(4), "", false)

		m.Set(id(4), "F") // forgets the (0, "A") entry but should preserve (0, "D")
		expectSize(m, 5)
		expectGet(m, id(0), "D", true)
		expectGet(m, id(1), "B", true)
		expectGet(m, id(2), "C", true)
		expectGet(m, id(3), "E", true)
		expectGet(m, id(4), "F", true)

		m.Set(id(5), "G") // forgets the (1, "B") entry
		m.Set(id(0), "H") // forgets the (2, "C") entry and shadows (0, "D")
		expectSize(m, 4)
		expectGet(m, id(0), "H", true)
		expectGet(m, id(1), "", false)
		expectGet(m, id(2), "", false)
		expectGet(m, id(3), "E", true)
		expectGet(m, id(4), "F", true)
		expectGet(m, id(5), "G", true)

		m.Set(id(0), "I") // forgets the (0, "D") entry and shadows (0, "H")
		m.Set(id(0), "J") // forgets the (3, "E") entry and shadows (0, "I")
		m.Set(id(0), "K") // forgets the (4, "F") entry and shadows (0, "J")
		m.Set(id(0), "L") // forgets the (5, "G") entry and shadows (0, "K")
		expectSize(m, 1)
		expectGet(m, id(0), "L", true)
		expectGet(m, id(1), "", false)
		expectGet(m, id(2), "", false)
		expectGet(m, id(3), "", false)
		expectGet(m, id(4), "", false)
		expectGet(m, id(5), "", false)
	}
}
