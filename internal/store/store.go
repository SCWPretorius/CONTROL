package store

import (
	"context"
	"time"
)

// SessionBinding maps a Telegram conversation to a Copilot session identifier.
type SessionBinding struct {
	Transport  string               `json:"transport,omitempty"`
	ChatID     int64                `json:"chat_id"`
	UserID     int64                `json:"user_id"`
	SessionID  string               `json:"session_id"`
	Generation int                  `json:"generation,omitempty"`
	Metadata   TelegramChatMetadata `json:"metadata,omitempty"`
	CreatedAt  time.Time            `json:"created_at"`
	UpdatedAt  time.Time            `json:"updated_at"`
}

// TelegramChatMetadata stores lightweight Telegram-specific metadata that helps
// the assistant resume and inspect sessions without depending on Copilot state.
type TelegramChatMetadata struct {
	ChatTitle    string `json:"chat_title,omitempty"`
	Username     string `json:"username,omitempty"`
	FirstName    string `json:"first_name,omitempty"`
	LastName     string `json:"last_name,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

// PrivilegedToolEvent is an append-only record for future privileged tool usage.
type PrivilegedToolEvent struct {
	ID         string            `json:"id,omitempty"`
	ChatID     int64             `json:"chat_id,omitempty"`
	UserID     int64             `json:"user_id,omitempty"`
	SessionID  string            `json:"session_id,omitempty"`
	ToolName   string            `json:"tool_name"`
	EventType  string            `json:"event_type"`
	Outcome    string            `json:"outcome,omitempty"`
	Summary    string            `json:"summary,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	OccurredAt time.Time         `json:"occurred_at"`
}

// MonitorCheckpoint stores app-owned state for monitor dedupe and cooldown logic.
type MonitorCheckpoint struct {
	CheckID           string    `json:"check_id"`
	LastSeenCondition string    `json:"last_seen_condition"`
	LastAlertAt       time.Time `json:"last_alert_at,omitempty"`
	CooldownUntil     time.Time `json:"cooldown_until,omitempty"`
	Fingerprint       string    `json:"fingerprint,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// ChatSessionStore is the persistence boundary for app-owned chat/session metadata.
type ChatSessionStore interface {
	Get(context.Context, string, int64) (SessionBinding, bool, error)
	Put(context.Context, SessionBinding) error
	List(context.Context) ([]SessionBinding, error)
}

// ChatSessionResetter increments the app-owned session generation for a chat so
// the next prompt starts a fresh Copilot session without changing transport IDs.
type ChatSessionResetter interface {
	Reset(context.Context, SessionBinding) (SessionBinding, error)
}

// PrivilegedToolEventStore stores append-only audit records for privileged tools.
type PrivilegedToolEventStore interface {
	Append(context.Context, PrivilegedToolEvent) error
	Load(context.Context) ([]PrivilegedToolEvent, error)
}

// MonitorCheckpointStore persists per-check monitor state for future runners.
type MonitorCheckpointStore interface {
	GetMonitorCheckpoint(context.Context, string) (MonitorCheckpoint, bool, error)
	PutMonitorCheckpoint(context.Context, MonitorCheckpoint) error
	ListMonitorCheckpoints(context.Context) ([]MonitorCheckpoint, error)
}
