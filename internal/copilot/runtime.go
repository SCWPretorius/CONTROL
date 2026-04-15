package copilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/github/copilot-sdk/go"

	"github.com/SCWPretorius/CONTROL/internal/config"
)

const (
	defaultClientName = "control-assistant"
	defaultLogLevel   = "error"
)

var resumeNotFoundMarkers = []string{
	"not found",
	"no such session",
	"unknown session",
	"does not exist",
	"missing session",
}

// Endpoint describes how the assistant will reach the Copilot runtime.
type Endpoint struct {
	CLIPath    string
	CLIURL     string
	WorkingDir string
	ConfigDir  string
}

func (e Endpoint) UsesRemoteServer() bool {
	return strings.TrimSpace(e.CLIURL) != ""
}

// ClientOptions maps the local endpoint config into the official Copilot SDK.
func (e Endpoint) ClientOptions(logLevel string) *sdk.ClientOptions {
	options := &sdk.ClientOptions{
		LogLevel: defaultIfBlank(logLevel, defaultLogLevel),
	}

	if e.UsesRemoteServer() {
		options.CLIUrl = strings.TrimSpace(e.CLIURL)
		return options
	}

	options.CLIPath = defaultIfBlank(strings.TrimSpace(e.CLIPath), "copilot")
	options.Cwd = strings.TrimSpace(e.WorkingDir)
	return options
}

// SessionOptions captures the session knobs the foundation already models in config.
type SessionOptions struct {
	Model           string
	ReasoningEffort string
	Namespace       string
	ResumeSessions  bool
	WorkingDir      string
	ConfigDir       string
	ClientName      string
	Tools           []sdk.Tool
	Provider        *sdk.ProviderConfig
	MCPServers      map[string]sdk.MCPServerConfig
}

func (o SessionOptions) CreateConfig(sessionID string, hooks RuntimeHooks, externalKey string) *sdk.SessionConfig {
	return &sdk.SessionConfig{
		SessionID:           sessionID,
		ClientName:          defaultIfBlank(strings.TrimSpace(o.ClientName), defaultClientName),
		Model:               strings.TrimSpace(o.Model),
		ReasoningEffort:     strings.TrimSpace(o.ReasoningEffort),
		ConfigDir:           strings.TrimSpace(o.ConfigDir),
		Tools:               cloneTools(o.Tools),
		Provider:            cloneProviderConfig(o.Provider),
		MCPServers:          cloneMCPServers(o.MCPServers),
		OnPermissionRequest: hooks.wrapPermissionHandler(externalKey),
		OnUserInputRequest:  hooks.OnUserInputRequest,
		Hooks:               hooks.wrapSessionHooks(externalKey),
		WorkingDirectory:    strings.TrimSpace(o.WorkingDir),
		InfiniteSessions:    &sdk.InfiniteSessionConfig{Enabled: sdk.Bool(o.ResumeSessions)},
	}
}

func (o SessionOptions) ResumeConfig(hooks RuntimeHooks, externalKey string) *sdk.ResumeSessionConfig {
	return &sdk.ResumeSessionConfig{
		ClientName:          defaultIfBlank(strings.TrimSpace(o.ClientName), defaultClientName),
		Model:               strings.TrimSpace(o.Model),
		ReasoningEffort:     strings.TrimSpace(o.ReasoningEffort),
		ConfigDir:           strings.TrimSpace(o.ConfigDir),
		Tools:               cloneTools(o.Tools),
		Provider:            cloneProviderConfig(o.Provider),
		MCPServers:          cloneMCPServers(o.MCPServers),
		OnPermissionRequest: hooks.wrapPermissionHandler(externalKey),
		OnUserInputRequest:  hooks.OnUserInputRequest,
		Hooks:               hooks.wrapSessionHooks(externalKey),
		WorkingDirectory:    strings.TrimSpace(o.WorkingDir),
		InfiniteSessions:    &sdk.InfiniteSessionConfig{Enabled: sdk.Bool(o.ResumeSessions)},
	}
}

