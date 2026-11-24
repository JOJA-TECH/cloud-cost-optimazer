package analytics

import (
	"errors"
	"fmt"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/ai"
)

var (
	ErrNotFound           = errors.New("analytics: not found")
	ErrInvalidQuery       = errors.New("analytics: invalid query")
	ErrStorageUnavailable = errors.New("analytics storage unavailable (CCO_DATA_DSN not configured)")
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
	CodeNotFound           = "ANALYTICS_NOT_FOUND"
	CodeInvalidQuery       = "ANALYTICS_INVALID_QUERY"
	CodeStorageUnavailable = "ANALYTICS_STORAGE_UNAVAILABLE"
	CodeMLUnavailable      = "ANALYTICS_ML_UNAVAILABLE"
	CodeContractMismatch   = "ANALYTICS_CONTRACT_MISMATCH"
	CodeLLMUnavailable     = "ANALYTICS_LLM_UNAVAILABLE"
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

// fromAIError convierte un error proveniente de la frontera ai.MLPort a un
// error de analytics, para que cada paquete mantenga sus propios códigos
// diferenciables y un llamador de analytics pueda distinguir, por ejemplo,
// ML no disponible de un contrato mal versionado usando codes de analytics.
func fromAIError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ai.ErrMLUnavailable):
		return NewError(CodeMLUnavailable, err.Error(), ErrMLUnavailable)
	case errors.Is(err, ai.ErrContractMismatch):
		return NewError(CodeContractMismatch, err.Error(), ErrContractMismatch)
	case errors.Is(err, ai.ErrInvalidQuery):
		return NewError(CodeInvalidQuery, err.Error(), ErrInvalidQuery)
	case errors.Is(err, ai.ErrNotFound):
		return NewError(CodeNotFound, err.Error(), ErrNotFound)
	default:
		return NewError("INTERNAL_ERROR", err.Error(), err)
	}
}
