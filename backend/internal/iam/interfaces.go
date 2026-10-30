package iam

import (
	"context"

	"github.com/google/uuid"
)

// IdentityContext travels in request context after auth middleware.
// Contains everything downstream handlers need for authorization.
type IdentityContext struct {
	// Identity
	UserID      uuid.UUID
	Email       string
	DisplayName string
	AuthMethod  string // "oidc" | "password" | "service_principal" | "managed_identity"

	// Effective RBAC
	SystemRoles []string   // platform_admin, db_admin, analyst, viewer
	OrgID       uuid.UUID  // active organization (validated membership)
	OrgRole     string     // owner | admin | member | viewer
	ProjectID   *uuid.UUID // active project (validated membership)
	ProjectRole *string    // role in project (inherits from org or override)
	Permissions []string   // effective permission_codes (cached)

	// Traceability (RNF-13)
	RequestID     string
	CorrelationID string

	// Token expiry
	TokenExpiresAt int64 // unix ms
}

// Context key for IdentityContext.
type contextKey string

const identityContextKey contextKey = "iam.identity"

// AuthenticationProvider authenticates credentials and returns an IdentityContext.
// Implementations: OIDC (Entra ID), Password (dev), Service Principal, Managed Identity.
type AuthenticationProvider interface {
	Authenticate(ctx context.Context, cred Credentials) (IdentityContext, error)
}

// Credentials represents authentication input.
type Credentials struct {
	Type         string // "password" | "oidc" | "client_credentials" | "managed_identity"
	Email        string // password / oidc
	Password     string // password
	IDToken      string // oidc
	ClientID     string // client_credentials
	ClientSecret string // client_credentials
	Scope        string // client_credentials
	MIToken      string // managed_identity
	MIClientID   string // managed_identity (user-assigned MI client ID)
}

// TokenValidator validates a bearer token and returns IdentityContext.
// Used by auth middleware for every authenticated request.
// Implemented by TokenManager (HS256 self-issued) and OIDCAdapter.
type TokenValidator interface {
	Validate(ctx context.Context, token string) (IdentityContext, error)
}

// PermissionChecker checks if an identity can perform an action on a resource.
// Resource examples: "cloud_accounts", "resources", "recommendations", "reports", "organizations", "projects", "members".
// Action examples: "read", "write", "delete", "manage", "create", "invite".
type PermissionChecker interface {
	Can(ctx context.Context, identity IdentityContext, resource, action string) (bool, error)
}

// AuthorizationService resolves effective permissions for an identity.
// Combines system roles + org membership + project membership.
type AuthorizationService interface {
	ResolvePermissions(ctx context.Context, userID uuid.UUID, orgID uuid.UUID, projectID *uuid.UUID) ([]string, error)
	ResolveOrgRole(ctx context.Context, userID, orgID uuid.UUID) (string, error)
	ResolveProjectRole(ctx context.Context, userID, projectID uuid.UUID) (string, error)
}

// IdentityRepository groups all persistence contracts for IAM.
// Implementations use GORM against cco_app.
// Method names are unique across the embedded interfaces so the composite
// compiles; this is why they carry explicit prefixes (FindUserByID, FindRoleByCode...).
type IdentityRepository interface {
	UserRepository
	RoleRepository
	PermissionRepository
	UserRoleRepository
	ExternalIdentityRepository
	MembershipRepository
	ProjectRepository
}

// UserRepository manages users.
type UserRepository interface {
	CreateUser(ctx context.Context, input CreateUserInput) (UserDTO, error)
	FindUserByID(ctx context.Context, id uuid.UUID) (UserDTO, error)
	FindUserByEmail(ctx context.Context, email string) (UserDTO, error)
	FindPasswordHash(ctx context.Context, userID uuid.UUID) (string, error)
	UpdateUser(ctx context.Context, id uuid.UUID, input UpdateUserInput) (UserDTO, error)
	DeleteUser(ctx context.Context, id uuid.UUID) error
	ListUsers(ctx context.Context, filter UserListFilter) ([]UserDTO, error)
}

type CreateUserInput struct {
	Email        string
	DisplayName  string
	PasswordHash *string // dev-only; nil for OIDC users
}

type UpdateUserInput struct {
	DisplayName   *string
	Status        *string
	EmailVerified *bool
}

type UserListFilter struct {
	Status *string
	OrgID  *uuid.UUID
	Limit  int
	Offset int
}

type UserDTO struct {
	ID            uuid.UUID
	Email         string
	DisplayName   string
	Status        string
	EmailVerified bool
	LastLoginAt   *int64 // unix ms
	CreatedAt     int64
	UpdatedAt     int64
}

