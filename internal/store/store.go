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

// MonitorEvent is an append-only monitor audit record for incidents, actions, and
// outcomes that may later be replayed or linked to Copilot sessions.
type MonitorEvent struct {
	ID            string            `json:"id,omitempty"`
	CheckID       string            `json:"check_id"`
	IncidentID    string            `json:"incident_id,omitempty"`
	CorrelationID string            `json:"correlation_id,omitempty"`
	SessionID     string            `json:"session_id,omitempty"`
	EventType     string            `json:"event_type"`
	Condition     string            `json:"condition,omitempty"`
	Fingerprint   string            `json:"fingerprint,omitempty"`
	Outcome       string            `json:"outcome,omitempty"`
	Summary       string            `json:"summary,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	OccurredAt    time.Time         `json:"occurred_at"`
}

// MonitorCorrelation is an append-only relationship record that links monitor
// incidents and events to downstream actions, outcomes, and future Copilot
// sessions without rewriting historical records.
type MonitorCorrelation struct {
	ID            string            `json:"id,omitempty"`
	CheckID       string            `json:"check_id"`
	IncidentID    string            `json:"incident_id,omitempty"`
	CorrelationID string            `json:"correlation_id,omitempty"`
	SourceType    string            `json:"source_type"`
	SourceID      string            `json:"source_id"`
	TargetType    string            `json:"target_type"`
	TargetID      string            `json:"target_id"`
	Relationship  string            `json:"relationship"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	RecordedAt    time.Time         `json:"recorded_at"`
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

// MonitorEventStore stores append-only monitor event records for audit and replay.
type MonitorEventStore interface {
	AppendMonitorEvent(context.Context, MonitorEvent) error
	LoadMonitorEvents(context.Context) ([]MonitorEvent, error)
}

// MonitorCorrelationStore stores append-only relationships between monitor
// incidents, events, actions, and future Copilot sessions.
type MonitorCorrelationStore interface {
	AppendMonitorCorrelation(context.Context, MonitorCorrelation) error
	LoadMonitorCorrelations(context.Context) ([]MonitorCorrelation, error)
}
