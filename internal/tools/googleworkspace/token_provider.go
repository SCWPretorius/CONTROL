package googleworkspace

import (
	"context"
	"errors"
	"strings"

	"github.com/SCWPretorius/CONTROL/internal/config"
)

// EnvAccessTokenProvider returns the configured bearer token exactly as loaded
// from the assistant environment.
type EnvAccessTokenProvider struct {
	accessToken string
}

// NewEnvAccessTokenProvider builds the v1 token provider from env-backed config.
func NewEnvAccessTokenProvider(cfg config.GoogleToolConfig) (*EnvAccessTokenProvider, error) {
	if !cfg.OAuth.Enabled() {
		return nil, errors.New("google workspace: oauth config is not enabled")
	}
	token := strings.TrimSpace(cfg.AccessToken)
	if token == "" {
		return nil, errors.New("google workspace: access token is required")
	}

	return &EnvAccessTokenProvider{accessToken: token}, nil
}

// AccessToken returns the current Google bearer token.
func (p *EnvAccessTokenProvider) AccessToken(context.Context) (string, error) {
	if p == nil {
		return "", errors.New("google workspace: access token provider is not configured")
	}
	return p.accessToken, nil
}
