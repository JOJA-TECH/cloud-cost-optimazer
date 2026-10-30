package iam_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
)

func TestMapErrorToCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"invalid credentials", iam.ErrInvalidCredentials, iam.CodeInvalidCredentials},
		{"user not found", iam.ErrUserNotFound, iam.CodeUserNotFound},
		{"user already exists", iam.ErrUserExists, iam.CodeUserExists},
		{"user inactive", iam.ErrUserInactive, iam.CodeUserInactive},
		{"email not verified", iam.ErrEmailNotVerified, iam.CodeEmailNotVerified},
		{"token expired", iam.ErrTokenExpired, iam.CodeTokenExpired},
		{"token invalid", iam.ErrTokenInvalid, iam.CodeTokenInvalid},
		{"token malformed", iam.ErrTokenMalformed, iam.CodeTokenMalformed},
		{"unauthorized", iam.ErrUnauthorized, iam.CodeUnauthorized},
		{"forbidden", iam.ErrForbidden, iam.CodeForbidden},
		{"insufficient scope", iam.ErrInsufficientScope, iam.CodeInsufficientScope},
		{"organization not found", iam.ErrOrgNotFound, iam.CodeOrgNotFound},
		{"organization inactive", iam.ErrOrgInactive, iam.CodeOrgInactive},
		{"project not found", iam.ErrProjectNotFound, iam.CodeProjectNotFound},
		{"project inactive", iam.ErrProjectInactive, iam.CodeProjectInactive},
		{"not member", iam.ErrNotMember, iam.CodeNotMember},
		{"not project member", iam.ErrNotProjectMember, iam.CodeNotProjectMember},
		{"role not found", iam.ErrRoleNotFound, iam.CodeRoleNotFound},
		{"permission not found", iam.ErrPermissionNotFound, iam.CodePermissionNotFound},
		{"invalid role", iam.ErrInvalidRole, iam.CodeInvalidRole},
		{"invalid membership role", iam.ErrInvalidMembershipRole, iam.CodeInvalidMembershipRole},
		{"cannot remove owner", iam.ErrCannotRemoveOwner, iam.CodeCannotRemoveOwner},
		{"last owner", iam.ErrLastOwner, iam.CodeLastOwner},
		{"provider not supported", iam.ErrProviderNotSupported, iam.CodeProviderNotSupported},
		{"external identity exists", iam.ErrExternalIdentityExists, iam.CodeExternalIdentityExists},
		{"invalid token", iam.ErrInvalidToken, iam.CodeInvalidToken},
		{"managed identity validation failed", iam.ErrMIValidationFailed, iam.CodeMIValidationFailed},
		{"service principal validation failed", iam.ErrSPValidationFailed, iam.CodeSPValidationFailed},
		{"unknown error maps to INTERNAL_ERROR", errors.New("unexpected failure"), "INTERNAL_ERROR"},
		{"wrapped error still resolves via errors.Is", fmt.Errorf("upstream: %w", iam.ErrForbidden), iam.CodeForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := iam.MapErrorToCode(tt.err); got != tt.want {
				t.Errorf("MapErrorToCode(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}
