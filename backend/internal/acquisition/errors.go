package acquisition

import (
	"errors"
	"fmt"
)

// Domain errors - returned by the acquisition package, wrapped with context
// by callers. MapErrorToCode converts them to API error codes.
var (
	ErrStorageUnavailable     = errors.New("acquisition storage unavailable (CCO_DATA_DSN not configured)")
	ErrCycleInProgress        = errors.New("acquisition cycle already in progress")
	ErrResourceNotFound       = errors.New("resource not found")
	ErrRunNotFound            = errors.New("acquisition run not found")
	ErrInvalidInput           = errors.New("invalid request")
	ErrCredentialsUnavailable = errors.New("credentials unavailable")
)

// ErrorWithCode wraps an error with a machine-readable code for API responses.
type ErrorWithCode struct {
	Code    string
	Message string
	Err     error
}

func (e *ErrorWithCode) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *ErrorWithCode) Unwrap() error {
	return e.Err
}

// NewError creates an ErrorWithCode.
func NewError(code, message string, err error) *ErrorWithCode {
	return &ErrorWithCode{Code: code, Message: message, Err: err}
}

// Standard error codes for API responses.
const (
	CodeStorageUnavailable     = "STORAGE_UNAVAILABLE"
	CodeCycleInProgress        = "CYCLE_IN_PROGRESS"
	CodeResourceNotFound       = "RESOURCE_NOT_FOUND"
	CodeRunNotFound            = "ACQUISITION_RUN_NOT_FOUND"
	CodeInvalidRequest         = "INVALID_REQUEST"
	CodeCredentialsUnavailable = "CREDENTIALS_UNAVAILABLE"
)

// MapErrorToCode converts a domain error to an API error code.
func MapErrorToCode(err error) string {
	switch {
	case errors.Is(err, ErrStorageUnavailable):
		return CodeStorageUnavailable
	case errors.Is(err, ErrCycleInProgress):
		return CodeCycleInProgress
	case errors.Is(err, ErrResourceNotFound):
		return CodeResourceNotFound
	case errors.Is(err, ErrRunNotFound):
		return CodeRunNotFound
	case errors.Is(err, ErrInvalidInput):
		return CodeInvalidRequest
	case errors.Is(err, ErrCredentialsUnavailable):
		return CodeCredentialsUnavailable
	default:
		return "INTERNAL_ERROR"
	}
}
