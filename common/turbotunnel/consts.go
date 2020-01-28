// Package turbotunnel provides support for overlaying a virtual net.PacketConn
// on some other network carrier.
//
// https://github.com/net4people/bbs/issues/9
package turbotunnel

import "errors"

// The size of receive and send queues.
const queueSize = 32

var errClosedPacketConn = errors.New("operation on closed connection")
var errNotImplemented = errors.New("not implemented")
