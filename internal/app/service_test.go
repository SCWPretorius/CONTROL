package app

import (
	"bytes"
	"context"
	"errors"
	"log"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/github/copilot-sdk/go"

	"github.com/SCWPretorius/CONTROL/internal/config"
	"github.com/SCWPretorius/CONTROL/internal/copilot"
	"github.com/SCWPretorius/CONTROL/internal/router"
	"github.com/SCWPretorius/CONTROL/internal/store"
	"github.com/SCWPretorius/CONTROL/internal/telegram"
	"github.com/SCWPretorius/CONTROL/internal/tools/googleworkspace/copilottools"
	"github.com/SCWPretorius/CONTROL/internal/tools/privileged"
)

func TestParseExternalKeyParsesCanonicalPipeDelimitedValue(t *testing.T) {
	t.Parallel()

	ref := parseExternalKey("chat_id=77|transport=telegram|user_id=88")
	if ref.Transport != "telegram" {
		t.Fatalf("Transport = %q, want %q", ref.Transport, "telegram")
	}
	if ref.ChatID != 77 {
		t.Fatalf("ChatID = %d, want %d", ref.ChatID, 77)
	}
	if ref.UserID != 88 {
		t.Fatalf("UserID = %d, want %d", ref.UserID, 88)
	}
}

func TestBuildRuntimeToolsetIncludesPrivilegedToolsAndSkipsGoogleWithoutToken(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	var logs bytes.Buffer
	toolset, err := buildRuntimeToolset(config.Config{
		Paths: config.PathConfig{
			RuntimeDir: root,
			StorageDir: root,
		},
		Tools: config.ToolConfig{
			Google: config.GoogleToolConfig{
				OAuth: config.GoogleOAuthConfig{
					ClientID:     "client-id",
					ClientSecret: "client-secret",
					RedirectURL:  "http://127.0.0.1:8787/oauth/callback",
					Scopes:       []string{"scope-a"},
				},
			},
			Privileged: config.PrivilegedToolConfig{
				AllowedWorkspaceRoots:  []string{root},
				AssistantWritableRoots: []string{root},
			},
			Runtime: config.ToolRuntimeConfig{
				HTTPTimeout:           time.Second,
				ShellCommandTimeout:   time.Second,
				MaxCommandOutputBytes: 1024,
			},
		},
	}, log.New(&logs, "", 0), nil)
	if err != nil {
		t.Fatalf("buildRuntimeToolset() error = %v", err)
	}
	if !toolset.PrivilegedEnabled {
		t.Fatal("PrivilegedEnabled = false, want true")
	}
	if toolset.GoogleEnabled {
		t.Fatal("GoogleEnabled = true, want false when access token is absent")
	}
	if len(toolset.Tools) != 2 {
		t.Fatalf("tool count = %d, want %d", len(toolset.Tools), 2)
	}
	if got := toolset.Tools[0].Name; got != privileged.ToolNameFileWrite {
		t.Fatalf("tool[0] = %q, want %q", got, privileged.ToolNameFileWrite)
	}
	if got := toolset.Tools[1].Name; got != privileged.ToolNameShell {
		t.Fatalf("tool[1] = %q, want %q", got, privileged.ToolNameShell)
	}
	if logs.String() == "" {
		t.Fatal("expected google disabled log message")
	}
}

func TestBuildRuntimeToolsetIncludesGoogleToolsWhenConfigured(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := config.Config{
		Paths: config.PathConfig{
			RuntimeDir: root,
			StorageDir: root,
		},
		Tools: config.ToolConfig{
			Google: config.GoogleToolConfig{
				OAuth: config.GoogleOAuthConfig{
					ClientID:     "client-id",
					ClientSecret: "client-secret",
					RedirectURL:  "http://127.0.0.1:8787/oauth/callback",
					Scopes:       []string{"scope-a"},
				},
				AccessToken: "access-token",
			},
			Privileged: config.PrivilegedToolConfig{
				AllowedWorkspaceRoots:  []string{root},
				AssistantWritableRoots: []string{root},
			},
			Runtime: config.ToolRuntimeConfig{
				HTTPTimeout:           time.Second,
				ShellCommandTimeout:   time.Second,
				MaxCommandOutputBytes: 1024,
			},
		},
	}

	toolset, err := buildRuntimeToolset(cfg, nil, nil)
	if err != nil {
		t.Fatalf("buildRuntimeToolset() error = %v", err)
	}
	if !toolset.PrivilegedEnabled || !toolset.GoogleEnabled {
		t.Fatalf("toolset enabled flags = %#v", toolset)
	}
	assertToolNames(t, toolset.Tools, expectedRuntimeToolNames())
}

