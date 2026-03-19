package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/SCWPretorius/CONTROL/internal/config"
	"github.com/SCWPretorius/CONTROL/internal/tools/googleworkspace"
)

// InitializeGoogleOAuth handles interactive OAuth setup on first run.
// If Google OAuth is configured but no access token is set, it:
//   - Displays the authorization URL
//   - Starts a local HTTP server to handle the callback
//   - Prints the resulting access token to the terminal
//
// Returns the access token if successful, empty string if skipped.
func InitializeGoogleOAuth(ctx context.Context, cfg config.Config, logger *log.Logger) (string, error) {
	logger = defaultLogger(logger)

	// Skip if OAuth config is incomplete
	if !cfg.Tools.Google.Enabled() {
		return "", nil
	}

	// Skip if access token is already configured
	if cfg.Tools.Google.AccessTokenConfigured() {
		return cfg.Tools.Google.AccessToken, nil
	}

	logger.Printf("google workspace: initializing oauth flow (interactive)")

	// Generate authorization URL
	authURL, err := googleworkspace.AuthorizationURL(
		cfg.Tools.Google.OAuth,
		"state-"+fmt.Sprintf("%d", time.Now().Unix()),
		googleworkspace.AuthorizationURLOptions{
			AccessType: "offline",
			Prompt:     "consent",
		},
	)
	if err != nil {
		return "", fmt.Errorf("generate oauth url: %w", err)
	}

	// Extract port from redirect URL
	redirectURL := cfg.Tools.Google.OAuth.RedirectURL
	redirectURLParsed, err := url.Parse(redirectURL)
	if err != nil {
		return "", fmt.Errorf("parse redirect url: %w", err)
	}

	port := redirectURLParsed.Port()
	if port == "" {
		if redirectURLParsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	callbackPath := strings.TrimSpace(redirectURLParsed.Path)
	if callbackPath == "" {
		callbackPath = "/"
	}

	// Start local HTTP server to handle callback
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing authorization code", http.StatusBadRequest)
			errChan <- fmt.Errorf("oauth callback: missing code parameter")
			return
		}

		// Send code to channel
		select {
		case codeChan <- code:
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, "Authorization successful! You can close this window and return to the terminal.")
		default:
			http.Error(w, "code already received", http.StatusBadRequest)
		}
	})

	server := &http.Server{
		Addr:    "127.0.0.1:" + port,
		Handler: mux,
		// Set timeouts to prevent hanging
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       5 * time.Second,
	}

	// Start server in background
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("oauth callback server: %w", err)
		}
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Check if port is actually listening on localhost
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 1*time.Second)
	if err != nil {
		server.Close()
		return "", fmt.Errorf("oauth callback server failed to start on %s: %w", server.Addr, err)
	}
	conn.Close()

	defer server.Close()

	// Display authorization URL to user
	logger.Printf("")
	logger.Printf("╔════════════════════════════════════════════════════════════════════════════════╗")
	logger.Printf("║                                                                                ║")
	logger.Printf("║  Open this URL in your browser to authorize Google Workspace access:         ║")
	logger.Printf("║                                                                                ║")
	logger.Printf("║  %s", authURL)
	logger.Printf("║                                                                                ║")
	logger.Printf("║  Waiting for authorization...                                                 ║")
	logger.Printf("║                                                                                ║")
	logger.Printf("╚════════════════════════════════════════════════════════════════════════════════╝")
	logger.Printf("")

	// Wait for code or timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	var code string
	select {
	case code = <-codeChan:
		logger.Printf("google workspace: authorization code received")
	case err := <-errChan:
		return "", fmt.Errorf("oauth callback error: %w", err)
	case <-timeoutCtx.Done():
		return "", fmt.Errorf("oauth flow timeout: authorization took longer than 5 minutes")
	}

	// For desktop app, we need to exchange the code for tokens manually
	// This requires calling the Google OAuth token endpoint
	tokenURL := "https://oauth2.googleapis.com/token"

	// Build token request
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURL)
	data.Set("client_id", cfg.Tools.Google.OAuth.ClientID)
	data.Set("client_secret", cfg.Tools.Google.OAuth.ClientSecret)

	// Exchange code for tokens
	resp, err := (&http.Client{Timeout: 15 * time.Second}).PostForm(tokenURL, data)
	if err != nil {
		return "", fmt.Errorf("exchange authorization code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token exchange failed (status %d): %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	accessToken := strings.TrimSpace(tokenResp.AccessToken)
	if accessToken == "" {
		return "", fmt.Errorf("no access token in response: %s", string(bodyBytes))
	}

	// Display token to user
	logger.Printf("")
	logger.Printf("╔════════════════════════════════════════════════════════════════════════════════╗")
	logger.Printf("║                                                                                ║")
	logger.Printf("║  ✓ Authorization successful!                                                   ║")
	logger.Printf("║                                                                                ║")
	logger.Printf("║  Add this to your setup.ps1:                                                   ║")
	logger.Printf("║                                                                                ║")
	logger.Printf("║  $env:GOOGLE_OAUTH_ACCESS_TOKEN = \"%s\"", accessToken)
	logger.Printf("║                                                                                ║")
	logger.Printf("║  Then re-run: .\\setup.ps1                                                      ║")
	logger.Printf("║                                                                                ║")
	logger.Printf("╚════════════════════════════════════════════════════════════════════════════════╝")
	logger.Printf("")

	return accessToken, nil
}
