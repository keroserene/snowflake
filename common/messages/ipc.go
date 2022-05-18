package messages

import (
	"errors"
)

type Arg struct {
	Body       []byte
	RemoteAddr string
}

var (
	ErrBadRequest = errors.New("bad request")
	ErrInternal   = errors.New("internal error")
	ErrExtraInfo  = errors.New("client sent extra info")

	StrTimedOut  = "timed out waiting for answer!"
	StrNoProxies = "no snowflake proxies currently available"
)
