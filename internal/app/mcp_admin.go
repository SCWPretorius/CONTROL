package app

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/SCWPretorius/CONTROL/internal/config"
)

const mcpAdminServersPath = "/admin/mcp/servers"
const defaultMCPAdminTimeout = 30 * time.Second

type mcpServerRegistrar interface {
	RegisterMCPServer(context.Context, string, config.MCPServerConfig) (bool, error)
}

type mcpRegistrationRequest struct {
	Name string `json:"name"`
	config.MCPServerConfig
}

type mcpRegistrationResponse struct {
	Name      string `json:"name"`
	Updated   bool   `json:"updated"`
	AppliesTo string `json:"applies_to"`
}

func startMCPAdminServer(ctx context.Context, cfg config.Config, logger *log.Logger, registrar mcpServerRegistrar) (func() error, error) {
	logger = defaultLogger(logger)
	if !cfg.Tools.MCP.Admin.Enabled() {
		return func() error { return nil }, nil
	}
	if registrar == nil {
		return nil, errors.New("mcp admin registrar is required")
	}

	listener, err := net.Listen("tcp", cfg.Tools.MCP.Admin.ListenAddress)
	if err != nil {
		return nil, fmt.Errorf("listen for MCP admin server on %s: %w", cfg.Tools.MCP.Admin.ListenAddress, err)
	}

	server := &http.Server{
		Addr:              cfg.Tools.MCP.Admin.ListenAddress,
		Handler:           newMCPAdminHandler(cfg.Tools.MCP.Admin, logger, registrar),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       fallbackDuration(cfg.Tools.Runtime.HTTPTimeout, defaultMCPAdminTimeout),
		WriteTimeout:      fallbackDuration(cfg.Tools.Runtime.HTTPTimeout, defaultMCPAdminTimeout),
		IdleTimeout:       fallbackDuration(cfg.Tools.Runtime.HTTPTimeout, defaultMCPAdminTimeout),
	}

	stopped := make(chan error, 1)
	go func() {
		serveErr := server.Serve(listener)
		if serveErr != nil && serveErr != http.ErrServerClosed {
			stopped <- fmt.Errorf("serve MCP admin HTTP listener: %w", serveErr)
			return
		}
		stopped <- nil
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	logger.Printf("mcp admin endpoint enabled addr=%s path=%s", cfg.Tools.MCP.Admin.ListenAddress, mcpAdminServersPath)

	return func() error {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		shutdownErr := server.Shutdown(shutdownCtx)
		serveErr := <-stopped
		return errors.Join(shutdownErr, serveErr)
	}, nil
}

func newMCPAdminHandler(admin config.MCPAdminConfig, logger *log.Logger, registrar mcpServerRegistrar) http.Handler {
	logger = defaultLogger(logger)

	mux := http.NewServeMux()
	mux.HandleFunc("POST "+mcpAdminServersPath, func(w http.ResponseWriter, r *http.Request) {
		if !authorizedBearer(r, admin.BearerToken) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "missing or invalid bearer token", http.StatusUnauthorized)
			return
		}

		var request mcpRegistrationRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, fmt.Sprintf("invalid MCP registration payload: %v", err), http.StatusBadRequest)
			return
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			http.Error(w, "invalid MCP registration payload: unexpected trailing JSON", http.StatusBadRequest)
			return
		}

		updated, err := registrar.RegisterMCPServer(r.Context(), request.Name, request.MCPServerConfig)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		logger.Printf(
			"registered runtime MCP server name=%s type=%s updated=%t remote=%s",
			strings.TrimSpace(request.Name),
			strings.TrimSpace(request.Type),
			updated,
			r.RemoteAddr,
		)

		statusCode := http.StatusCreated
		if updated {
			statusCode = http.StatusOK
		}
		writeJSON(w, statusCode, mcpRegistrationResponse{
			Name:      strings.TrimSpace(request.Name),
			Updated:   updated,
			AppliesTo: "future create/resume operations only; existing in-memory sessions keep their current MCP snapshot",
		})
	})

	return mux
}

func authorizedBearer(r *http.Request, token string) bool {
	got := strings.TrimSpace(r.Header.Get("Authorization"))
	expected := "Bearer " + strings.TrimSpace(token)
	if len(got) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func fallbackDuration(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}
