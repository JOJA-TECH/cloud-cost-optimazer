package audit

import (
	"context"
	"fmt"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/google/uuid"
)

type correlationContextKey struct{}

func WithCorrelationContext(ctx context.Context, trace CorrelationContext) context.Context {
	return context.WithValue(ctx, correlationContextKey{}, trace)
}

func CorrelationContextFromContext(ctx context.Context) (CorrelationContext, bool) {
	identity, hasIdentity := iam.IdentityContextFromContext(ctx)
	trace, hasTrace := ctx.Value(correlationContextKey{}).(CorrelationContext)

	if hasIdentity {
		requestID, err := uuid.Parse(identity.RequestID)
		if err != nil {
			return CorrelationContext{}, false
		}
		correlationID, err := uuid.Parse(identity.CorrelationID)
		if err != nil {
			return CorrelationContext{}, false
		}
		result := CorrelationContext{
			RequestID:     requestID,
			CorrelationID: correlationID,
		}
		if hasTrace {
			result.OperationID = trace.OperationID
		}
		return result, true
	}

	if hasTrace {
		return trace, true
	}
	return CorrelationContext{}, false
}

func AuditIdentityContextFromContext(ctx context.Context) (AuditIdentityContext, error) {
	identity, ok := iam.IdentityContextFromContext(ctx)
	if !ok {
		return AuditIdentityContext{}, ErrIdentityContextMissing
	}
	if identity.UserID == uuid.Nil {
		return AuditIdentityContext{}, fmt.Errorf("%w: user ID is empty", ErrInvalidIdentityContext)
	}
	trace, ok := CorrelationContextFromContext(ctx)
	if !ok {
		return AuditIdentityContext{}, ErrInvalidTraceContext
	}

	actorType := ActorTypeUser
	switch identity.AuthMethod {
	case ActorTypeServicePrincipal:
		actorType = ActorTypeServicePrincipal
	case ActorTypeManagedIdentity:
		actorType = ActorTypeManagedIdentity
	}
	userID := identity.UserID
	result := AuditIdentityContext{
		Actor: Actor{
			UserID:     &userID,
			ActorType:  actorType,
			AuthMethod: identity.AuthMethod,
		},
		Trace: trace,
	}
	if identity.OrgID != uuid.Nil {
		organizationID := identity.OrgID
		result.OrganizationID = &organizationID
	}
	return result, nil
}

type ContextTrace struct{}

func (ContextTrace) FromContext(ctx context.Context) (CorrelationContext, bool) {
	return CorrelationContextFromContext(ctx)
}

func (ContextTrace) WithContext(ctx context.Context, trace CorrelationContext) context.Context {
	return WithCorrelationContext(ctx, trace)
}

var _ TraceContext = ContextTrace{}
