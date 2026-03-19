package googleworkspace

import (
	"net/url"
	"strings"
	"testing"

	"github.com/SCWPretorius/CONTROL/internal/config"
)

func TestAuthorizationURLUsesSharedOAuthConfig(t *testing.T) {
	t.Parallel()

	rawURL, err := AuthorizationURL(config.GoogleOAuthConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "http://127.0.0.1:8787/oauth/callback",
		Scopes: []string{
			"scope-a",
			"scope-b",
		},
	}, "state-123", AuthorizationURLOptions{
		AccessType:           "offline",
		Prompt:               "consent",
		IncludeGrantedScopes: true,
		LoginHint:            "assistant@example.com",
	})
	if err != nil {
		t.Fatalf("AuthorizationURL() error = %v", err)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	if got, want := parsed.Scheme, "https"; got != want {
		t.Fatalf("scheme = %q, want %q", got, want)
	}

	query := parsed.Query()
	if got, want := query.Get("client_id"), "client-id"; got != want {
		t.Fatalf("client_id = %q, want %q", got, want)
	}
	if got, want := query.Get("redirect_uri"), "http://127.0.0.1:8787/oauth/callback"; got != want {
		t.Fatalf("redirect_uri = %q, want %q", got, want)
	}
	if got, want := query.Get("state"), "state-123"; got != want {
		t.Fatalf("state = %q, want %q", got, want)
	}
	if got, want := query.Get("scope"), "scope-a scope-b"; got != want {
		t.Fatalf("scope = %q, want %q", got, want)
	}
	if got, want := query.Get("access_type"), "offline"; got != want {
		t.Fatalf("access_type = %q, want %q", got, want)
	}
	if got, want := query.Get("prompt"), "consent"; got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
	if got, want := query.Get("include_granted_scopes"), "true"; got != want {
		t.Fatalf("include_granted_scopes = %q, want %q", got, want)
	}
}

func TestValidateOAuthConfigRejectsDisabledConfig(t *testing.T) {
	t.Parallel()

	err := ValidateOAuthConfig(config.GoogleOAuthConfig{})
	if err == nil || !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("ValidateOAuthConfig() error = %v, want disabled-config error", err)
	}
}
