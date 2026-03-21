package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	sdk "github.com/github/copilot-sdk/go"

	"github.com/SCWPretorius/CONTROL/internal/config"
	"github.com/SCWPretorius/CONTROL/internal/copilot"
	"github.com/SCWPretorius/CONTROL/internal/router"
	"github.com/SCWPretorius/CONTROL/internal/store"
	"github.com/SCWPretorius/CONTROL/internal/telegram"
	"github.com/SCWPretorius/CONTROL/internal/tools/googleworkspace/copilottools"
)

var telegramAutoApprovedReadOnlyTools = map[string]struct{}{
	copilottools.ToolNameCalendarListCalendars: {},
	copilottools.ToolNameCalendarListEvents:    {},
	copilottools.ToolNameGmailSearchMessages:   {},
	copilottools.ToolNameGmailGetMessage:       {},
}

const telegramTypingInterval = 4 * time.Second

type telegramResponder interface {
	SendTyping(context.Context, router.Message) error
	Reply(context.Context, router.Message, string) error
}

type telegramMessageHandler func(context.Context, router.Message) (string, error)

// Run wires the runtime, persistence, router, and Telegram polling loop into a
// runnable assistant service.
func Run(ctx context.Context, cfg config.Config, logger *log.Logger) error {
	logger = defaultLogger(logger)

	if err := Prepare(cfg); err != nil {
		return fmt.Errorf("prepare app: %w", err)
	}

	sessionStore, err := store.NewLocalFileStore(cfg.Paths.StorageDir)
	if err != nil {
		return fmt.Errorf("create local store: %w", err)
	}

	statusTracker := newRuntimeStatusTracker()

	// Initialize Google OAuth if needed
	if token, err := InitializeGoogleOAuth(ctx, cfg, logger); err != nil {
		logger.Printf("google workspace: oauth initialization failed (optional): %v", err)
		// Continue anyway - Google tools will be disabled
	} else if strings.TrimSpace(token) != "" {
		cfg.Tools.Google.AccessToken = token
	}

	toolset, err := buildRuntimeToolset(cfg, logger, sessionStore)
	if err != nil {
		return fmt.Errorf("compose runtime tools: %w", err)
	}

	runtimeCfg := copilot.ConfigFromFoundation(cfg)
	runtimeCfg.Session.Tools = toolset.Tools
	runtime := copilot.NewRuntime(runtimeCfg, runtimeHooks(logger, sessionStore, statusTracker))
	if err := runtime.Start(ctx); err != nil {
		return fmt.Errorf("start copilot runtime: %w", err)
	}
	stopMCPAdmin, err := startMCPAdminServer(ctx, cfg, logger, runtime)
	if err != nil {
		closeErr := runtime.Close()
		return errors.Join(fmt.Errorf("start MCP admin server: %w", err), closeErr)
	}

	orchestrator, err := router.NewOrchestrator(
		sessionResolver{runtime: runtime},
		sessionStore,
		router.WithPrivilegedToolEventStore(sessionStore),
		router.WithRuntimeStatusProvider(statusTracker),
	)
	if err != nil {
		adminErr := stopMCPAdmin()
		closeErr := runtime.Close()
		return errors.Join(err, adminErr, closeErr)
	}

	var adapter *telegram.Adapter
	handler := func(msgCtx context.Context, message router.Message) error {
		return handleTelegramMessage(msgCtx, logger, adapter, message, orchestrator.HandleMessage)
	}

	adapter, err = telegram.New(
		cfg.Telegram.BotToken,
		telegram.AccessControl{
			AllowedUserID: cfg.Telegram.AllowedUserID,
			AllowedChatID: cfg.Telegram.AllowedChatID,
		},
		handler,
		telegram.WithErrorHandler(func(err error) {
			logger.Printf("telegram polling error: %v", err)
		}),
	)
	if err != nil {
		adminErr := stopMCPAdmin()
		closeErr := runtime.Close()
		return errors.Join(fmt.Errorf("create telegram adapter: %w", err), adminErr, closeErr)
	}

	logger.Printf(
		"assistant ready transport=%s model=%s namespace=%s allowed_user=%d allowed_chat=%d resume_sessions=%t runtime_dir=%s storage_dir=%s privileged_tools=%t google_tools=%t mcp_server_count=%d mcp_admin_enabled=%t custom_tool_count=%d",
		cfg.Copilot.Transport(),
		cfg.Session.Model,
		cfg.Session.Namespace,
		cfg.Telegram.AllowedUserID,
		cfg.Telegram.AllowedChatID,
		cfg.Session.ResumeSessions,
		cfg.Paths.RuntimeDir,
		cfg.Paths.StorageDir,
		toolset.PrivilegedEnabled,
		toolset.GoogleEnabled,
		len(cfg.Tools.MCP.Servers),
		cfg.Tools.MCP.Admin.Enabled(),
		len(toolset.Tools),
	)

	runErr := adapter.Start(ctx)
	adminErr := stopMCPAdmin()
	closeErr := runtime.Close()
	return errors.Join(runErr, adminErr, closeErr)
}