// RoleRepository manages system roles (platform_admin, db_admin, analyst, viewer).
type RoleRepository interface {
	FindRoleByCode(ctx context.Context, code string) (RoleDTO, error)
	FindRoleByID(ctx context.Context, id int16) (RoleDTO, error)
	ListRoles(ctx context.Context) ([]RoleDTO, error)
}

type RoleDTO struct {
	RoleID       int16
	RoleCode     string
	Description  *string
	IsSystemRole bool
	CreatedAt    int64
}

// PermissionRepository manages permission codes.
type PermissionRepository interface {
	FindPermissionByCode(ctx context.Context, code string) (PermissionDTO, error)
	FindPermissionByID(ctx context.Context, id int16) (PermissionDTO, error)
	ListPermissions(ctx context.Context) ([]PermissionDTO, error)
	ListPermissionsByRole(ctx context.Context, roleID int16) ([]PermissionDTO, error)
}

type PermissionDTO struct {
	PermissionID   int16
	PermissionCode string
	Description    *string
	CreatedAt      int64
}

// UserRoleRepository manages user <-> system role assignments.
type UserRoleRepository interface {
	AssignRole(ctx context.Context, userID uuid.UUID, roleID int16, assignedBy *uuid.UUID) error
	RevokeRole(ctx context.Context, userID uuid.UUID, roleID int16) error
	ListUserRolesByUser(ctx context.Context, userID uuid.UUID) ([]UserRoleDTO, error)
	ListUserRolesByRole(ctx context.Context, roleID int16) ([]UserRoleDTO, error)
}

type UserRoleDTO struct {
	UserID     uuid.UUID
	RoleID     int16
	AssignedAt int64
	AssignedBy *uuid.UUID
}

// ExternalIdentityRepository manages OIDC / federated identity mappings.
type ExternalIdentityRepository interface {
	LinkExternalIdentity(ctx context.Context, input CreateExternalIdentityInput) (ExternalIdentityDTO, error)
	FindExternalIdentityByProviderSubject(ctx context.Context, provider, subject string) (ExternalIdentityDTO, error)
	FindExternalIdentitiesByUser(ctx context.Context, userID uuid.UUID) ([]ExternalIdentityDTO, error)
	UpdateExternalIdentityLastSeen(ctx context.Context, id uuid.UUID) error
}

type CreateExternalIdentityInput struct {
	UserID   uuid.UUID
	Provider string
	Subject  string
}

type ExternalIdentityDTO struct {
	ExternalIdentityID uuid.UUID
	UserID             uuid.UUID
	Provider           string
	Subject            string
	CreatedAt          int64
	LastSeenAt         *int64
}

// MembershipRepository manages organization members and their roles.
type MembershipRepository interface {
	AddMember(ctx context.Context, input AddMemberInput) (MembershipDTO, error)
	UpdateMember(ctx context.Context, orgID, userID uuid.UUID, input UpdateMemberInput) (MembershipDTO, error)
	RemoveMember(ctx context.Context, orgID, userID uuid.UUID) error
	ListMembershipsByOrg(ctx context.Context, orgID uuid.UUID) ([]MembershipDTO, error)
	ListMembershipsByUser(ctx context.Context, userID uuid.UUID) ([]MembershipDTO, error)
	FindMembership(ctx context.Context, orgID, userID uuid.UUID) (MembershipDTO, error)
}

type AddMemberInput struct {
	OrgID          uuid.UUID
	UserID         uuid.UUID
	MembershipRole string // owner | admin | member | viewer
	InvitedBy      *uuid.UUID
}

type UpdateMemberInput struct {
	MembershipRole *string
	Status         *string
}

type MembershipDTO struct {
	OrgID          uuid.UUID
	UserID         uuid.UUID
	MembershipRole string
	Status         string
	JoinedAt       int64
	InvitedBy      *uuid.UUID
}

// ProjectRepository manages projects and project membership (for auth context).
type ProjectRepository interface {
	FindProjectByID(ctx context.Context, id uuid.UUID) (ProjectDTO, error)
	ListProjectsByOrg(ctx context.Context, orgID uuid.UUID) ([]ProjectDTO, error)
	CheckUserProjectAccess(ctx context.Context, userID, projectID uuid.UUID) (bool, error)
}

type ProjectDTO struct {
	ProjectID      uuid.UUID
	OrganizationID uuid.UUID
	Name           string
	Description    *string
	Status         string
	CreatedAt      int64
	UpdatedAt      int64
}
