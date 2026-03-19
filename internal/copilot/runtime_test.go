package copilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/github/copilot-sdk/go"

	"github.com/SCWPretorius/CONTROL/internal/config"
)

func TestEndpointClientOptionsForLocalCLI(t *testing.T) {
	t.Parallel()

	endpoint := Endpoint{
		CLIPath:    "custom-copilot",
		WorkingDir: `C:\worktree`,
	}

	options := endpoint.ClientOptions("warn")
	if options.CLIPath != "custom-copilot" {
		t.Fatalf("CLIPath = %q, want %q", options.CLIPath, "custom-copilot")
	}
	if options.CLIUrl != "" {
		t.Fatalf("CLIUrl = %q, want empty", options.CLIUrl)
	}
	if options.Cwd != `C:\worktree` {
		t.Fatalf("Cwd = %q, want %q", options.Cwd, `C:\worktree`)
	}
	if options.LogLevel != "warn" {
		t.Fatalf("LogLevel = %q, want %q", options.LogLevel, "warn")
	}
}

func TestEndpointClientOptionsForRemoteCLI(t *testing.T) {
	t.Parallel()

	endpoint := Endpoint{
		CLIPath: "ignored-local-path",
		CLIURL:  "http://127.0.0.1:7777",
	}

	options := endpoint.ClientOptions("")
	if options.CLIUrl != "http://127.0.0.1:7777" {
		t.Fatalf("CLIUrl = %q, want %q", options.CLIUrl, "http://127.0.0.1:7777")
	}
	if options.CLIPath != "" {
		t.Fatalf("CLIPath = %q, want empty for remote mode", options.CLIPath)
	}
	if options.Cwd != "" {
		t.Fatalf("Cwd = %q, want empty for remote mode", options.Cwd)
	}
	if options.LogLevel != defaultLogLevel {
		t.Fatalf("LogLevel = %q, want %q", options.LogLevel, defaultLogLevel)
	}
}

func TestExternalSessionKeySessionIDIsStableAndOpaque(t *testing.T) {
	t.Parallel()

	keyA := ExternalSessionKey{
		Identifiers: map[string]string{
			"chat_id": "123456",
			"user_id": "998877",
		},
	}
	keyB := ExternalSessionKey{
		Identifiers: map[string]string{
			"user_id": "998877",
			"chat_id": "123456",
		},
	}

	idA, canonicalA, err := keyA.SessionID("Telegram Personal Assistant")
	if err != nil {
		t.Fatalf("SessionID(keyA) error = %v", err)
	}
	idB, canonicalB, err := keyB.SessionID("Telegram Personal Assistant")
	if err != nil {
		t.Fatalf("SessionID(keyB) error = %v", err)
	}

	if canonicalA != `chat_id=123456|user_id=998877` {
		t.Fatalf("canonicalA = %q", canonicalA)
	}
	if canonicalA != canonicalB {
		t.Fatalf("canonical keys differ: %q vs %q", canonicalA, canonicalB)
	}
	if idA != idB {
		t.Fatalf("session IDs differ: %q vs %q", idA, idB)
	}
	if !strings.HasPrefix(idA, "telegram-personal-assistant-") {
		t.Fatalf("session ID %q missing sanitized namespace prefix", idA)
	}
	if strings.Contains(idA, "123456") || strings.Contains(idA, "998877") {
		t.Fatalf("session ID %q leaked raw external identifiers", idA)
	}

	legacyPayload, err := json.Marshal([]identifierPair{
		{Key: "chat_id", Value: "123456"},
		{Key: "user_id", Value: "998877"},
	})
	if err != nil {
		t.Fatalf("Marshal(legacyPayload) error = %v", err)
	}
	legacySum := sha256.Sum256([]byte("telegram-personal-assistant\n" + string(legacyPayload)))
	wantID := "telegram-personal-assistant-" + hex.EncodeToString(legacySum[:12])
	if idA != wantID {
		t.Fatalf("session ID = %q, want legacy-compatible %q", idA, wantID)
	}
}

func TestExternalSessionKeyRejectsEmptyIdentifierValues(t *testing.T) {
	t.Parallel()

	_, _, err := (ExternalSessionKey{
		Identifiers: map[string]string{"chat_id": "   "},
	}).SessionID("assistant")
	if err == nil {
		t.Fatal("SessionID error = nil, want error")
	}
}

