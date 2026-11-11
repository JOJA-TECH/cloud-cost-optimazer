package iam

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type stubOIDCAdapter struct{}

func (stubOIDCAdapter) AuthURL(state string) string {
	return "https://idp.example.com/authorize?state=" + url.QueryEscape(state)
}

func (stubOIDCAdapter) ExchangeCode(ctx context.Context, code string) (IdentityContext, error) {
	return IdentityContext{UserID: uuid.New(), Email: "user@example.com", AuthMethod: "oidc"}, nil
}

func TestOIDCLoginHandlerPerRequestState(t *testing.T) {
	h := NewOIDCLoginHandler(stubOIDCAdapter{}, "", false)

	states := make([]string, 2)
	for i := range states {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/oidc/login", nil))

		if rec.Code != http.StatusFound {
			t.Fatalf("req %d: status = %d, want 302", i, rec.Code)
		}
		var stateCookie *http.Cookie
		for _, c := range rec.Result().Cookies() {
			if c.Name == "cco_oidc_state" {
				stateCookie = c
			}
		}
		if stateCookie == nil {
			t.Fatalf("req %d: cco_oidc_state cookie not set", i)
		}
		loc, err := url.Parse(rec.Header().Get("Location"))
		if err != nil {
			t.Fatalf("req %d: bad Location: %v", i, err)
		}
		urlState := loc.Query().Get("state")
		if urlState != stateCookie.Value {
			t.Fatalf("req %d: URL state %q != cookie state %q", i, urlState, stateCookie.Value)
		}
		states[i] = urlState
	}
	if states[0] == states[1] {
		t.Fatal("state must differ between requests")
	}
}

func TestOIDCCallbackHandlerStateMismatch(t *testing.T) {
	h := NewOIDCCallbackHandler(stubOIDCAdapter{}, NewTokenManager(DefaultTokenConfig("test-secret"), nil), "", false)

	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?code=abc&state=bad", nil)
	req.AddCookie(&http.Cookie{Name: "cco_oidc_state", Value: "good"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestOIDCCallbackHandlerSuccess(t *testing.T) {
	h := NewOIDCCallbackHandler(stubOIDCAdapter{}, NewTokenManager(DefaultTokenConfig("test-secret"), nil), "", false)

	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?code=abc&state=good", nil)
	req.AddCookie(&http.Cookie{Name: "cco_oidc_state", Value: "good"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("status/location = %d/%q, want 302 to /", rec.Code, rec.Header().Get("Location"))
	}
	cookies := rec.Result().Cookies()
	var hasAccess, hasRefresh, hasState bool
	for _, c := range cookies {
		switch c.Name {
		case "cco_access":
			hasAccess = c.Value != ""
		case "cco_refresh":
			hasRefresh = c.Value != ""
		case "cco_oidc_state":
			hasState = c.MaxAge < 0
		}
	}
	if !hasAccess || !hasRefresh {
		t.Fatalf("auth cookies not set: access=%v refresh=%v (cookies: %s)", hasAccess, hasRefresh, cookies)
	}
	if !hasState {
		t.Fatal("state cookie not cleared")
	}
}

// guard: the code path rejects empty state/cookie combos
func TestOIDCCallbackHandlerMissingCookie(t *testing.T) {
	h := NewOIDCCallbackHandler(stubOIDCAdapter{}, NewTokenManager(DefaultTokenConfig("test-secret"), nil), "", false)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?code=abc&state=good", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "missing or invalid code/state") {
		t.Fatalf("body = %q, want code/state error", rec.Body.String())
	}
}
