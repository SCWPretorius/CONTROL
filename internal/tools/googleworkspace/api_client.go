package googleworkspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/SCWPretorius/CONTROL/internal/config"
)

const (
	// GmailBaseURL is the REST API root for Gmail operations.
	GmailBaseURL = "https://gmail.googleapis.com/gmail/v1"
	// CalendarBaseURL is the REST API root for Google Calendar operations.
	CalendarBaseURL  = "https://www.googleapis.com/calendar/v3"
	defaultUserAgent = "CONTROL-google-workspace"
)

// HTTPDoer is the minimal HTTP client contract used by Google Workspace clients.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// AccessTokenProvider provides OAuth bearer tokens without coupling this package
// to runtime token persistence or refresh wiring.
type AccessTokenProvider interface {
	AccessToken(context.Context) (string, error)
}

// APIClient is a shared authenticated JSON client for Google Workspace APIs.
type APIClient struct {
	baseURL       string
	httpClient    HTTPDoer
	tokenProvider AccessTokenProvider
	userAgent     string
}

// APIError describes a non-success API response.
type APIError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *APIError) Error() string {
	if strings.TrimSpace(e.Body) == "" {
		return fmt.Sprintf("google workspace api: %s", e.Status)
	}

	return fmt.Sprintf("google workspace api: %s: %s", e.Status, e.Body)
}

// NewAPIClient validates the shared OAuth configuration and builds a reusable
// authenticated JSON client.
func NewAPIClient(baseURL string, oauth config.GoogleOAuthConfig, tokenProvider AccessTokenProvider, httpClient HTTPDoer) (*APIClient, error) {
	if err := ValidateOAuthConfig(oauth); err != nil {
		return nil, err
	}
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("google workspace: base URL is required")
	}
	if tokenProvider == nil {
		return nil, errors.New("google workspace: access token provider is required")
	}
	if httpClient == nil {
		return nil, errors.New("google workspace: HTTP client is required")
	}

	return &APIClient{
		baseURL:       strings.TrimRight(baseURL, "/"),
		httpClient:    httpClient,
		tokenProvider: tokenProvider,
		userAgent:     defaultUserAgent,
	}, nil
}

// DoJSON executes an authenticated JSON request and decodes the JSON response
// into dst when provided.
func (c *APIClient) DoJSON(ctx context.Context, method, path string, query url.Values, body any, dst any) error {
	requestURL := c.baseURL + normalizePath(path)
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("google workspace: encode request body: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, requestURL, payload)
	if err != nil {
		return fmt.Errorf("google workspace: create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	token, err := c.tokenProvider.AccessToken(ctx)
	if err != nil {
		return fmt.Errorf("google workspace: get access token: %w", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("google workspace: access token is empty")
	}
	request.Header.Set("Authorization", "Bearer "+token)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("google workspace: execute request: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("google workspace: read response body: %w", err)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &APIError{
			StatusCode: response.StatusCode,
			Status:     response.Status,
			Body:       strings.TrimSpace(string(responseBody)),
		}
	}

	if dst == nil || len(bytes.TrimSpace(responseBody)) == 0 {
		return nil
	}

	if err := json.Unmarshal(responseBody, dst); err != nil {
		return fmt.Errorf("google workspace: decode response body: %w", err)
	}

	return nil
}

func normalizePath(path string) string {
	if strings.HasPrefix(path, "/") {
		return path
	}

	return "/" + path
}
