package router

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SCWPretorius/CONTROL/internal/store"
)

const defaultAuditEventLimit = 5

// RuntimeStatus is the app-owned runtime health snapshot surfaced by admin commands.
type RuntimeStatus struct {
	Running            bool
	StartedAt          time.Time
	LastEventAt        time.Time
	LastEventKind      string
	LastError          string
	EventCount         int
	PermissionRequests int
	ToolCalls          int
}

// RuntimeStatusProvider returns the latest runtime health snapshot for admin inspection.
type RuntimeStatusProvider interface {
	Snapshot() RuntimeStatus
}

func (o *Orchestrator) handleAdminCommand(ctx context.Context, prompt string, message Message, existing store.SessionBinding, found bool) (bool, string, error) {
	command, args, ok := parseAdminCommand(prompt)
	if !ok {
		return false, "", nil
	}

	switch command {
	case "help", "start":
		return true, o.helpText(), nil
	case "status", "health":
		reply, err := o.statusText(ctx, message, existing, found)
		return true, reply, err
	case "session":
		reply, err := o.sessionText(ctx, message, existing, found, args)
		return true, reply, err
	case "sessions":
		reply, err := o.sessionsText(ctx)
		return true, reply, err
	case "audit":
		reply, err := o.auditText(ctx, args)
		return true, reply, err
	case "reset":
		reply, err := o.resetText(ctx, message, existing, found)
		return true, reply, err
	default:
		return true, fmt.Sprintf("Unknown admin command %q.\n\n%s", command, o.helpText()), nil
	}
}

func parseAdminCommand(prompt string) (string, []string, bool) {
	fields := strings.Fields(strings.TrimSpace(prompt))
	if len(fields) == 0 {
		return "", nil, false
	}

	command := strings.TrimSpace(fields[0])
	if !strings.HasPrefix(command, "/") {
		return "", nil, false
	}

	command = strings.TrimPrefix(command, "/")
	if idx := strings.Index(command, "@"); idx >= 0 {
		command = command[:idx]
	}
	command = strings.ToLower(strings.TrimSpace(command))
	if command == "" {
		return "", nil, false
	}
	return command, fields[1:], true
}

func (o *Orchestrator) helpText() string {
	return strings.Join([]string{
		"CONTROL admin commands:",
		"/status - runtime health, current chat, and audit summary",
		"/session [chat_id] - inspect one persisted chat/session binding",
		"/sessions - list persisted chat/session bindings",
		"/audit [count] - show recent operational audit events",
		"/reset - start a fresh Copilot session for this chat on the next prompt",
		"/help - show this command list",
	}, "\n")
}

func (o *Orchestrator) statusText(ctx context.Context, message Message, existing store.SessionBinding, found bool) (string, error) {
	bindings, err := o.store.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list persisted sessions: %w", err)
	}

	var auditEvents int
	if o.auditStore != nil {
		events, err := o.auditStore.Load(ctx)
		if err != nil {
			return "", fmt.Errorf("load audit events: %w", err)
		}
		auditEvents = len(events)
	}

	lines := []string{
		"CONTROL status:",
		fmt.Sprintf("- persisted sessions: %d", len(bindings)),
		fmt.Sprintf("- audit events: %d", auditEvents),
	}
	if o.runtimeStatus != nil {
		snapshot := o.runtimeStatus.Snapshot()
		runtimeLine := "stopped"
		if snapshot.Running {
			runtimeLine = "running"
			if !snapshot.StartedAt.IsZero() {
				runtimeLine += " since " + snapshot.StartedAt.UTC().Format(time.RFC3339)
			}
		}
		if snapshot.LastError != "" {
			runtimeLine += " (last error: " + snapshot.LastError + ")"
		}
		lines = append(lines, "- runtime: "+runtimeLine)
		if snapshot.LastEventKind != "" {
			lastEvent := snapshot.LastEventKind
			if !snapshot.LastEventAt.IsZero() {
				lastEvent += " at " + snapshot.LastEventAt.UTC().Format(time.RFC3339)
			}
			lines = append(lines, "- last event: "+lastEvent)
		}
		lines = append(lines, fmt.Sprintf("- observed permission requests: %d", snapshot.PermissionRequests))
		lines = append(lines, fmt.Sprintf("- observed tool calls: %d", snapshot.ToolCalls))
	}

	current := existing
	currentFound := found
	if !currentFound {
		var lookupErr error
		current, currentFound, lookupErr = o.store.Get(ctx, message.Transport, message.ChatID)
		if lookupErr != nil {
			return "", fmt.Errorf("load current chat binding: %w", lookupErr)
		}
	}
	if currentFound {
		lines = append(lines, "- current chat: "+formatBindingSummary(current))
	} else {
		lines = append(lines, fmt.Sprintf("- current chat: chat=%d has no persisted session yet", message.ChatID))
	}

	return strings.Join(lines, "\n"), nil
}

