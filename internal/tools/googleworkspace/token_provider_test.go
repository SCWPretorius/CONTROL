package googleworkspace

import (
	"context"
	"strings"
	"testing"

	"github.com/SCWPretorius/CONTROL/internal/config"
)

func TestNewEnvAccessTokenProviderReturnsConfiguredToken(t *testing.T) {
	t.Parallel()

	provider, err := NewEnvAccessTokenProvider(config.GoogleToolConfig{
		OAuth: config.GoogleOAuthConfig{
			ClientID:     "client-id",
			ClientSecret: "client-secret",
			RedirectURL:  "http://127.0.0.1:8787/oauth/callback",
			Scopes:       []string{"scope-a"},
		},
		AccessToken: "access-token",
	})
	if err != nil {
		t.Fatalf("NewEnvAccessTokenProvider() error = %v", err)
	}

	token, err := provider.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("AccessToken() error = %v", err)
	}
	if token != "access-token" {
		t.Fatalf("AccessToken() = %q, want %q", token, "access-token")
	}
}

func TestNewEnvAccessTokenProviderRequiresToken(t *testing.T) {
	t.Parallel()

	_, err := NewEnvAccessTokenProvider(config.GoogleToolConfig{
		OAuth: config.GoogleOAuthConfig{
			ClientID:     "client-id",
			ClientSecret: "client-secret",
			RedirectURL:  "http://127.0.0.1:8787/oauth/callback",
			Scopes:       []string{"scope-a"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "access token is required") {
		t.Fatalf("NewEnvAccessTokenProvider() error = %v, want missing-token error", err)
	}
}
