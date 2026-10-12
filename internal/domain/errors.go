package domain

import (
	"errors"
	"strconv"
)

// Sentinel errors returned by service-layer code. HTTP handlers
// (internal/api/*) map these to the appropriate status code and
// OpenAI-style error envelope.
var (
	ErrNotFound           = errors.New("resource not found")
	ErrUnauthorized       = errors.New("invalid or inactive api key")
	ErrRateLimited        = errors.New("rate limit exceeded")
	ErrBudgetExceeded     = errors.New("budget exceeded")
	ErrNoHealthyCandidate = errors.New("no healthy routing candidate available")
	ErrUpstream           = errors.New("upstream provider error")
	ErrInvalidArgument    = errors.New("invalid argument")
	// ErrContentBlocked means a content filter rule rejected the request.
	ErrContentBlocked = errors.New("content blocked by filter policy")
	// ErrModelNotAllowed means the key's model allowlist excludes the model.
	ErrModelNotAllowed = errors.New("model not allowed for this api key")
	// ErrIPNotAllowed means the caller's address is not on the key's whitelist.
	ErrIPNotAllowed = errors.New("source ip not allowed for this api key")
)

// UpstreamError is a failed call to a supplier's API. It matches ErrUpstream
// with errors.Is, and keeps the HTTP status (0 = no response: network error
// or timeout) so availability tests can classify the failure.
type UpstreamError struct {
	Status  int
	Message string
}

func (e *UpstreamError) Error() string {
	if e.Status == 0 {
		return ErrUpstream.Error() + ": " + e.Message
	}
	return ErrUpstream.Error() + ": status " + strconv.Itoa(e.Status) + ": " + e.Message
}

func (e *UpstreamError) Unwrap() error { return ErrUpstream }
