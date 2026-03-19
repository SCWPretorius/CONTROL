package googleworkspace

import (
	"errors"
	"net/url"
	"strings"

	"github.com/SCWPretorius/CONTROL/internal/config"
)

const authorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"

// AuthorizationURLOptions defines optional Google OAuth parameters.
type AuthorizationURLOptions struct {
	AccessType           string
	Prompt               string
	LoginHint            string
	IncludeGrantedScopes bool
}

// ValidateOAuthConfig ensures the shared Google OAuth environment config is
// present before tool-specific clients are used.
func ValidateOAuthConfig(cfg config.GoogleOAuthConfig) error {
	if !cfg.Enabled() {
		return errors.New("google workspace: oauth config is not enabled")
	}
	if strings.TrimSpace(cfg.ClientID) == "" {
		return errors.New("google workspace: oauth client ID is required")
	}
	if strings.TrimSpace(cfg.ClientSecret) == "" {
		return errors.New("google workspace: oauth client secret is required")
	}
	if strings.TrimSpace(cfg.RedirectURL) == "" {
		return errors.New("google workspace: oauth redirect URL is required")
	}

	scopes := trimScopes(cfg.Scopes)
	if len(scopes) == 0 {
		return errors.New("google workspace: at least one oauth scope is required")
	}

	return nil
}

// AuthorizationURL builds a Google consent-screen URL from the shared env
// configuration. Runtime token exchange can be wired separately later.
func AuthorizationURL(cfg config.GoogleOAuthConfig, state string, opts AuthorizationURLOptions) (string, error) {
	if err := ValidateOAuthConfig(cfg); err != nil {
		return "", err
	}
	if strings.TrimSpace(state) == "" {
		return "", errors.New("google workspace: oauth state is required")
	}

	values := url.Values{}
	values.Set("client_id", cfg.ClientID)
	values.Set("redirect_uri", cfg.RedirectURL)
	values.Set("response_type", "code")
	values.Set("scope", strings.Join(trimScopes(cfg.Scopes), " "))
	values.Set("state", state)

	if accessType := strings.TrimSpace(opts.AccessType); accessType != "" {
		values.Set("access_type", accessType)
	}
	if prompt := strings.TrimSpace(opts.Prompt); prompt != "" {
		values.Set("prompt", prompt)
	}
	if loginHint := strings.TrimSpace(opts.LoginHint); loginHint != "" {
		values.Set("login_hint", loginHint)
	}
	if opts.IncludeGrantedScopes {
		values.Set("include_granted_scopes", "true")
	}

	return authorizeURL + "?" + values.Encode(), nil
}

func trimScopes(scopes []string) []string {
	trimmed := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		trimmed = append(trimmed, scope)
	}

	return trimmed
}
