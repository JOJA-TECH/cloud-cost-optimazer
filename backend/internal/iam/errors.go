package iam

import (
	"errors"
	"fmt"
)

// Domain errors - returned by IAM package, wrap with context in callers.
var (
	ErrInvalidCredentials     = errors.New("invalid credentials")
	ErrUserNotFound           = errors.New("user not found")
	ErrUserExists             = errors.New("user already exists")
	ErrUserInactive           = errors.New("user account is inactive")
	ErrEmailNotVerified       = errors.New("email not verified")
	ErrTokenExpired           = errors.New("token expired")
	ErrTokenInvalid           = errors.New("token invalid")
	ErrTokenMalformed         = errors.New("token malformed")
	ErrUnauthorized           = errors.New("unauthorized")
	ErrForbidden              = errors.New("forbidden")
	ErrInsufficientScope      = errors.New("insufficient scope")
	ErrOrgNotFound            = errors.New("organization not found")
	ErrOrgInactive            = errors.New("organization inactive")
	ErrProjectNotFound        = errors.New("project not found")
	ErrProjectInactive        = errors.New("project inactive")
	ErrNotMember              = errors.New("user is not a member of the organization")
	ErrNotProjectMember       = errors.New("user is not a member of the project")
	ErrRoleNotFound           = errors.New("role not found")
	ErrPermissionNotFound     = errors.New("permission not found")
	ErrInvalidRole            = errors.New("invalid role")
	ErrInvalidMembershipRole  = errors.New("invalid membership role (owner|admin|member|viewer)")
	ErrCannotRemoveOwner      = errors.New("cannot remove organization owner")
	ErrLastOwner              = errors.New("organization must have at least one owner")
	ErrProviderNotSupported   = errors.New("identity provider not supported")
	ErrExternalIdentityExists = errors.New("external identity already linked")
	ErrInvalidToken           = errors.New("invalid token")
	ErrMIValidationFailed     = errors.New("managed identity validation failed")
	ErrSPValidationFailed     = errors.New("service principal validation failed")
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
	CodeInvalidCredentials     = "INVALID_CREDENTIALS"
	CodeUserNotFound           = "USER_NOT_FOUND"
	CodeUserExists             = "USER_EXISTS"
	CodeUserInactive           = "USER_INACTIVE"
	CodeEmailNotVerified       = "EMAIL_NOT_VERIFIED"
	CodeTokenExpired           = "TOKEN_EXPIRED"
	CodeTokenInvalid           = "TOKEN_INVALID"
	CodeTokenMalformed         = "TOKEN_MALFORMED"
	CodeUnauthorized           = "UNAUTHORIZED"
	CodeForbidden              = "FORBIDDEN"
	CodeInsufficientScope      = "INSUFFICIENT_SCOPE"
	CodeOrgNotFound            = "ORG_NOT_FOUND"
	CodeOrgInactive            = "ORG_INACTIVE"
	CodeProjectNotFound        = "PROJECT_NOT_FOUND"
	CodeProjectInactive        = "PROJECT_INACTIVE"
	CodeNotMember              = "NOT_MEMBER"
	CodeNotProjectMember       = "NOT_PROJECT_MEMBER"
	CodeRoleNotFound           = "ROLE_NOT_FOUND"
	CodePermissionNotFound     = "PERMISSION_NOT_FOUND"
	CodeInvalidRole            = "INVALID_ROLE"
	CodeInvalidMembershipRole  = "INVALID_MEMBERSHIP_ROLE"
	CodeCannotRemoveOwner      = "CANNOT_REMOVE_OWNER"
	CodeLastOwner              = "LAST_OWNER"
	CodeProviderNotSupported   = "PROVIDER_NOT_SUPPORTED"
	CodeExternalIdentityExists = "EXTERNAL_IDENTITY_EXISTS"
	CodeInvalidToken           = "INVALID_TOKEN"
	CodeMIValidationFailed     = "MI_VALIDATION_FAILED"
	CodeSPValidationFailed     = "SP_VALIDATION_FAILED"
)

// MapErrorToCode converts domain error to API error code.
func MapErrorToCode(err error) string {
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		return CodeInvalidCredentials
	case errors.Is(err, ErrUserNotFound):
		return CodeUserNotFound
	case errors.Is(err, ErrUserExists):
		return CodeUserExists
	case errors.Is(err, ErrUserInactive):
		return CodeUserInactive
	case errors.Is(err, ErrEmailNotVerified):
		return CodeEmailNotVerified
	case errors.Is(err, ErrTokenExpired):
		return CodeTokenExpired
	case errors.Is(err, ErrTokenInvalid):
		return CodeTokenInvalid
	case errors.Is(err, ErrTokenMalformed):
		return CodeTokenMalformed
	case errors.Is(err, ErrUnauthorized):
		return CodeUnauthorized
	case errors.Is(err, ErrForbidden):
		return CodeForbidden
	case errors.Is(err, ErrInsufficientScope):
		return CodeInsufficientScope
	case errors.Is(err, ErrOrgNotFound):
		return CodeOrgNotFound
	case errors.Is(err, ErrOrgInactive):
		return CodeOrgInactive
	case errors.Is(err, ErrProjectNotFound):
		return CodeProjectNotFound
	case errors.Is(err, ErrProjectInactive):
		return CodeProjectInactive
	case errors.Is(err, ErrNotMember):
		return CodeNotMember
	case errors.Is(err, ErrNotProjectMember):
		return CodeNotProjectMember
	case errors.Is(err, ErrRoleNotFound):
		return CodeRoleNotFound
	case errors.Is(err, ErrPermissionNotFound):
		return CodePermissionNotFound
	case errors.Is(err, ErrInvalidRole):
		return CodeInvalidRole
	case errors.Is(err, ErrInvalidMembershipRole):
		return CodeInvalidMembershipRole
	case errors.Is(err, ErrCannotRemoveOwner):
		return CodeCannotRemoveOwner
	case errors.Is(err, ErrLastOwner):
		return CodeLastOwner
	case errors.Is(err, ErrProviderNotSupported):
		return CodeProviderNotSupported
	case errors.Is(err, ErrExternalIdentityExists):
		return CodeExternalIdentityExists
	case errors.Is(err, ErrInvalidToken):
		return CodeInvalidToken
	case errors.Is(err, ErrMIValidationFailed):
		return CodeMIValidationFailed
	case errors.Is(err, ErrSPValidationFailed):
		return CodeSPValidationFailed
	default:
		return "INTERNAL_ERROR"
	}
}
