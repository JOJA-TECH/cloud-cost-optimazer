package ai

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound           = errors.New("ai: not found")
	ErrInvalidQuery       = errors.New("ai: invalid query")
	ErrStorageUnavailable = errors.New("ai storage unavailable (CCO_DATA_DSN not configured)")
	ErrMLUnavailable      = errors.New("ML service unavailable")
	ErrContractMismatch   = errors.New("ML contract version mismatch")
	ErrLLMUnavailable     = errors.New("LLM service unavailable")
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
	CodeNotFound           = "AI_NOT_FOUND"
	CodeInvalidQuery       = "AI_INVALID_QUERY"
	CodeStorageUnavailable = "AI_STORAGE_UNAVAILABLE"
	CodeMLUnavailable      = "AI_ML_UNAVAILABLE"
	CodeContractMismatch   = "AI_CONTRACT_MISMATCH"
	CodeLLMUnavailable     = "AI_LLM_UNAVAILABLE"
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
	case errors.Is(err, ErrLLMUnavailable):
		return CodeLLMUnavailable
	default:
		return "INTERNAL_ERROR"
	}
}
