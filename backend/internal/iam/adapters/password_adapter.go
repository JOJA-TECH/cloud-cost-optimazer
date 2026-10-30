package adapters

import (
	"context"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/google/uuid"
)

// PasswordAdapter implements AuthenticationProvider for email/password (dev/local only).
type PasswordAdapter struct {
	repo         iam.IdentityRepository
	tokenManager *iam.TokenManager
}

// NewPasswordAdapter creates a new PasswordAdapter.
func NewPasswordAdapter(repo iam.IdentityRepository, tokenManager *iam.TokenManager) *PasswordAdapter {
	return &PasswordAdapter{repo: repo, tokenManager: tokenManager}
}

// Authenticate verifies email/password and returns IdentityContext.
// Implements AuthenticationProvider for "password" credential type.
func (a *PasswordAdapter) Authenticate(ctx context.Context, cred iam.Credentials) (iam.IdentityContext, error) {
	if cred.Type != "password" {
		return iam.IdentityContext{}, iam.ErrProviderNotSupported
	}
	if cred.Email == "" || cred.Password == "" {
		return iam.IdentityContext{}, iam.ErrInvalidCredentials
	}

	user, err := a.repo.FindUserByEmail(ctx, cred.Email)
	if err != nil {
		return iam.IdentityContext{}, iam.ErrInvalidCredentials
	}

	if user.Status != "active" {
		return iam.IdentityContext{}, iam.ErrUserInactive
	}

	// Verify password hash
	hash, err := a.repo.FindPasswordHash(ctx, user.ID)
	if err != nil {
		return iam.IdentityContext{}, iam.ErrInvalidCredentials
	}
	if err := iam.VerifyPassword(hash, cred.Password); err != nil {
		return iam.IdentityContext{}, iam.ErrInvalidCredentials
	}

	// Resolve org context
	orgID, orgRole, _ := a.resolveDefaultOrg(ctx, user.ID)

	perms, _ := iam.NewMembershipResolver(a.repo).ResolvePermissions(ctx, user.ID, orgID, nil)

	ic := iam.IdentityContext{
		UserID:         user.ID,
		Email:          user.Email,
		DisplayName:    user.DisplayName,
		AuthMethod:     "password",
		SystemRoles:    []string{}, // resolved via resolver
		OrgID:          orgID,
		OrgRole:        orgRole,
		ProjectID:      nil,
		ProjectRole:    nil,
		Permissions:    perms,
		RequestID:      uuid.New().String(),
		CorrelationID:  uuid.New().String(),
		TokenExpiresAt: 0, // set by token manager
	}

	return ic, nil
}

func (a *PasswordAdapter) resolveDefaultOrg(ctx context.Context, userID uuid.UUID) (uuid.UUID, string, error) {
	memberships, err := a.repo.ListMembershipsByUser(ctx, userID)
	if err != nil || len(memberships) == 0 {
		return uuid.Nil, "viewer", nil
	}
	for _, m := range memberships {
		if m.Status == "active" {
			return m.OrgID, m.MembershipRole, nil
		}
	}
	return memberships[0].OrgID, memberships[0].MembershipRole, nil
}
