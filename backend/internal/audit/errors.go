package audit

import "errors"

var (
	ErrIdentityContextMissing = errors.New("audit identity context is missing")
	ErrInvalidIdentityContext = errors.New("audit identity context is invalid")
	ErrInvalidTraceContext    = errors.New("audit trace context is invalid")
)
