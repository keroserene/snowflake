/*
The majority of this code is taken from a utility I wrote for pion/stun
https://github.com/pion/stun/blob/master/cmd/stun-nat-behaviour/main.go

Copyright 2018 Pion LLC

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package nat

import (
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/pion/stun"
)

var ErrTimedOut = errors.New("timed out waiting for response")

const (
	NATUnknown      = "unknown"
	NATRestricted   = "restricted"
	NATUnrestricted = "unrestricted"
)

// This function checks the NAT mapping and filtering
// behaviour and returns true if the NAT is restrictive
// (address-dependent mapping and/or port-dependent filtering)
// and false if the NAT is unrestrictive (meaning it
// will work with most other NATs),
func CheckIfRestrictedNAT(server string) (bool, error) {
	return isRestrictedMapping(server)
}

// Performs two tests from RFC 5780 to determine whether the mapping type
// of the client's NAT is address-independent or address-dependent
// Returns true if the mapping is address-dependent and false otherwise
func isRestrictedMapping(addrStr string) (bool, error) {
	var xorAddr1 stun.XORMappedAddress
	var xorAddr2 stun.XORMappedAddress

	mapTestConn, err := connect(addrStr)
	if err != nil {
		log.Printf("Error creating STUN connection: %s", err.Error())
		return false, err
	}

	defer mapTestConn.Close()

	// Test I: Regular binding request
	message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)

	resp, err := mapTestConn.RoundTrip(message, mapTestConn.PrimaryAddr)
	if err == ErrTimedOut {
		log.Printf("Error: no response from server")
		return false, err
	}
	if err != nil {
		log.Printf("Error receiving response from server: %s", err.Error())
		return false, err
	}

	// Decoding XOR-MAPPED-ADDRESS attribute from message.
	if err = xorAddr1.GetFrom(resp); err != nil {
		log.Printf("Error retrieving XOR-MAPPED-ADDRESS resonse: %s", err.Error())
		return false, err
	}

	// Decoding OTHER-ADDRESS attribute from message.
	var otherAddr stun.OtherAddress
	if err = otherAddr.GetFrom(resp); err != nil {
		log.Println("NAT discovery feature not supported by this server")
		return false, err
	}

	if err = mapTestConn.AddOtherAddr(otherAddr.String()); err != nil {
		log.Printf("Failed to resolve address %s\t", otherAddr.String())
		return false, err
	}

	// Test II: Send binding request to other address
	resp, err = mapTestConn.RoundTrip(message, mapTestConn.OtherAddr)
	if err == ErrTimedOut {
		log.Printf("Error: no response from server")
		return false, err
	}
	if err != nil {
		log.Printf("Error retrieving server response: %s", err.Error())
		return false, err
	}

	// Decoding XOR-MAPPED-ADDRESS attribute from message.
	if err = xorAddr2.GetFrom(resp); err != nil {
		log.Printf("Error retrieving XOR-MAPPED-ADDRESS resonse: %s", err.Error())
		return false, err
	}

	return xorAddr1.String() != xorAddr2.String(), nil

}

// Performs two tests from RFC 5780 to determine whether the filtering type
// of the client's NAT is port-dependent.
// Returns true if the filtering is port-dependent and false otherwise
// Note: This function is no longer used because a client's NAT type is
// determined only by their mapping type, but the functionality might
// be useful in the future and remains here.
func isRestrictedFiltering(addrStr string) (bool, error) {
	var xorAddr stun.XORMappedAddress

	mapTestConn, err := connect(addrStr)
	if err != nil {
		log.Printf("Error creating STUN connection: %s", err.Error())
		return false, err
	}

	defer mapTestConn.Close()

	// Test I: Regular binding request
	message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)

	resp, err := mapTestConn.RoundTrip(message, mapTestConn.PrimaryAddr)
	if err == ErrTimedOut {
		log.Printf("Error: no response from server")
		return false, err
	}
	if err != nil {
		log.Printf("Error: %s", err.Error())
		return false, err
	}

	// Decoding XOR-MAPPED-ADDRESS attribute from message.
	if err = xorAddr.GetFrom(resp); err != nil {
		log.Printf("Error retrieving XOR-MAPPED-ADDRESS from resonse: %s", err.Error())
		return false, err
	}

	// Test III: Request port change
	message.Add(stun.AttrChangeRequest, []byte{0x00, 0x00, 0x00, 0x02})

	_, err = mapTestConn.RoundTrip(message, mapTestConn.PrimaryAddr)
	if err != ErrTimedOut && err != nil {
		// something else went wrong
		log.Printf("Error reading response from server: %s", err.Error())
		return false, err
	}

	return err == ErrTimedOut, nil
}

// Given an address string, returns a StunServerConn
func connect(addrStr string) (*StunServerConn, error) {
	// Creating a "connection" to STUN server.
	addr, err := net.ResolveUDPAddr("udp4", addrStr)
	if err != nil {
		log.Printf("Error resolving address: %s\n", err.Error())
		return nil, err
	}

	c, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, err
	}

	mChan := listen(c)

	return &StunServerConn{
		conn:        c,
		PrimaryAddr: addr,
		messageChan: mChan,
	}, nil
}

type StunServerConn struct {
	conn        net.PacketConn
	PrimaryAddr *net.UDPAddr
	OtherAddr   *net.UDPAddr
	messageChan chan *stun.Message
}

func (c *StunServerConn) Close() {
	c.conn.Close()
}

func (c *StunServerConn) RoundTrip(msg *stun.Message, addr net.Addr) (*stun.Message, error) {
	_, err := c.conn.WriteTo(msg.Raw, addr)
	if err != nil {
		return nil, err
	}

	// Wait for response or timeout
	select {
	case m, ok := <-c.messageChan:
		if !ok {
			return nil, fmt.Errorf("error reading from messageChan")
		}
		return m, nil
	case <-time.After(10 * time.Second):
		return nil, ErrTimedOut
	}
}

func (c *StunServerConn) AddOtherAddr(addrStr string) error {
	addr2, err := net.ResolveUDPAddr("udp4", addrStr)
	if err != nil {
		return err
	}
	c.OtherAddr = addr2
	return nil
}

// taken from https://github.com/pion/stun/blob/master/cmd/stun-traversal/main.go
func listen(conn *net.UDPConn) chan *stun.Message {
	messages := make(chan *stun.Message)
	go func() {
		for {
			buf := make([]byte, 1024)

			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				close(messages)
				return
			}
			buf = buf[:n]

			m := new(stun.Message)
			m.Raw = buf
			err = m.Decode()
			if err != nil {
				close(messages)
				return
			}

			messages <- m
		}
	}()
	return messages
}
