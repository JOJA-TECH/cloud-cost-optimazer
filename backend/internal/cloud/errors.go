package cloud

import (
	"errors"
	"fmt"
)

// Domain errors - returned by cloud package, wrap with context in callers.
var (
	ErrAccountNotFound        = errors.New("account not found")
	ErrAccountExists          = errors.New("account already exists")
	ErrResourceNotFound       = errors.New("resource not found")
	ErrBindingNotFound        = errors.New("data source not found")
	ErrBindingExists          = errors.New("data source already exists")
	ErrProjectNotFound        = errors.New("project not found")
	ErrInvalidInput           = errors.New("invalid request")
	ErrCredentialsUnavailable = errors.New("credentials unavailable")
	ErrConnectionFailed       = errors.New("connection failed")
	ErrProviderUnavailable    = errors.New("provider unavailable")
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
	CodeAccountNotFound        = "ACCOUNT_NOT_FOUND"
	CodeAccountExists          = "ACCOUNT_EXISTS"
	CodeResourceNotFound       = "RESOURCE_NOT_FOUND"
	CodeDataSourceNotFound     = "DATA_SOURCE_NOT_FOUND"
	CodeDataSourceExists       = "DATA_SOURCE_EXISTS"
	CodeProjectNotFound        = "PROJECT_NOT_FOUND"
	CodeInvalidRequest         = "INVALID_REQUEST"
	CodeCredentialsUnavailable = "CREDENTIALS_UNAVAILABLE"
	CodeConnectionFailed       = "CONNECTION_FAILED"
	CodeProviderUnavailable    = "PROVIDER_UNAVAILABLE"
)

// MapErrorToCode converts domain error to API error code.
func MapErrorToCode(err error) string {
	switch {
	case errors.Is(err, ErrAccountNotFound):
		return CodeAccountNotFound
	case errors.Is(err, ErrAccountExists):
		return CodeAccountExists
	case errors.Is(err, ErrResourceNotFound):
		return CodeResourceNotFound
	case errors.Is(err, ErrBindingNotFound):
		return CodeDataSourceNotFound
	case errors.Is(err, ErrBindingExists):
		return CodeDataSourceExists
	case errors.Is(err, ErrProjectNotFound):
		return CodeProjectNotFound
	case errors.Is(err, ErrInvalidInput):
		return CodeInvalidRequest
	case errors.Is(err, ErrCredentialsUnavailable):
		return CodeCredentialsUnavailable
	case errors.Is(err, ErrConnectionFailed):
		return CodeConnectionFailed
	case errors.Is(err, ErrProviderUnavailable):
		return CodeProviderUnavailable
	default:
		return "INTERNAL_ERROR"
	}
}
