package messages

import (
	"errors"
)

type Arg struct {
	Body       []byte
	RemoteAddr string
	NatType    string
}

var (
	ErrBadRequest  = errors.New("bad request")
	ErrInternal    = errors.New("internal error")
	ErrUnavailable = errors.New("service unavailable")
	ErrTimeout     = errors.New("timeout")
)