type sessionResolver struct {
	runtime copilot.Runtime
}

func (r sessionResolver) EnsureSession(ctx context.Context, key copilot.ExternalSessionKey) (router.Session, error) {
	return r.runtime.EnsureSession(ctx, key)
}

func runtimeHooks(logger *log.Logger, auditStore store.PrivilegedToolEventStore, tracker *runtimeStatusTracker) copilot.RuntimeHooks {
	return copilot.RuntimeHooks{
		OnPermissionRequest: func(ctx context.Context, event copilot.PermissionEvent) (sdk.PermissionRequestResult, error) {
			result := permissionRequestResult(event)
			toolName := permissionAuditToolName(event.Request)
			decision := "denied"
			if result.Kind == sdk.PermissionRequestResultKindApproved {
				decision = "approved"
			}
			logger.Printf(
				"copilot permission %s session=%s external_key=%s kind=%s tool=%s",
				decision,
				event.SessionID,
				event.ExternalKey,
				event.Request.Kind,
				toolName,
			)
			if auditStore != nil {
				record := newPermissionAuditRecord(event, result)
				if err := auditStore.Append(ctx, record); err != nil {
					logger.Printf("persist permission audit failed session=%s err=%v", event.SessionID, err)
				}
			}
			return result, nil
		},
		OnEvent: func(ctx context.Context, event copilot.RuntimeEvent) {
			if tracker != nil {
				tracker.Observe(event)
			}
			logger.Printf(
				"copilot event kind=%s session=%s external_key=%s message=%s err=%v",
				event.Kind,
				event.SessionID,
				event.ExternalKey,
				event.Message,
				event.Err,
			)
			if auditStore != nil {
				record, ok := newRuntimeAuditRecord(event)
				if ok {
					if err := auditStore.Append(ctx, record); err != nil {
						logger.Printf("persist runtime audit failed kind=%s session=%s err=%v", event.Kind, event.SessionID, err)
					}
				}
			}
		},
	}
}

func defaultLogger(logger *log.Logger) *log.Logger {
	if logger != nil {
		return logger
	}
	return log.New(io.Discard, "", 0)
}

func handleTelegramMessage(ctx context.Context, logger *log.Logger, responder telegramResponder, message router.Message, handler telegramMessageHandler) error {
	logger = defaultLogger(logger)

	stopTyping := startTelegramTypingLoop(ctx, logger, responder, message, telegramTypingInterval)
	defer stopTyping()

	reply, err := handler(ctx, message)
	if err != nil {
		logger.Printf("assistant message failed transport=%s chat=%d user=%d err=%v", message.Transport, message.ChatID, message.UserID, err)
		return err
	}

	if err := responder.Reply(ctx, message, reply); err != nil {
		logger.Printf("assistant reply failed transport=%s chat=%d user=%d err=%v", message.Transport, message.ChatID, message.UserID, err)
		return err
	}

	return nil
}