func (o *Orchestrator) sessionText(ctx context.Context, message Message, existing store.SessionBinding, found bool, args []string) (string, error) {
	chatID := message.ChatID
	if len(args) > 0 {
		parsed, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return "", fmt.Errorf("parse chat id %q: %w", args[0], err)
		}
		chatID = parsed
	}

	binding := existing
	currentFound := found && chatID == message.ChatID
	if !currentFound {
		var err error
		binding, currentFound, err = o.store.Get(ctx, message.Transport, chatID)
		if err != nil {
			return "", fmt.Errorf("load session binding for chat %d: %w", chatID, err)
		}
	}
	if !currentFound {
		return fmt.Sprintf("No persisted session binding for transport=%s chat=%d.", message.Transport, chatID), nil
	}

	lines := []string{
		fmt.Sprintf("Session binding for transport=%s chat=%d:", binding.Transport, binding.ChatID),
		fmt.Sprintf("- user: %d", binding.UserID),
		fmt.Sprintf("- session: %s", formatSessionID(binding.SessionID)),
		fmt.Sprintf("- generation: %d", binding.Generation),
		fmt.Sprintf("- created: %s", binding.CreatedAt.UTC().Format(time.RFC3339)),
		fmt.Sprintf("- updated: %s", binding.UpdatedAt.UTC().Format(time.RFC3339)),
	}
	if binding.Metadata.ChatTitle != "" {
		lines = append(lines, "- chat title: "+binding.Metadata.ChatTitle)
	}
	if binding.Metadata.Username != "" {
		lines = append(lines, "- username: "+binding.Metadata.Username)
	}
	if binding.Metadata.FirstName != "" || binding.Metadata.LastName != "" {
		lines = append(lines, "- name: "+strings.TrimSpace(binding.Metadata.FirstName+" "+binding.Metadata.LastName))
	}
	if binding.Metadata.LanguageCode != "" {
		lines = append(lines, "- language: "+binding.Metadata.LanguageCode)
	}
	return strings.Join(lines, "\n"), nil
}

func (o *Orchestrator) sessionsText(ctx context.Context) (string, error) {
	bindings, err := o.store.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list persisted sessions: %w", err)
	}
	if len(bindings) == 0 {
		return "No persisted chat/session bindings.", nil
	}

	lines := []string{"Persisted chat/session bindings:"}
	for _, binding := range bindings {
		lines = append(lines, "- "+formatBindingSummary(binding))
	}
	return strings.Join(lines, "\n"), nil
}

func (o *Orchestrator) auditText(ctx context.Context, args []string) (string, error) {
	if o.auditStore == nil {
		return "Operational audit log is not configured.", nil
	}

	limit := defaultAuditEventLimit
	if len(args) > 0 {
		parsed, err := strconv.Atoi(args[0])
		if err != nil {
			return "", fmt.Errorf("parse audit count %q: %w", args[0], err)
		}
		if parsed > 0 {
			limit = parsed
		}
	}

	events, err := o.auditStore.Load(ctx)
	if err != nil {
		return "", fmt.Errorf("load audit events: %w", err)
	}
	if len(events) == 0 {
		return "Operational audit log is empty.", nil
	}

	sort.SliceStable(events, func(i, j int) bool {
		return events[i].OccurredAt.Before(events[j].OccurredAt)
	})
	if limit > len(events) {
		limit = len(events)
	}

	lines := []string{fmt.Sprintf("Latest %d operational audit event(s):", limit)}
	for i := len(events) - 1; i >= len(events)-limit; i-- {
		lines = append(lines, "- "+formatAuditEvent(events[i]))
	}
	return strings.Join(lines, "\n"), nil
}

func (o *Orchestrator) resetText(ctx context.Context, message Message, existing store.SessionBinding, found bool) (string, error) {
	resetter, ok := o.store.(store.ChatSessionResetter)
	if !ok {
		return "Session reset is not available for the configured store.", nil
	}

	binding := store.SessionBinding{
		Transport:  message.Transport,
		ChatID:     message.ChatID,
		UserID:     message.UserID,
		Generation: bindingGeneration(existing, found),
		Metadata:   mergeMetadata(existing.Metadata, message.Metadata),
	}
	resetBinding, err := resetter.Reset(ctx, binding)
	if err != nil {
		return "", fmt.Errorf("reset chat %d session: %w", message.ChatID, err)
	}

	return fmt.Sprintf(
		"Reset chat %d. Session generation is now %d. Send the next non-command message to start a fresh Copilot session.",
		resetBinding.ChatID,
		resetBinding.Generation,
	), nil
}

func formatBindingSummary(binding store.SessionBinding) string {
	return fmt.Sprintf(
		"transport=%s chat=%d user=%d session=%s generation=%d updated=%s",
		binding.Transport,
		binding.ChatID,
		binding.UserID,
		formatSessionID(binding.SessionID),
		binding.Generation,
		binding.UpdatedAt.UTC().Format(time.RFC3339),
	)
}

func formatSessionID(sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		return "(pending new session)"
	}
	return sessionID
}

func formatAuditEvent(event store.PrivilegedToolEvent) string {
	parts := []string{event.OccurredAt.UTC().Format(time.RFC3339), event.EventType}
	if event.ToolName != "" {
		parts = append(parts, "tool="+event.ToolName)
	}
	if event.Outcome != "" {
		parts = append(parts, "outcome="+event.Outcome)
	}
	if event.ChatID != 0 {
		parts = append(parts, "chat="+strconv.FormatInt(event.ChatID, 10))
	}
	if event.SessionID != "" {
		parts = append(parts, "session="+event.SessionID)
	}
	if event.Summary != "" {
		parts = append(parts, "summary="+event.Summary)
	}
	return strings.Join(parts, " | ")
}