// RuntimeConfig is the app-facing configuration needed by the Copilot runtime boundary.
type RuntimeConfig struct {
	Endpoint Endpoint
	Session  SessionOptions
	LogLevel string
}

// ConfigFromFoundation adapts the existing app config into the internal Copilot runtime boundary.
func ConfigFromFoundation(cfg config.Config) RuntimeConfig {
	return RuntimeConfig{
		Endpoint: Endpoint{
			CLIPath:    cfg.Copilot.CLIPath,
			CLIURL:     cfg.Copilot.CLIURL,
			WorkingDir: cfg.Session.WorkingDir,
			ConfigDir:  cfg.Session.ConfigDir,
		},
		Session: SessionOptions{
			Model:           cfg.Session.Model,
			ReasoningEffort: cfg.Session.ReasoningEffort,
			Namespace:       cfg.Session.Namespace,
			ResumeSessions:  cfg.Session.ResumeSessions,
			WorkingDir:      cfg.Session.WorkingDir,
			ConfigDir:       cfg.Session.ConfigDir,
			ClientName:      defaultClientName,
			Provider:        configProviderToSDK(cfg.Session.Provider),
			MCPServers:      configMCPServersToSDK(cfg.Tools.MCP.Servers),
		},
		LogLevel: defaultLogLevel,
	}
}

// ExternalSessionKey identifies an app-owned conversation without leaking the raw
// external identifiers into the Copilot session ID.
type ExternalSessionKey struct {
	Identifiers map[string]string
}

func (k ExternalSessionKey) Canonical() (string, error) {
	pairs, err := k.normalizedIdentifiers()
	if err != nil {
		return "", err
	}

	parts := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		parts = append(parts, url.QueryEscape(pair.Key)+"="+url.QueryEscape(pair.Value))
	}

	return strings.Join(parts, "|"), nil
}

func (k ExternalSessionKey) legacyCanonical() (string, error) {
	pairs, err := k.normalizedIdentifiers()
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(pairs)
	if err != nil {
		return "", fmt.Errorf("marshal legacy external key: %w", err)
	}

	return string(payload), nil
}

func (k ExternalSessionKey) normalizedIdentifiers() ([]identifierPair, error) {
	if len(k.Identifiers) == 0 {
		return nil, errors.New("at least one external identifier is required")
	}

	normalized := make(map[string]string, len(k.Identifiers))
	keys := make([]string, 0, len(k.Identifiers))
	for key, value := range k.Identifiers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return nil, errors.New("external identifier keys must not be empty")
		}
		if value == "" {
			return nil, fmt.Errorf("external identifier %q must not be empty", key)
		}
		if _, exists := normalized[key]; exists {
			return nil, fmt.Errorf("external identifier %q is duplicated after normalization", key)
		}
		normalized[key] = value
		keys = append(keys, key)
	}

	sort.Strings(keys)
	parts := make([]identifierPair, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, identifierPair{Key: key, Value: normalized[key]})
	}

	return parts, nil
}

// SessionID derives a stable Copilot session ID from the namespace and external identifiers.
func (k ExternalSessionKey) SessionID(namespace string) (string, string, error) {
	canonical, err := k.Canonical()
	if err != nil {
		return "", "", err
	}

	legacyCanonical, err := k.legacyCanonical()
	if err != nil {
		return "", "", err
	}

	safeNamespace := sanitizeNamespace(namespace)
	// Keep the session hash input backward-compatible with the original JSON
	// encoding so persisted chat bindings continue to resume existing sessions.
	sum := sha256.Sum256([]byte(safeNamespace + "\n" + legacyCanonical))
	return safeNamespace + "-" + hex.EncodeToString(sum[:12]), canonical, nil
}

type identifierPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ParseExternalSessionKey decodes the canonical external-key form emitted by this
// package. It also accepts the legacy JSON encoding so older audit data remains
// readable.
func ParseExternalSessionKey(externalKey string) (map[string]string, error) {
	externalKey = strings.TrimSpace(externalKey)
	if externalKey == "" {
		return nil, errors.New("external key is required")
	}

	if strings.HasPrefix(externalKey, "[") {
		return parseLegacyExternalSessionKey(externalKey)
	}

	identifiers := make(map[string]string)
	parts := strings.Split(externalKey, "|")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		key, value, found := strings.Cut(part, "=")
		if !found {
			return nil, fmt.Errorf("invalid external key segment %q", part)
		}

		decodedKey, err := url.QueryUnescape(strings.TrimSpace(key))
		if err != nil {
			return nil, fmt.Errorf("decode external key name %q: %w", key, err)
		}
		decodedValue, err := url.QueryUnescape(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("decode external key value for %q: %w", decodedKey, err)
		}
		if decodedKey == "" {
			return nil, errors.New("external key name must not be empty")
		}
		if _, exists := identifiers[decodedKey]; exists {
			return nil, fmt.Errorf("external key %q is duplicated", decodedKey)
		}
		identifiers[decodedKey] = decodedValue
	}
	if len(identifiers) == 0 {
		return nil, errors.New("external key does not contain any identifiers")
	}

	return identifiers, nil
}

func parseLegacyExternalSessionKey(externalKey string) (map[string]string, error) {
	var identifiers []identifierPair
	if err := json.Unmarshal([]byte(externalKey), &identifiers); err != nil {
		return nil, fmt.Errorf("parse legacy external key: %w", err)
	}

	result := make(map[string]string, len(identifiers))
	for _, identifier := range identifiers {
		if identifier.Key == "" {
			return nil, errors.New("external key name must not be empty")
		}
		if _, exists := result[identifier.Key]; exists {
			return nil, fmt.Errorf("external key %q is duplicated", identifier.Key)
		}
		result[identifier.Key] = identifier.Value
	}
	if len(result) == 0 {
		return nil, errors.New("external key does not contain any identifiers")
	}

	return result, nil
}

// PermissionEvent contains the app context for a Copilot permission request.
type PermissionEvent struct {
	ExternalKey string
	SessionID   string
	Request     sdk.PermissionRequest
}

// PermissionHandler decides whether a Copilot permission request should proceed.
type PermissionHandler func(context.Context, PermissionEvent) (sdk.PermissionRequestResult, error)

// RuntimeEvent is a lightweight event envelope emitted by the Copilot runtime boundary.
type RuntimeEvent struct {
	Kind        string
	ExternalKey string
	SessionID   string
	Event       *sdk.SessionEvent
	Err         error
	Message     string
	Metadata    map[string]string
	OccurredAt  time.Time
}

// EventLogger receives best-effort runtime/session events.
type EventLogger func(context.Context, RuntimeEvent)

// RuntimeHooks wires app-owned permission and observability hooks into the SDK.
type RuntimeHooks struct {
	OnPermissionRequest PermissionHandler
	OnUserInputRequest  sdk.UserInputHandler
	SessionHooks        *sdk.SessionHooks
	OnEvent             EventLogger
}

func (h RuntimeHooks) emit(ctx context.Context, event RuntimeEvent) {
	if h.OnEvent == nil {
		return
	}

	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}

	defer func() {
		_ = recover()
	}()

	h.OnEvent(ctx, event)
}

