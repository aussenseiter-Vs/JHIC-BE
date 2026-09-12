package nexxa

import "errors"

var (
	ErrUpstreamUnavailable = errors.New("upstream service unavailable")
	ErrUpstreamTimeout     = errors.New("upstream timed out")
)