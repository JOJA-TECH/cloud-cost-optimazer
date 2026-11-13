package data

import (
	"errors"
	"fmt"
)

// Domain errors - returned by the data package, wrapped with context by
// callers. MapErrorToCode converts them to API error codes.
var (
	ErrNotFound           = errors.New("data: not found")
	ErrInvalidQuery       = errors.New("data: invalid query")
	ErrInvalidCostRecord  = errors.New("data: invalid cost record")
	ErrStorageUnavailable = errors.New("data storage unavailable (CCO_DATA_DSN not configured)")
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
	CodeNotFound           = "DATA_NOT_FOUND"
	CodeInvalidQuery       = "DATA_INVALID_QUERY"
	CodeInvalidCostRecord  = "DATA_INVALID_COST"
	CodeStorageUnavailable = "DATA_STORAGE_UNAVAILABLE"
)

// MapErrorToCode converts a domain error to an API error code.
func MapErrorToCode(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return CodeNotFound
	case errors.Is(err, ErrInvalidQuery):
		return CodeInvalidQuery
	case errors.Is(err, ErrInvalidCostRecord):
		return CodeInvalidCostRecord
	case errors.Is(err, ErrStorageUnavailable):
		return CodeStorageUnavailable
	default:
		return "INTERNAL_ERROR"
	}
}
