//Package for a safer logging wrapper around the standard logging package

//import "git.torproject.org/pluggable-transports/snowflake.git/common/safelog"
package safelog

import (
	"bytes"
	"io"
	"regexp"
	"sync"
)

const ipv4Address = `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`
const ipv6Address = `([0-9a-fA-F]{0,4}:){5,7}([0-9a-fA-F]{0,4})?`
const ipv6Compressed = `([0-9a-fA-F]{0,4}:){0,5}([0-9a-fA-F]{0,4})?(::)([0-9a-fA-F]{0,4}:){0,5}([0-9a-fA-F]{0,4})?`
const ipv6Full = `(` + ipv6Address + `(` + ipv4Address + `))` +
	`|(` + ipv6Compressed + `(` + ipv4Address + `))` +
	`|(` + ipv6Address + `)` + `|(` + ipv6Compressed + `)`
const optionalPort = `(:\d{1,5})?`
const addressPattern = `((` + ipv4Address + `)|(\[(` + ipv6Full + `)\])|(` + ipv6Full + `))` + optionalPort
const fullAddrPattern = `(^|\s|[^\w:])` + addressPattern + `(\s|(:\s)|[^\w:]|$)`

var scrubberPatterns = []*regexp.Regexp{
	regexp.MustCompile(fullAddrPattern),
}

var addressRegexp = regexp.MustCompile(addressPattern)

// An io.Writer that can be used as the output for a logger that first
// sanitizes logs and then writes to the provided io.Writer
type LogScrubber struct {
	Output io.Writer
	buffer []byte

	lock sync.Mutex
}

func (ls *LogScrubber) Lock()   { (*ls).lock.Lock() }
func (ls *LogScrubber) Unlock() { (*ls).lock.Unlock() }

func scrub(b []byte) []byte {
	scrubbedBytes := b
	for _, pattern := range scrubberPatterns {
		// this is a workaround since go does not yet support look ahead or look
		// behind for regular expressions.
		scrubbedBytes = pattern.ReplaceAllFunc(scrubbedBytes, func(b []byte) []byte {
			return addressRegexp.ReplaceAll(b, []byte("[scrubbed]"))
		})
	}
	return scrubbedBytes
}

func (ls *LogScrubber) Write(b []byte) (n int, err error) {
	ls.Lock()
	defer ls.Unlock()

	n = len(b)
	ls.buffer = append(ls.buffer, b...)
	for {
		i := bytes.LastIndexByte(ls.buffer, '\n')
		if i == -1 {
			return
		}
		fullLines := ls.buffer[:i+1]
		_, err = ls.Output.Write(scrub(fullLines))
		if err != nil {
			return
		}
		ls.buffer = ls.buffer[i+1:]
	}
}