func TestExternalSessionKeyRejectsDuplicateKeysAfterNormalization(t *testing.T) {
	t.Parallel()

	_, err := (ExternalSessionKey{
		Identifiers: map[string]string{
			" chat_id ": "1",
			"chat_id":   "2",
		},
	}).Canonical()
	if err == nil {
		t.Fatal("Canonical error = nil, want error")
	}
}

func TestExternalSessionKeyCanonicalEncodingAvoidsDelimiterCollisions(t *testing.T) {
	t.Parallel()

	keyA := ExternalSessionKey{
		Identifiers: map[string]string{"a": "1|b=2"},
	}
	keyB := ExternalSessionKey{
		Identifiers: map[string]string{"a": "1", "b": "2"},
	}

	canonicalA, err := keyA.Canonical()
	if err != nil {
		t.Fatalf("Canonical(keyA) error = %v", err)
	}
	canonicalB, err := keyB.Canonical()
	if err != nil {
		t.Fatalf("Canonical(keyB) error = %v", err)
	}
	if canonicalA == canonicalB {
		t.Fatalf("canonical encodings collided: %q", canonicalA)
	}
}

func TestParseExternalSessionKeyParsesCanonicalEncoding(t *testing.T) {
	t.Parallel()

	identifiers, err := ParseExternalSessionKey("chat_id=123456|note=hello%7Cworld%3D1|transport=telegram|user_id=998877")
	if err != nil {
		t.Fatalf("ParseExternalSessionKey() error = %v", err)
	}
	if identifiers["chat_id"] != "123456" {
		t.Fatalf("chat_id = %q, want %q", identifiers["chat_id"], "123456")
	}
	if identifiers["note"] != "hello|world=1" {
		t.Fatalf("note = %q, want decoded delimiter content", identifiers["note"])
	}
	if identifiers["transport"] != "telegram" {
		t.Fatalf("transport = %q, want %q", identifiers["transport"], "telegram")
	}
	if identifiers["user_id"] != "998877" {
		t.Fatalf("user_id = %q, want %q", identifiers["user_id"], "998877")
	}
}

func TestParseExternalSessionKeyAcceptsLegacyJSONEncoding(t *testing.T) {
	t.Parallel()

	identifiers, err := ParseExternalSessionKey(`[{"key":"chat_id","value":"123456"},{"key":"user_id","value":"998877"}]`)
	if err != nil {
		t.Fatalf("ParseExternalSessionKey() legacy error = %v", err)
	}
	if identifiers["chat_id"] != "123456" || identifiers["user_id"] != "998877" {
		t.Fatalf("ParseExternalSessionKey() = %#v", identifiers)
	}
}

func TestShouldCreateAfterResumeError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "missing session", err: errors.New("session not found"), want: true},
		{name: "unknown session", err: errors.New("unknown session id"), want: true},
		{name: "transport issue", err: errors.New("connection refused"), want: false},
		{name: "auth issue", err: errors.New("authentication failed"), want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldCreateAfterResumeError(tc.err); got != tc.want {
				t.Fatalf("shouldCreateAfterResumeError(%v) = %t, want %t", tc.err, got, tc.want)
			}
		})
	}
}