func TestRuntimeConfigSessionToolsCarryGoogleWorkspaceToolsIntoSDKConfigs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := config.Config{
		Copilot: config.CopilotConfig{
			CLIPath: "copilot",
		},
		Session: config.SessionConfig{
			Model:           "gpt-5.4",
			ReasoningEffort: "medium",
			Namespace:       "telegram-personal-assistant",
			ResumeSessions:  true,
			WorkingDir:      root,
			ConfigDir:       root,
		},
		Paths: config.PathConfig{
			RuntimeDir: root,
			StorageDir: root,
		},
		Tools: config.ToolConfig{
			Google: config.GoogleToolConfig{
				OAuth: config.GoogleOAuthConfig{
					ClientID:     "client-id",
					ClientSecret: "client-secret",
					RedirectURL:  "http://127.0.0.1:8787/oauth/callback",
					Scopes:       []string{"scope-a"},
				},
				AccessToken: "access-token",
			},
			Privileged: config.PrivilegedToolConfig{
				AllowedWorkspaceRoots:  []string{root},
				AssistantWritableRoots: []string{root},
			},
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
			Runtime: config.ToolRuntimeConfig{
				HTTPTimeout:           time.Second,
				ShellCommandTimeout:   time.Second,
				MaxCommandOutputBytes: 1024,
			},
		},
	}

	toolset, err := buildRuntimeToolset(cfg, nil, nil)
	if err != nil {
		t.Fatalf("buildRuntimeToolset() error = %v", err)
	}

	runtimeCfg := copilot.ConfigFromFoundation(cfg)
	runtimeCfg.Session.Tools = toolset.Tools

	createCfg := runtimeCfg.Session.CreateConfig("session-123", copilot.RuntimeHooks{}, "chat_id=77|transport=telegram|user_id=88")
	resumeCfg := runtimeCfg.Session.ResumeConfig(copilot.RuntimeHooks{}, "chat_id=77|transport=telegram|user_id=88")

	assertToolNames(t, runtimeCfg.Session.Tools, expectedRuntimeToolNames())
	assertToolNames(t, createCfg.Tools, expectedRuntimeToolNames())
	assertToolNames(t, resumeCfg.Tools, expectedRuntimeToolNames())
	if got := createCfg.MCPServers["filesystem"]["command"]; got != "npx" {
		t.Fatalf("CreateConfig MCP filesystem command = %#v, want %q", got, "npx")
	}
	if got := resumeCfg.MCPServers["filesystem"]["command"]; got != "npx" {
		t.Fatalf("ResumeConfig MCP filesystem command = %#v, want %q", got, "npx")
	}
}

func TestPermissionRequestResultDeniesReadOnlyGoogleToolOutsideTelegram(t *testing.T) {
	t.Parallel()

	toolName := copilottools.ToolNameCalendarListEvents
	readOnly := true
	result := permissionRequestResult(copilot.PermissionEvent{
		ExternalKey: "chat_id=77|transport=slack|user_id=88",
		SessionID:   "session-123",
		Request: sdk.PermissionRequest{
			Kind:     sdk.CustomTool,
			ToolName: &toolName,
			ReadOnly: &readOnly,
		},
	})

	if result.Kind != sdk.PermissionRequestResultKindDeniedCouldNotRequestFromUser {
		t.Fatalf("result.Kind = %q, want %q", result.Kind, sdk.PermissionRequestResultKindDeniedCouldNotRequestFromUser)
	}
}

func TestPermissionRequestResultApprovesReadOnlyGoogleToolForTelegram(t *testing.T) {
	t.Parallel()

	toolName := copilottools.ToolNameCalendarListEvents
	readOnly := true
	result := permissionRequestResult(copilot.PermissionEvent{
		ExternalKey: "chat_id=77|transport=telegram|user_id=88",
		SessionID:   "session-123",
		Request: sdk.PermissionRequest{
			Kind:     sdk.CustomTool,
			ToolName: &toolName,
			ReadOnly: &readOnly,
		},
	})

	if result.Kind != sdk.PermissionRequestResultKindApproved {
		t.Fatalf("result.Kind = %q, want %q", result.Kind, sdk.PermissionRequestResultKindApproved)
	}
}

func TestPermissionRequestResultApprovesKnownReadOnlyGoogleToolForTelegramWhenReadonlyMetadataIsMissing(t *testing.T) {
	t.Parallel()

	toolName := copilottools.ToolNameCalendarListEvents
	result := permissionRequestResult(copilot.PermissionEvent{
		ExternalKey: "chat_id=77|transport=telegram|user_id=88",
		SessionID:   "session-123",
		Request: sdk.PermissionRequest{
			Kind:     sdk.CustomTool,
			ToolName: &toolName,
		},
	})

	if result.Kind != sdk.PermissionRequestResultKindApproved {
		t.Fatalf("result.Kind = %q, want %q", result.Kind, sdk.PermissionRequestResultKindApproved)
	}
}