func (h RuntimeHooks) wrapPermissionHandler(externalKey string) sdk.PermissionHandlerFunc {
	return func(request sdk.PermissionRequest, invocation sdk.PermissionInvocation) (sdk.PermissionRequestResult, error) {
		ctx := context.Background()
		h.emit(ctx, RuntimeEvent{
			Kind:        "permission.requested",
			ExternalKey: externalKey,
			SessionID:   invocation.SessionID,
			Message:     string(request.Kind),
		})

		if h.OnPermissionRequest == nil {
			result := sdk.PermissionRequestResult{Kind: sdk.PermissionRequestResultKindDeniedCouldNotRequestFromUser}
			h.emit(ctx, RuntimeEvent{
				Kind:        "permission.decided",
				ExternalKey: externalKey,
				SessionID:   invocation.SessionID,
				Message:     string(result.Kind),
			})
			return result, nil
		}

		result, err := safePermissionDecision(ctx, h.OnPermissionRequest, PermissionEvent{
			ExternalKey: externalKey,
			SessionID:   invocation.SessionID,
			Request:     request,
		})
		if result.Kind == "" {
			result.Kind = sdk.PermissionRequestResultKindDeniedCouldNotRequestFromUser
		}

		h.emit(ctx, RuntimeEvent{
			Kind:        "permission.decided",
			ExternalKey: externalKey,
			SessionID:   invocation.SessionID,
			Err:         err,
			Message:     string(result.Kind),
		})
		return result, err
	}
}

func (h RuntimeHooks) wrapSessionHooks(externalKey string) *sdk.SessionHooks {
	if h.SessionHooks == nil && h.OnEvent == nil {
		return nil
	}

	var source sdk.SessionHooks
	if h.SessionHooks != nil {
		source = *h.SessionHooks
	}

	return &sdk.SessionHooks{
		OnPreToolUse: func(input sdk.PreToolUseHookInput, invocation sdk.HookInvocation) (*sdk.PreToolUseHookOutput, error) {
			h.emit(context.Background(), RuntimeEvent{
				Kind:        "hook.pre_tool_use",
				ExternalKey: externalKey,
				SessionID:   invocation.SessionID,
				Metadata:    map[string]string{"tool_name": input.ToolName},
			})
			if source.OnPreToolUse == nil {
				return nil, nil
			}
			return source.OnPreToolUse(input, invocation)
		},
		OnPostToolUse: func(input sdk.PostToolUseHookInput, invocation sdk.HookInvocation) (*sdk.PostToolUseHookOutput, error) {
			h.emit(context.Background(), RuntimeEvent{
				Kind:        "hook.post_tool_use",
				ExternalKey: externalKey,
				SessionID:   invocation.SessionID,
				Metadata:    map[string]string{"tool_name": input.ToolName},
			})
			if source.OnPostToolUse == nil {
				return nil, nil
			}
			return source.OnPostToolUse(input, invocation)
		},
		OnUserPromptSubmitted: func(input sdk.UserPromptSubmittedHookInput, invocation sdk.HookInvocation) (*sdk.UserPromptSubmittedHookOutput, error) {
			h.emit(context.Background(), RuntimeEvent{
				Kind:        "hook.user_prompt_submitted",
				ExternalKey: externalKey,
				SessionID:   invocation.SessionID,
			})
			if source.OnUserPromptSubmitted == nil {
				return nil, nil
			}
			return source.OnUserPromptSubmitted(input, invocation)
		},
		OnSessionStart: func(input sdk.SessionStartHookInput, invocation sdk.HookInvocation) (*sdk.SessionStartHookOutput, error) {
			h.emit(context.Background(), RuntimeEvent{
				Kind:        "hook.session_start",
				ExternalKey: externalKey,
				SessionID:   invocation.SessionID,
				Metadata:    map[string]string{"source": input.Source},
			})
			if source.OnSessionStart == nil {
				return nil, nil
			}
			return source.OnSessionStart(input, invocation)
		},
		OnSessionEnd: func(input sdk.SessionEndHookInput, invocation sdk.HookInvocation) (*sdk.SessionEndHookOutput, error) {
			h.emit(context.Background(), RuntimeEvent{
				Kind:        "hook.session_end",
				ExternalKey: externalKey,
				SessionID:   invocation.SessionID,
				Metadata:    map[string]string{"reason": input.Reason},
			})
			if source.OnSessionEnd == nil {
				return nil, nil
			}
			return source.OnSessionEnd(input, invocation)
		},
		OnErrorOccurred: func(input sdk.ErrorOccurredHookInput, invocation sdk.HookInvocation) (*sdk.ErrorOccurredHookOutput, error) {
			h.emit(context.Background(), RuntimeEvent{
				Kind:        "hook.error_occurred",
				ExternalKey: externalKey,
				SessionID:   invocation.SessionID,
				Message:     input.ErrorContext,
				Metadata:    map[string]string{"recoverable": fmt.Sprintf("%t", input.Recoverable)},
			})
			if source.OnErrorOccurred == nil {
				return nil, nil
			}
			return source.OnErrorOccurred(input, invocation)
		},
	}
}

