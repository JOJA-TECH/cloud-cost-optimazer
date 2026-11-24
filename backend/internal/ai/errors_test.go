package ai

import (
	"errors"
	"testing"
)

func TestMapErrorToCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"not found", ErrNotFound, CodeNotFound},
		{"invalid query", ErrInvalidQuery, CodeInvalidQuery},
		{"storage unavailable", ErrStorageUnavailable, CodeStorageUnavailable},
		{"ML unavailable", ErrMLUnavailable, CodeMLUnavailable},
		{"contract mismatch", ErrContractMismatch, CodeContractMismatch},
		{"LLM unavailable", ErrLLMUnavailable, CodeLLMUnavailable},
		{"wrapped ML unavailable", errors.New("ai: ML service unavailable"), "INTERNAL_ERROR"},
		{"wrapped with Unwrap", fmtWrappedErr(), CodeMLUnavailable},
		{"nil error", nil, "INTERNAL_ERROR"},
		{"unknown error", errors.New("something else"), "INTERNAL_ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapErrorToCode(tt.err)
			if got != tt.want {
				t.Errorf("MapErrorToCode(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

// fmtWrappedErr returns an error that unwraps to ErrMLUnavailable, exercising
// the errors.Is path in MapErrorToCode.
func fmtWrappedErr() error {
	return &wrappedErr{inner: ErrMLUnavailable}
}

type wrappedErr struct {
	inner error
}

func (e *wrappedErr) Error() string   { return e.inner.Error() }
func (e *wrappedErr) Unwrap() error   { return e.inner }
func (e *wrappedErr) Is(target error) bool { return errors.Is(e.inner, target) }