func TestPermissionRequestResultDeniesMCPToolForTelegram(t *testing.T) {
	t.Parallel()

	result := permissionRequestResult(copilot.PermissionEvent{
		ExternalKey: "chat_id=77|transport=telegram|user_id=88",
		SessionID:   "session-123",
		Request: sdk.PermissionRequest{
			Kind: sdk.MCP,
		},
	})

	if result.Kind != sdk.PermissionRequestResultKindDeniedCouldNotRequestFromUser {
		t.Fatalf("result.Kind = %q, want %q", result.Kind, sdk.PermissionRequestResultKindDeniedCouldNotRequestFromUser)
	}
}

func TestPermissionRequestResultDeniesWriteGoogleToolForTelegram(t *testing.T) {
	t.Parallel()

	toolName := copilottools.ToolNameCalendarCreateEvent
	readOnly := false
	result := permissionRequestResult(copilot.PermissionEvent{
		ExternalKey: "chat_id=77|transport=telegram|user_id=88",
		SessionID:   "session-123",
		Request: sdk.PermissionRequest{
			Kind:     sdk.CustomTool,
			ToolName: &toolName,
			ReadOnly: &readOnly,
		},
	})

	if result.Kind != sdk.PermissionRequestResultKindDeniedCouldNotRequestFromUser {
		t.Fatalf("result.Kind = %q, want %q", result.Kind, sdk.PermissionRequestResultKindDeniedCouldNotRequestFromUser)
	}
}

func TestPermissionRequestResultDeniesUnknownCustomToolForTelegramWhenReadonlyMetadataIsMissing(t *testing.T) {
	t.Parallel()

	toolName := "google_calendar_delete_event"
	result := permissionRequestResult(copilot.PermissionEvent{
		ExternalKey: "chat_id=77|transport=telegram|user_id=88",
		SessionID:   "session-123",
		Request: sdk.PermissionRequest{
			Kind:     sdk.CustomTool,
			ToolName: &toolName,
		},
	})

	if result.Kind != sdk.PermissionRequestResultKindDeniedCouldNotRequestFromUser {
		t.Fatalf("result.Kind = %q, want %q", result.Kind, sdk.PermissionRequestResultKindDeniedCouldNotRequestFromUser)
	}
}

func TestRuntimeHooksAuditApprovedGoogleTool(t *testing.T) {
	t.Parallel()

	toolName := copilottools.ToolNameCalendarListEvents
	readOnly := true
	var logs bytes.Buffer
	auditStore := &stubPrivilegedToolEventStore{}
	hooks := runtimeHooks(log.New(&logs, "", 0), auditStore, nil)

	result, err := hooks.OnPermissionRequest(context.Background(), copilot.PermissionEvent{
		ExternalKey: "chat_id=77|transport=telegram|user_id=88",
		SessionID:   "session-123",
		Request: sdk.PermissionRequest{
			Kind:     sdk.CustomTool,
			ToolName: &toolName,
			ReadOnly: &readOnly,
		},
	})
	if err != nil {
		t.Fatalf("OnPermissionRequest() error = %v", err)
	}
	if result.Kind != sdk.PermissionRequestResultKindApproved {
		t.Fatalf("result.Kind = %q, want %q", result.Kind, sdk.PermissionRequestResultKindApproved)
	}
	if len(auditStore.events) != 1 {
		t.Fatalf("audit event count = %d, want %d", len(auditStore.events), 1)
	}
	record := auditStore.events[0]
	if record.ToolName != toolName {
		t.Fatalf("record.ToolName = %q, want %q", record.ToolName, toolName)
	}
	if record.Outcome != string(sdk.PermissionRequestResultKindApproved) {
		t.Fatalf("record.Outcome = %q, want %q", record.Outcome, sdk.PermissionRequestResultKindApproved)
	}
	if !strings.Contains(record.Summary, "auto-approved") {
		t.Fatalf("record.Summary = %q, want auto-approved summary", record.Summary)
	}
	if !strings.Contains(logs.String(), "permission approved") {
		t.Fatalf("logs = %q, want approved decision log", logs.String())
	}
}

type stubPrivilegedToolEventStore struct {
	events []store.PrivilegedToolEvent
}

func (s *stubPrivilegedToolEventStore) Append(_ context.Context, event store.PrivilegedToolEvent) error {
	s.events = append(s.events, event)
	return nil
}

func (s *stubPrivilegedToolEventStore) Load(context.Context) ([]store.PrivilegedToolEvent, error) {
	return append([]store.PrivilegedToolEvent(nil), s.events...), nil
}