// SessionHandle is the app-owned wrapper around an SDK session.
type SessionHandle struct {
	externalKey string
	sessionID   string
	session     *sdk.Session
	unsubscribe func()
	onClose     func()
	closeOnce   sync.Once
}

func (s *SessionHandle) ID() string {
	if s == nil {
		return ""
	}
	if s.sessionID != "" {
		return s.sessionID
	}
	if s.session == nil {
		return ""
	}
	return s.session.SessionID
}

func (s *SessionHandle) ExternalKey() string {
	if s == nil {
		return ""
	}
	return s.externalKey
}

func (s *SessionHandle) SDK() *sdk.Session {
	if s == nil {
		return nil
	}
	return s.session
}

func (s *SessionHandle) SendAndWait(ctx context.Context, options sdk.MessageOptions) (*sdk.SessionEvent, error) {
	if s == nil || s.session == nil {
		return nil, errors.New("copilot session is not available")
	}
	return s.session.SendAndWait(ctx, options)
}

func (s *SessionHandle) Close() error {
	if s == nil || s.session == nil {
		return nil
	}

	var err error
	s.closeOnce.Do(func() {
		if s.unsubscribe != nil {
			s.unsubscribe()
			s.unsubscribe = nil
		}
		err = s.session.Disconnect()
		if s.onClose != nil {
			s.onClose()
		}
	})
	return err
}

// Runtime is the Copilot SDK integration boundary used by the rest of the app.
type Runtime interface {
	Start(context.Context) error
	Close() error
	EnsureSession(context.Context, ExternalSessionKey) (*SessionHandle, error)
}

type clientAPI interface {
	Start(context.Context) error
	Stop() error
	CreateSession(context.Context, *sdk.SessionConfig) (*sdk.Session, error)
	ResumeSession(context.Context, string, *sdk.ResumeSessionConfig) (*sdk.Session, error)
}

// ClientRuntime is the concrete Copilot SDK-backed runtime implementation.
type ClientRuntime struct {
	cfg        RuntimeConfig
	hooks      RuntimeHooks
	client     clientAPI
	mu         sync.Mutex
	mcpMu      sync.RWMutex
	started    bool
	starting   chan struct{}
	startErr   error
	sessions   map[string]*SessionHandle
	pending    map[string]chan struct{}
	mcpServers map[string]sdk.MCPServerConfig
}

// NewRuntime constructs the Copilot runtime boundary around the official Go SDK.
func NewRuntime(cfg RuntimeConfig, hooks RuntimeHooks) *ClientRuntime {
	return &ClientRuntime{
		cfg:        cfg,
		hooks:      hooks,
		client:     sdk.NewClient(cfg.Endpoint.ClientOptions(cfg.LogLevel)),
		sessions:   make(map[string]*SessionHandle),
		pending:    make(map[string]chan struct{}),
		mcpServers: cloneMCPServers(cfg.Session.MCPServers),
	}
}

