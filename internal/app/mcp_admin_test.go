package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SCWPretorius/CONTROL/internal/config"
)

func TestMCPAdminHandlerRegistersServerWhenAuthorized(t *testing.T) {
	t.Parallel()

	registrar := &stubMCPRegistrar{}
	handler := newMCPAdminHandler(config.MCPAdminConfig{
		ListenAddress: "127.0.0.1:8788",
		BearerToken:   "secret-token",
	}, nil, registrar)

	body, err := json.Marshal(map[string]any{
		"name":    "filesystem",
		"type":    "stdio",
		"command": "npx",
		"tools":   []string{"*"},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, mcpAdminServersPath, bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer secret-token")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)
	if got, want := recorder.Code, http.StatusCreated; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if registrar.name != "filesystem" {
		t.Fatalf("registered name = %q, want %q", registrar.name, "filesystem")
	}
	if registrar.server.Command != "npx" {
		t.Fatalf("registered command = %q, want %q", registrar.server.Command, "npx")
	}
}

func TestMCPAdminHandlerRejectsUnauthorizedRequests(t *testing.T) {
	t.Parallel()

	handler := newMCPAdminHandler(config.MCPAdminConfig{
		ListenAddress: "127.0.0.1:8788",
		BearerToken:   "secret-token",
	}, nil, &stubMCPRegistrar{})

	request := httptest.NewRequest(http.MethodPost, mcpAdminServersPath, bytes.NewReader([]byte(`{"name":"filesystem"}`)))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)
	if got, want := recorder.Code, http.StatusUnauthorized; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestMCPAdminHandlerSurfacesValidationErrors(t *testing.T) {
	t.Parallel()

	handler := newMCPAdminHandler(config.MCPAdminConfig{
		ListenAddress: "127.0.0.1:8788",
		BearerToken:   "secret-token",
	}, nil, &stubMCPRegistrar{err: errors.New("invalid MCP config")})

	request := httptest.NewRequest(http.MethodPost, mcpAdminServersPath, bytes.NewReader([]byte(`{"name":"filesystem","type":"stdio","command":"npx","tools":["*"]}`)))
	request.Header.Set("Authorization", "Bearer secret-token")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)
	if got, want := recorder.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestMCPAdminHandlerRejectsTrailingJSON(t *testing.T) {
	t.Parallel()

	handler := newMCPAdminHandler(config.MCPAdminConfig{
		ListenAddress: "127.0.0.1:8788",
		BearerToken:   "secret-token",
	}, nil, &stubMCPRegistrar{})

	request := httptest.NewRequest(http.MethodPost, mcpAdminServersPath, bytes.NewReader([]byte(`{"name":"filesystem","type":"stdio","command":"npx","tools":["*"]}{"extra":true}`)))
	request.Header.Set("Authorization", "Bearer secret-token")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)
	if got, want := recorder.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

type stubMCPRegistrar struct {
	name   string
	server config.MCPServerConfig
	err    error
}

func (s *stubMCPRegistrar) RegisterMCPServer(_ context.Context, name string, server config.MCPServerConfig) (bool, error) {
	s.name = name
	s.server = server
	if s.err != nil {
		return false, s.err
	}
	return false, nil
}