func TestSessionOptionsCreateConfigMapsFoundationValues(t *testing.T) {
	t.Parallel()

	options := SessionOptions{
		Model:           "gpt-5.4",
		ReasoningEffort: "medium",
		ResumeSessions:  true,
		WorkingDir:      `C:\repo`,
		ConfigDir:       `C:\runtime\copilot`,
		ClientName:      "control-tests",
		MCPServers: map[string]sdk.MCPServerConfig{
			"filesystem": {
				"type":    "stdio",
				"command": "npx",
				"args":    []string{"-y", "@modelcontextprotocol/server-filesystem"},
				"tools":   []string{"*"},
			},
		},
	}

	cfg := options.CreateConfig("session-123", RuntimeHooks{}, "chat_id=1")
	if cfg.SessionID != "session-123" {
		t.Fatalf("SessionID = %q, want %q", cfg.SessionID, "session-123")
	}
	if cfg.Model != "gpt-5.4" {
		t.Fatalf("Model = %q, want %q", cfg.Model, "gpt-5.4")
	}
	if cfg.ReasoningEffort != "medium" {
		t.Fatalf("ReasoningEffort = %q, want %q", cfg.ReasoningEffort, "medium")
	}
	if cfg.WorkingDirectory != `C:\repo` {
		t.Fatalf("WorkingDirectory = %q, want %q", cfg.WorkingDirectory, `C:\repo`)
	}
	if cfg.ConfigDir != `C:\runtime\copilot` {
		t.Fatalf("ConfigDir = %q, want %q", cfg.ConfigDir, `C:\runtime\copilot`)
	}
	if cfg.ClientName != "control-tests" {
		t.Fatalf("ClientName = %q, want %q", cfg.ClientName, "control-tests")
	}
	if len(cfg.Tools) != 0 {
		t.Fatalf("Tools length = %d, want %d", len(cfg.Tools), 0)
	}
	if got := cfg.MCPServers["filesystem"]["command"]; got != "npx" {
		t.Fatalf("MCPServers filesystem command = %#v, want %q", got, "npx")
	}
	if cfg.OnPermissionRequest == nil {
		t.Fatal("OnPermissionRequest = nil, want wrapper")
	}
	if cfg.InfiniteSessions == nil || cfg.InfiniteSessions.Enabled == nil || !*cfg.InfiniteSessions.Enabled {
		t.Fatal("InfiniteSessions.Enabled should be true when ResumeSessions is true")
	}
}

func TestDefaultPermissionHandlerDeniesWhenNoHookIsConfigured(t *testing.T) {
	t.Parallel()

	handler := (RuntimeHooks{}).wrapPermissionHandler("chat_id=1")
	result, err := handler(sdk.PermissionRequest{Kind: sdk.KindShell}, sdk.PermissionInvocation{SessionID: "session-123"})
	if err != nil {
		t.Fatalf("permission handler error = %v", err)
	}
	if result.Kind != sdk.PermissionRequestResultKindDeniedCouldNotRequestFromUser {
		t.Fatalf("result.Kind = %q, want %q", result.Kind, sdk.PermissionRequestResultKindDeniedCouldNotRequestFromUser)
	}
}

func TestSessionOptionsCreateAndResumeConfigIncludeCustomTools(t *testing.T) {
	t.Parallel()

	customTool := sdk.DefineTool("echo_tool", "Echo test payloads", func(input struct {
		Value string `json:"value"`
	}, invocation sdk.ToolInvocation) (map[string]string, error) {
		return map[string]string{"value": input.Value, "session": invocation.SessionID}, nil
	})
	secondTool := sdk.DefineTool("status_tool", "Report a static status", func(struct{}, sdk.ToolInvocation) (map[string]string, error) {
		return map[string]string{"status": "ok"}, nil
	})

	options := SessionOptions{
		Model:           "gpt-5.4",
		ReasoningEffort: "medium",
		ResumeSessions:  true,
		WorkingDir:      `C:\repo`,
		ConfigDir:       `C:\runtime\copilot`,
		ClientName:      "control-tests",
		Tools:           []sdk.Tool{customTool, secondTool},
		MCPServers: map[string]sdk.MCPServerConfig{
			"tickets": {
				"type":  "http",
				"url":   "https://example.com/mcp",
				"tools": []string{"list_tickets"},
			},
		},
	}

	createCfg := options.CreateConfig("session-123", RuntimeHooks{}, "chat_id=1")
	resumeCfg := options.ResumeConfig(RuntimeHooks{}, "chat_id=1")
	if len(createCfg.Tools) != 2 || createCfg.Tools[0].Name != "echo_tool" || createCfg.Tools[1].Name != "status_tool" {
		t.Fatalf("CreateConfig tools = %#v", createCfg.Tools)
	}
	if len(resumeCfg.Tools) != 2 || resumeCfg.Tools[0].Name != "echo_tool" || resumeCfg.Tools[1].Name != "status_tool" {
		t.Fatalf("ResumeConfig tools = %#v", resumeCfg.Tools)
	}
	if got := createCfg.MCPServers["tickets"]["url"]; got != "https://example.com/mcp" {
		t.Fatalf("CreateConfig MCP url = %#v", got)
	}
	if got := resumeCfg.MCPServers["tickets"]["url"]; got != "https://example.com/mcp" {
		t.Fatalf("ResumeConfig MCP url = %#v", got)
	}
	createCfg.MCPServers["tickets"]["url"] = "https://mutated.example.com/mcp"
	if got := options.MCPServers["tickets"]["url"]; got != "https://example.com/mcp" {
		t.Fatalf("options MCP url mutated to %#v, want original value", got)
	}
	if &createCfg.Tools[0] == &options.Tools[0] || &resumeCfg.Tools[0] == &options.Tools[0] {
		t.Fatal("session config tools should be cloned, not alias the source slice")
	}
	if &createCfg.Tools[1] == &options.Tools[1] || &resumeCfg.Tools[1] == &options.Tools[1] {
		t.Fatal("session config tools should clone every tool entry")
	}
}