func (r *ClientRuntime) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return nil
	}
	if starting := r.starting; starting != nil {
		r.mu.Unlock()
		select {
		case <-starting:
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.started {
				return nil
			}
			if r.startErr != nil {
				return r.startErr
			}
			return errors.New("copilot runtime start did not complete")
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	starting := make(chan struct{})
	r.starting = starting
	r.startErr = nil
	r.mu.Unlock()

	err := r.client.Start(ctx)
	if err != nil {
		r.hooks.emit(context.Background(), RuntimeEvent{
			Kind:    "runtime.start_failed",
			Err:     err,
			Message: "failed to start copilot runtime",
		})
	}

	r.mu.Lock()
	r.started = err == nil
	r.startErr = nil
	if err != nil {
		r.startErr = fmt.Errorf("start copilot runtime: %w", err)
	}
	close(starting)
	r.starting = nil
	r.mu.Unlock()

	if err != nil {
		return fmt.Errorf("start copilot runtime: %w", err)
	}

	r.hooks.emit(context.Background(), RuntimeEvent{
		Kind:    "runtime.started",
		Message: "copilot runtime ready",
	})
	return nil
}

func (r *ClientRuntime) Close() error {
	for {
		r.mu.Lock()
		starting := r.starting
		r.mu.Unlock()
		if starting == nil {
			break
		}
		<-starting
	}

	r.mu.Lock()
	sessions := make([]*SessionHandle, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	r.sessions = make(map[string]*SessionHandle)
	wasStarted := r.started
	r.started = false
	r.startErr = nil
	r.mu.Unlock()

	var errs []error
	for _, session := range sessions {
		if err := session.Close(); err != nil {
			errs = append(errs, fmt.Errorf("disconnect session %q: %w", session.ID(), err))
		}
	}

	if wasStarted {
		if err := r.client.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("stop copilot runtime: %w", err))
		}
	}

	result := errors.Join(errs...)
	r.hooks.emit(context.Background(), RuntimeEvent{
		Kind:    "runtime.stopped",
		Err:     result,
		Message: "copilot runtime stopped",
	})
	return result
}

func (r *ClientRuntime) EnsureSession(ctx context.Context, key ExternalSessionKey) (*SessionHandle, error) {
	if err := r.Start(ctx); err != nil {
		return nil, err
	}

	sessionID, externalKey, err := key.SessionID(r.cfg.Session.Namespace)
	if err != nil {
		return nil, err
	}

	for {
		r.mu.Lock()
		if session, ok := r.sessions[sessionID]; ok {
			r.mu.Unlock()
			return session, nil
		}
		if pending := r.pending[sessionID]; pending != nil {
			r.mu.Unlock()
			select {
			case <-pending:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		pending := make(chan struct{})
		r.pending[sessionID] = pending
		r.mu.Unlock()

		handle, eventKind, loadErr := r.loadSession(ctx, sessionID, externalKey)

		r.mu.Lock()
		delete(r.pending, sessionID)
		close(pending)
		if loadErr != nil {
			r.mu.Unlock()
			return nil, loadErr
		}
		if existing, ok := r.sessions[sessionID]; ok {
			r.mu.Unlock()
			_ = handle.Close()
			return existing, nil
		}
		r.sessions[sessionID] = handle
		r.mu.Unlock()

		r.hooks.emit(context.Background(), RuntimeEvent{
			Kind:        eventKind,
			ExternalKey: externalKey,
			SessionID:   sessionID,
		})
		return handle, nil
	}
}

// RegisterMCPServer validates and registers or replaces one MCP server definition.
// New sessions created or resumed after registration receive the updated snapshot.
func (r *ClientRuntime) RegisterMCPServer(ctx context.Context, name string, server config.MCPServerConfig) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, errors.New("mcp server name must not be empty")
	}

	normalized := server
	if err := config.ValidateAndNormalizeMCPServer(r.cfg.Session.WorkingDir, name, &normalized); err != nil {
		return false, err
	}

	converted := configMCPServersToSDK(map[string]config.MCPServerConfig{name: normalized})
	entry, ok := converted[name]
	if !ok {
		return false, fmt.Errorf("convert MCP server %q to SDK config", name)
	}

	r.mcpMu.Lock()
	defer r.mcpMu.Unlock()
	if r.mcpServers == nil {
		r.mcpServers = make(map[string]sdk.MCPServerConfig)
	}
	_, updated := r.mcpServers[name]
	r.mcpServers[name] = entry
	r.hooks.emit(ctx, RuntimeEvent{
		Kind:    "mcp.server_registered",
		Message: name,
		Metadata: map[string]string{
			"name":    name,
			"type":    normalized.Type,
			"updated": strconv.FormatBool(updated),
		},
	})
	return updated, nil
}