func expectedRuntimeToolNames() []string {
	return []string{
		privileged.ToolNameFileWrite,
		privileged.ToolNameShell,
		copilottools.ToolNameGmailSearchMessages,
		copilottools.ToolNameGmailGetMessage,
		copilottools.ToolNameGmailSendMessage,
		copilottools.ToolNameCalendarListCalendars,
		copilottools.ToolNameCalendarListEvents,
		copilottools.ToolNameCalendarCreateEvent,
	}
}

func assertToolNames(t *testing.T, tools []sdk.Tool, want []string) {
	t.Helper()

	got := make([]string, 0, len(tools))
	for _, tool := range tools {
		got = append(got, tool.Name)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("tool names = %v, want %v", got, want)
	}
}

func TestHandleTelegramMessageSendsTypingAndReply(t *testing.T) {
	t.Parallel()

	typingSeen := make(chan struct{})
	responder := &stubTelegramResponder{
		onTyping: func() {
			select {
			case <-typingSeen:
			default:
				close(typingSeen)
			}
		},
	}

	err := handleTelegramMessage(context.Background(), nil, responder, router.Message{
		Transport: telegram.TransportName,
		ChatID:    77,
		UserID:    88,
	}, func(ctx context.Context, message router.Message) (string, error) {
		<-typingSeen
		return "done", nil
	})
	if err != nil {
		t.Fatalf("handleTelegramMessage() error = %v", err)
	}
	if responder.replyText != "done" {
		t.Fatalf("replyText = %q, want %q", responder.replyText, "done")
	}
	if responder.typingCount.Load() == 0 {
		t.Fatal("expected typing indicator before reply")
	}
}

func TestStartTelegramTypingLoopRepeatsForTelegramMessages(t *testing.T) {
	t.Parallel()

	responder := &stubTelegramResponder{}
	stop := startTelegramTypingLoop(context.Background(), nil, responder, router.Message{
		Transport: telegram.TransportName,
		ChatID:    77,
		UserID:    88,
	}, 5*time.Millisecond)
	defer stop()

	deadline := time.Now().Add(200 * time.Millisecond)
	for responder.typingCount.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	if responder.typingCount.Load() < 2 {
		t.Fatalf("typingCount = %d, want at least 2", responder.typingCount.Load())
	}
}

func TestStartTelegramTypingLoopSkipsNonTelegramTransport(t *testing.T) {
	t.Parallel()

	responder := &stubTelegramResponder{}
	stop := startTelegramTypingLoop(context.Background(), nil, responder, router.Message{
		Transport: "slack",
		ChatID:    77,
		UserID:    88,
	}, 5*time.Millisecond)
	defer stop()

	time.Sleep(20 * time.Millisecond)
	if responder.typingCount.Load() != 0 {
		t.Fatalf("typingCount = %d, want %d", responder.typingCount.Load(), 0)
	}
}

func TestHandleTelegramMessageContinuesWhenTypingIndicatorFails(t *testing.T) {
	t.Parallel()

	responder := &stubTelegramResponder{
		typingErr: errors.New("typing failed"),
	}

	err := handleTelegramMessage(context.Background(), nil, responder, router.Message{
		Transport: telegram.TransportName,
		ChatID:    77,
		UserID:    88,
	}, func(context.Context, router.Message) (string, error) {
		return "done", nil
	})
	if err != nil {
		t.Fatalf("handleTelegramMessage() error = %v", err)
	}
	if responder.replyText != "done" {
		t.Fatalf("replyText = %q, want %q", responder.replyText, "done")
	}
}

func TestStartMonitorLifecycleRunsInBackgroundAndStopsWithContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{})
	runner := &stubMonitorLifecycle{
		run: func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return nil
		},
	}

	start := time.Now()
	wait := startMonitorLifecycle(ctx, runner)
	select {
	case <-started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("monitor runner did not start")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("startMonitorLifecycle() blocked for %s", elapsed)
	}

	cancel()
	if err := wait(); err != nil {
		t.Fatalf("wait() error = %v", err)
	}
	if runner.calls.Load() != 1 {
		t.Fatalf("Run() calls = %d, want 1", runner.calls.Load())
	}
}

type stubTelegramResponder struct {
	typingCount atomic.Int32
	replyText   string
	typingErr   error
	onTyping    func()
	mu          sync.Mutex
}

func (s *stubTelegramResponder) SendTyping(context.Context, router.Message) error {
	s.typingCount.Add(1)
	if s.onTyping != nil {
		s.onTyping()
	}
	return s.typingErr
}

func (s *stubTelegramResponder) Reply(_ context.Context, _ router.Message, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replyText = text
	return nil
}

type stubMonitorLifecycle struct {
	calls atomic.Int32
	run   func(context.Context) error
}

func (s *stubMonitorLifecycle) Run(ctx context.Context) error {
	s.calls.Add(1)
	if s.run != nil {
		return s.run(ctx)
	}
	return nil
}
