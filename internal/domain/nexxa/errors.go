package nexxa

import "errors"

var (
	ErrN8NUnavailable = errors.New("upstream service unavailable")
	ErrN8NTimeout     = errors.New("upstream timed out")
)