func startTelegramTypingLoop(ctx context.Context, logger *log.Logger, responder telegramResponder, message router.Message, interval time.Duration) func() {
	logger = defaultLogger(logger)

	if responder == nil || strings.TrimSpace(message.Transport) != telegram.TransportName || interval <= 0 {
		return func() {}
	}

	typingCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	sendTyping := func() {
		if err := responder.SendTyping(typingCtx, message); err != nil && typingCtx.Err() == nil {
			logger.Printf("telegram typing indicator failed transport=%s chat=%d user=%d err=%v", message.Transport, message.ChatID, message.UserID, err)
		}
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer close(done)
		defer ticker.Stop()
		sendTyping()
		for {
			select {
			case <-typingCtx.Done():
				return
			case <-ticker.C:
				sendTyping()
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}
}

func newPermissionAuditRecord(event copilot.PermissionEvent, result sdk.PermissionRequestResult) store.PrivilegedToolEvent {
	ref := parseExternalKey(event.ExternalKey)
	toolName := permissionAuditToolName(event.Request)
	return store.PrivilegedToolEvent{
		ChatID:     ref.ChatID,
		UserID:     ref.UserID,
		SessionID:  event.SessionID,
		ToolName:   toolName,
		EventType:  "permission.decision",
		Outcome:    string(result.Kind),
		Summary:    permissionDecisionSummary(event, result),
		OccurredAt: time.Now().UTC(),
		Metadata: map[string]string{
			"external_key": event.ExternalKey,
			"request_kind": string(event.Request.Kind),
			"tool_name":    toolName,
			"transport":    ref.Transport,
		},
	}
}

func permissionRequestResult(event copilot.PermissionEvent) sdk.PermissionRequestResult {
	if shouldAutoApprovePermissionRequest(event) {
		return sdk.PermissionRequestResult{Kind: sdk.PermissionRequestResultKindApproved}
	}
	return sdk.PermissionRequestResult{Kind: sdk.PermissionRequestResultKindDeniedCouldNotRequestFromUser}
}

func shouldAutoApprovePermissionRequest(event copilot.PermissionEvent) bool {
	if event.Request.Kind != sdk.CustomTool {
		return false
	}
	ref := parseExternalKey(event.ExternalKey)
	if ref.Transport != "telegram" {
		return false
	}

	toolName := strings.TrimSpace(pointerValue(event.Request.ToolName))
	if _, ok := telegramAutoApprovedReadOnlyTools[toolName]; !ok {
		return false
	}

	// The Go SDK tool definition does not let callers mark a custom tool as
	// read-only up front, so the runtime may omit ReadOnly for safe tools we
	// already know by name. Still reject any tool explicitly marked writable.
	if event.Request.ReadOnly != nil && !*event.Request.ReadOnly {
		return false
	}
	return true
}

func permissionAuditToolName(request sdk.PermissionRequest) string {
	if toolName := strings.TrimSpace(pointerValue(request.ToolName)); toolName != "" {
		return toolName
	}
	return string(request.Kind)
}

func permissionDecisionSummary(event copilot.PermissionEvent, result sdk.PermissionRequestResult) string {
	if result.Kind == sdk.PermissionRequestResultKindApproved {
		return fmt.Sprintf(
			"CONTROL auto-approved read-only Google Workspace tool %q for Telegram session",
			permissionAuditToolName(event.Request),
		)
	}
	return "CONTROL denied the permission request because Telegram operator prompts are disabled"
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func newRuntimeAuditRecord(event copilot.RuntimeEvent) (store.PrivilegedToolEvent, bool) {
	toolName := strings.TrimSpace(event.Metadata["tool_name"])
	switch event.Kind {
	case "hook.pre_tool_use", "hook.post_tool_use":
		ref := parseExternalKey(event.ExternalKey)
		outcome := "started"
		if event.Kind == "hook.post_tool_use" {
			outcome = "completed"
		}
		return store.PrivilegedToolEvent{
			ChatID:     ref.ChatID,
			UserID:     ref.UserID,
			SessionID:  event.SessionID,
			ToolName:   toolName,
			EventType:  event.Kind,
			Outcome:    outcome,
			Summary:    event.Message,
			OccurredAt: event.OccurredAt,
			Metadata:   cloneMetadata(event.Metadata),
		}, true
	case "runtime.started", "runtime.stopped", "runtime.start_failed", "session.created", "session.resumed", "session.resume_failed", "session.create_failed":
		ref := parseExternalKey(event.ExternalKey)
		summary := event.Message
		if summary == "" && event.Err != nil {
			summary = event.Err.Error()
		}
		return store.PrivilegedToolEvent{
			ChatID:     ref.ChatID,
			UserID:     ref.UserID,
			SessionID:  event.SessionID,
			ToolName:   auditToolName(event.Kind),
			EventType:  event.Kind,
			Outcome:    auditOutcome(event),
			Summary:    summary,
			OccurredAt: event.OccurredAt,
			Metadata:   cloneMetadata(event.Metadata),
		}, true
	default:
		return store.PrivilegedToolEvent{}, false
	}
}

type externalKeyRef struct {
	Transport string
	ChatID    int64
	UserID    int64
}

func parseExternalKey(externalKey string) externalKeyRef {
	identifiers, err := copilot.ParseExternalSessionKey(externalKey)
	if err != nil {
		return externalKeyRef{}
	}

	ref := externalKeyRef{}
	for key, value := range identifiers {
		switch key {
		case "transport":
			ref.Transport = value
		case "chat_id":
			ref.ChatID, _ = strconv.ParseInt(value, 10, 64)
		case "user_id":
			ref.UserID, _ = strconv.ParseInt(value, 10, 64)
		}
	}
	return ref
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		result[key] = value
	}
	return result
}

func auditToolName(kind string) string {
	switch {
	case strings.HasPrefix(kind, "runtime."):
		return "runtime"
	case strings.HasPrefix(kind, "session."):
		return "session"
	default:
		return "control"
	}
}

func auditOutcome(event copilot.RuntimeEvent) string {
	if event.Err != nil {
		return "error"
	}
	switch event.Kind {
	case "runtime.started", "session.created", "session.resumed":
		return "ok"
	case "runtime.stopped":
		return "stopped"
	default:
		return "observed"
	}
}
