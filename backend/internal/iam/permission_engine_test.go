package iam_test

import (
	"context"
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
)

// TestPermissionEngineCan covers Can() wildcard and exact-match behavior
// using only roles from the static rolePermission maps, so each case is
// isolated (OrgRole "" and ProjectRole nil grant nothing).
func TestPermissionEngineCan(t *testing.T) {
	engine := iam.NewPermissionEngine()

	tests := []struct {
		name        string
		systemRoles []string
		orgRole     string
		resource    string
		action      string
		want        bool
	}{
		{
			name:        "super wildcard *:* allows any resource:action",
			systemRoles: []string{"platform_admin"},
			resource:    "cloud_accounts",
			action:      "delete",
			want:        true,
		},
		{
			name:     "resource wildcard resources:* allows resources:read",
			orgRole:  "owner",
			resource: "resources",
			action:   "read",
			want:     true,
		},
		{
			name:     "resource wildcard resources:* allows resources:write",
			orgRole:  "owner",
			resource: "resources",
			action:   "write",
			want:     true,
		},
		{
			name:        "exact match resources:read allows resources:read",
			systemRoles: []string{"viewer"},
			resource:    "resources",
			action:      "read",
			want:        true,
		},
		{
			name:        "exact match resources:read does not allow resources:write",
			systemRoles: []string{"viewer"},
			resource:    "resources",
			action:      "write",
			want:        false,
		},
		{
			name:        "no matching permission returns false",
			systemRoles: []string{"viewer"},
			resource:    "cloud_accounts",
			action:      "manage",
			want:        false,
		},
		{
			name:     "empty permissions returns false",
			resource: "resources",
			action:   "read",
			want:     false,
		},
		{
			name:        "platform_admin passes everything",
			systemRoles: []string{"platform_admin"},
			resource:    "reports",
			action:      "write",
			want:        true,
		},
		{
			name:        "viewer passes resources:read",
			systemRoles: []string{"viewer"},
			resource:    "resources",
			action:      "read",
			want:        true,
		},
		{
			name:        "viewer fails resources:write",
			systemRoles: []string{"viewer"},
			resource:    "resources",
			action:      "write",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ic := iam.IdentityContext{
				SystemRoles: tt.systemRoles,
				OrgRole:     tt.orgRole,
			}
			got, err := engine.Can(context.Background(), ic, tt.resource, tt.action)
			if err != nil {
				t.Fatalf("Can(%q, %q) returned error: %v", tt.resource, tt.action, err)
			}
			if got != tt.want {
				t.Errorf("Can(%q, %q) = %v, want %v", tt.resource, tt.action, got, tt.want)
			}
		})
	}
}
