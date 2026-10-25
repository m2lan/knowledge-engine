package domain

import "errors"

// Sentinel errors used by the services so transports can map failures to the
// right status code without string matching.
var (
	ErrNotFound     = errors.New("not found")
	ErrForbidden    = errors.New("forbidden")
	ErrInvalidInput = errors.New("invalid input")
)