func TestConfigFromFoundationMapsExistingConfig(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Copilot: config.CopilotConfig{
			CLIPath: "copilot",
		},
		Session: config.SessionConfig{
			Model:           "gpt-5.4",
			ReasoningEffort: "high",
			Namespace:       "telegram-personal-assistant",
			ResumeSessions:  true,
			WorkingDir:      `C:\repo`,
			ConfigDir:       `C:\repo\var\runtime\copilot`,
		},
		Tools: config.ToolConfig{
			MCP: config.MCPToolConfig{
				Servers: map[string]config.MCPServerConfig{
					"filesystem": {
						Type:    "stdio",
						Command: "npx",
						Args:    []string{"-y", "@modelcontextprotocol/server-filesystem"},
						Tools:   []string{"*"},
					},
				},
			},
		},
	}

	runtimeCfg := ConfigFromFoundation(cfg)
	if runtimeCfg.Endpoint.CLIPath != "copilot" {
		t.Fatalf("Endpoint.CLIPath = %q, want %q", runtimeCfg.Endpoint.CLIPath, "copilot")
	}
	if runtimeCfg.Session.Model != "gpt-5.4" {
		t.Fatalf("Session.Model = %q, want %q", runtimeCfg.Session.Model, "gpt-5.4")
	}
	if runtimeCfg.Session.Namespace != "telegram-personal-assistant" {
		t.Fatalf("Session.Namespace = %q, want %q", runtimeCfg.Session.Namespace, "telegram-personal-assistant")
	}
	if got := runtimeCfg.Session.MCPServers["filesystem"]["command"]; got != "npx" {
		t.Fatalf("Session.MCPServers filesystem command = %#v, want %q", got, "npx")
	}
	if runtimeCfg.LogLevel != defaultLogLevel {
		t.Fatalf("LogLevel = %q, want %q", runtimeCfg.LogLevel, defaultLogLevel)
	}
}

func TestStartSerializesConcurrentCalls(t *testing.T) {
	t.Parallel()

	client := &fakeClient{
		startGate:  make(chan struct{}),
		startedSig: make(chan struct{}, 1),
	}
	runtime := &ClientRuntime{
		client:   client,
		sessions: make(map[string]*SessionHandle),
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- runtime.Start(context.Background())
		}()
	}

	select {
	case <-client.startedSig:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first Start call")
	}

	if got := client.startCalls.Load(); got != 1 {
		t.Fatalf("start calls after first start = %d, want %d", got, 1)
	}

	close(client.startGate)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
	}

	if got := client.startCalls.Load(); got != 1 {
		t.Fatalf("start calls after completion = %d, want %d", got, 1)
	}
}

type fakeClient struct {
	startCalls atomic.Int32
	startGate  chan struct{}
	startedSig chan struct{}
}

func (f *fakeClient) Start(context.Context) error {
	f.startCalls.Add(1)
	if f.startedSig != nil {
		select {
		case f.startedSig <- struct{}{}:
		default:
		}
	}
	if f.startGate != nil {
		<-f.startGate
	}
	return nil
}

func (f *fakeClient) Stop() error {
	return nil
}

func (f *fakeClient) CreateSession(context.Context, *sdk.SessionConfig) (*sdk.Session, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeClient) ResumeSession(context.Context, string, *sdk.ResumeSessionConfig) (*sdk.Session, error) {
	return nil, errors.New("not implemented")
}