func (r *ClientRuntime) loadSession(ctx context.Context, sessionID, externalKey string) (*SessionHandle, string, error) {
	var (
		sdkSession *sdk.Session
		err        error
	)
	sessionOptions := r.sessionOptionsSnapshot()

	if sessionOptions.ResumeSessions {
		r.hooks.emit(context.Background(), RuntimeEvent{
			Kind:        "session.resuming",
			ExternalKey: externalKey,
			SessionID:   sessionID,
		})
		sdkSession, err = r.client.ResumeSession(ctx, sessionID, sessionOptions.ResumeConfig(r.hooks, externalKey))
		if err == nil {
			return r.newSessionHandle(sessionID, externalKey, sdkSession), "session.resumed", nil
		}
		if !shouldCreateAfterResumeError(err) {
			r.hooks.emit(context.Background(), RuntimeEvent{
				Kind:        "session.resume_failed",
				ExternalKey: externalKey,
				SessionID:   sessionID,
				Err:         err,
			})
			return nil, "", fmt.Errorf("resume copilot session %q: %w", sessionID, err)
		}
		r.hooks.emit(context.Background(), RuntimeEvent{
			Kind:        "session.resume_miss",
			ExternalKey: externalKey,
			SessionID:   sessionID,
			Err:         err,
		})
	}

	r.hooks.emit(context.Background(), RuntimeEvent{
		Kind:        "session.creating",
		ExternalKey: externalKey,
		SessionID:   sessionID,
	})
	sdkSession, err = r.client.CreateSession(ctx, sessionOptions.CreateConfig(sessionID, r.hooks, externalKey))
	if err != nil {
		r.hooks.emit(context.Background(), RuntimeEvent{
			Kind:        "session.create_failed",
			ExternalKey: externalKey,
			SessionID:   sessionID,
			Err:         err,
		})
		return nil, "", fmt.Errorf("create copilot session %q: %w", sessionID, err)
	}

	return r.newSessionHandle(sessionID, externalKey, sdkSession), "session.created", nil
}

func (r *ClientRuntime) sessionOptionsSnapshot() SessionOptions {
	session := r.cfg.Session
	r.mcpMu.RLock()
	session.MCPServers = cloneMCPServers(r.mcpServers)
	r.mcpMu.RUnlock()
	return session
}

func (r *ClientRuntime) newSessionHandle(sessionID, externalKey string, session *sdk.Session) *SessionHandle {
	handle := &SessionHandle{
		externalKey: externalKey,
		sessionID:   sessionID,
		session:     session,
	}
	handle.onClose = func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if current, ok := r.sessions[sessionID]; ok && current == handle {
			delete(r.sessions, sessionID)
		}
	}
	handle.unsubscribe = session.On(func(event sdk.SessionEvent) {
		r.hooks.emit(context.Background(), RuntimeEvent{
			Kind:        "session.event",
			ExternalKey: externalKey,
			SessionID:   sessionID,
			Event:       &event,
			Message:     string(event.Type),
		})
	})
	return handle
}

func safePermissionDecision(ctx context.Context, handler PermissionHandler, event PermissionEvent) (result sdk.PermissionRequestResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("permission handler panic: %v", recovered)
			result = sdk.PermissionRequestResult{Kind: sdk.PermissionRequestResultKindDeniedCouldNotRequestFromUser}
		}
	}()
	return handler(ctx, event)
}

