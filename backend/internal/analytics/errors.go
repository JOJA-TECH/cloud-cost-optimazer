package analytics

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound           = errors.New("analytics: not found")
	ErrInvalidQuery       = errors.New("analytics: invalid query")
	ErrStorageUnavailable = errors.New("analytics storage unavailable (CCO_DATA_DSN not configured)")
	ErrMLUnavailable      = errors.New("ML service unavailable")
	ErrContractMismatch   = errors.New("ML contract version mismatch")
)

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

func NewError(code, message string, err error) *ErrorWithCode {
	return &ErrorWithCode{Code: code, Message: message, Err: err}
}

const (
	CodeNotFound           = "ANALYTICS_NOT_FOUND"
	CodeInvalidQuery       = "ANALYTICS_INVALID_QUERY"
	CodeStorageUnavailable = "ANALYTICS_STORAGE_UNAVAILABLE"
	CodeMLUnavailable      = "ANALYTICS_ML_UNAVAILABLE"
	CodeContractMismatch   = "ANALYTICS_CONTRACT_MISMATCH"
)

func MapErrorToCode(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return CodeNotFound
	case errors.Is(err, ErrInvalidQuery):
		return CodeInvalidQuery
	case errors.Is(err, ErrStorageUnavailable):
		return CodeStorageUnavailable
	case errors.Is(err, ErrMLUnavailable):
		return CodeMLUnavailable
	case errors.Is(err, ErrContractMismatch):
		return CodeContractMismatch
	default:
		return "INTERNAL_ERROR"
	}
}
