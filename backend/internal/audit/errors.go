package audit

import "errors"

var (
	ErrIdentityContextMissing = errors.New("audit identity context is missing")
	ErrInvalidIdentityContext = errors.New("audit identity context is invalid")
	ErrInvalidTraceContext    = errors.New("audit trace context is invalid")
	ErrAuditRepository        = errors.New("audit repository is unavailable")
	ErrAuditLogger            = errors.New("audit logger is unavailable")
	ErrInvalidAuditEvent      = errors.New("invalid audit event")
	ErrEventAlreadyPersisted  = errors.New("audit event already has a persistence ID")
)