func shouldCreateAfterResumeError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	for _, marker := range resumeNotFoundMarkers {
		if strings.Contains(message, marker) {
			return true
		}
	}

	return false
}

func sanitizeNamespace(namespace string) string {
	namespace = strings.TrimSpace(strings.ToLower(namespace))
	if namespace == "" {
		return "copilot-session"
	}

	var b strings.Builder
	lastHyphen := false
	for _, r := range namespace {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastHyphen = false
			continue
		}
		if !lastHyphen {
			b.WriteByte('-')
			lastHyphen = true
		}
	}

	safe := strings.Trim(b.String(), "-")
	if safe == "" {
		return "copilot-session"
	}
	return safe
}

func cloneTools(tools []sdk.Tool) []sdk.Tool {
	if len(tools) == 0 {
		return nil
	}

	cloned := make([]sdk.Tool, len(tools))
	copy(cloned, tools)
	return cloned
}

func cloneProviderConfig(provider *sdk.ProviderConfig) *sdk.ProviderConfig {
	if provider == nil {
		return nil
	}

	cloned := *provider
	if provider.Azure != nil {
		azure := *provider.Azure
		cloned.Azure = &azure
	}

	return &cloned
}

func cloneMCPServers(servers map[string]sdk.MCPServerConfig) map[string]sdk.MCPServerConfig {
	if len(servers) == 0 {
		return nil
	}

	cloned := make(map[string]sdk.MCPServerConfig, len(servers))
	for name, server := range servers {
		serverClone := make(sdk.MCPServerConfig, len(server))
		for key, value := range server {
			serverClone[key] = cloneMCPValue(value)
		}
		cloned[name] = serverClone
	}

	return cloned
}

func cloneMCPValue(value any) any {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case map[string]string:
		cloned := make(map[string]string, len(typed))
		for key, entry := range typed {
			cloned[key] = entry
		}
		return cloned
	default:
		return typed
	}
}

func configMCPServersToSDK(servers map[string]config.MCPServerConfig) map[string]sdk.MCPServerConfig {
	if len(servers) == 0 {
		return nil
	}

	converted := make(map[string]sdk.MCPServerConfig, len(servers))
	for name, server := range servers {
		entry := sdk.MCPServerConfig{
			"type":  server.Type,
			"tools": append([]string(nil), server.Tools...),
		}
		if server.Timeout > 0 {
			entry["timeout"] = server.Timeout
		}

		switch server.Type {
		case "local", "stdio":
			entry["command"] = server.Command
			if len(server.Args) > 0 {
				entry["args"] = append([]string(nil), server.Args...)
			}
			if len(server.Env) > 0 {
				env := make(map[string]string, len(server.Env))
				for key, value := range server.Env {
					env[key] = value
				}
				entry["env"] = env
			}
			if strings.TrimSpace(server.Cwd) != "" {
				entry["cwd"] = server.Cwd
			}
		case "http", "sse":
			entry["url"] = server.URL
			if len(server.Headers) > 0 {
				headers := make(map[string]string, len(server.Headers))
				for key, value := range server.Headers {
					headers[key] = value
				}
				entry["headers"] = headers
			}
		}

		converted[name] = entry
	}

	return converted
}

func configProviderToSDK(provider *config.CopilotProviderConfig) *sdk.ProviderConfig {
	if provider == nil {
		return nil
	}

	converted := &sdk.ProviderConfig{
		Type:        provider.Type,
		WireApi:     provider.WireAPI,
		BaseURL:     provider.BaseURL,
		APIKey:      provider.APIKey,
		BearerToken: provider.BearerToken,
	}
	if provider.Azure != nil {
		converted.Azure = &sdk.AzureProviderOptions{
			APIVersion: provider.Azure.APIVersion,
		}
	}

	return converted
}

func defaultIfBlank(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
