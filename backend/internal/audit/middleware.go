package audit

import (
	"log/slog"
	"net/http"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/google/uuid"
)

func TraceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestValue := r.Header.Get("X-Request-ID")
		requestID, invalidRequestID := iam.NormalizeTraceID(requestValue)
		if invalidRequestID {
			logInvalidTraceID("X-Request-ID", requestValue, requestID)
		}

		correlationValue := r.Header.Get("X-Correlation-ID")
		correlationID := requestID
		if correlationValue != "" {
			var invalidCorrelationID bool
			correlationID, invalidCorrelationID = iam.NormalizeTraceID(correlationValue)
			if invalidCorrelationID {
				logInvalidTraceID("X-Correlation-ID", correlationValue, correlationID)
			}
		}

		operationID := uuid.New()
		trace := CorrelationContext{
			RequestID:     requestID,
			CorrelationID: correlationID,
			OperationID:   &operationID,
		}
		r.Header.Set("X-Request-ID", requestID.String())
		r.Header.Set("X-Correlation-ID", correlationID.String())
		r.Header.Set("X-Operation-ID", operationID.String())
		w.Header().Set("X-Request-ID", requestID.String())
		w.Header().Set("X-Correlation-ID", correlationID.String())
		w.Header().Set("X-Operation-ID", operationID.String())

		next.ServeHTTP(w, r.WithContext(WithCorrelationContext(r.Context(), trace)))
	})
}

func logInvalidTraceID(header, value string, replacement uuid.UUID) {
	const maxLoggedValueLength = 256
	truncated := len(value) > maxLoggedValueLength
	if truncated {
		value = value[:maxLoggedValueLength]
	}
	slog.Warn("invalid trace identifier replaced",
		"header", header,
		"invalid_value", value,
		"value_truncated", truncated,
		"replacement", replacement.String(),
	)
}
