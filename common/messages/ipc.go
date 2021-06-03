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
)
