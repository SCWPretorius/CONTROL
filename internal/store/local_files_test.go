package store

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLocalFileStoreSessionRoundTrip(t *testing.T) {
	t.Parallel()

	store, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileStore() error = %v", err)
	}

	want := SessionBinding{
		Transport: "telegram",
		ChatID:    42,
		UserID:    7,
		SessionID: "copilot-session-123",
		Metadata: TelegramChatMetadata{
			ChatTitle:    "CONTROL",
			Username:     "pretorius",
			FirstName:    "Pieter",
			LanguageCode: "en",
		},
	}

	if err := store.Put(context.Background(), want); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	got, ok, err := store.Get(context.Background(), want.Transport, want.ChatID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() did not find stored binding")
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps to be set, got created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}

	got.CreatedAt = time.Time{}
	got.UpdatedAt = time.Time{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Get() mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestLocalFileStoreListLoadsExistingSessionBindings(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewLocalFileStore(root)
	if err != nil {
		t.Fatalf("NewLocalFileStore() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(store.sessionsDir, "chat-telegram-2.json"), []byte(`{"transport":"telegram","chat_id":2,"user_id":20,"session_id":"two","created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(chat-telegram-2.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.sessionsDir, "chat-telegram-1.json"), []byte(`{"transport":"telegram","chat_id":1,"user_id":10,"session_id":"one","created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(chat-telegram-1.json) error = %v", err)
	}

	bindings, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(bindings) != 2 {
		t.Fatalf("List() len = %d, want 2", len(bindings))
	}
	if bindings[0].ChatID != 1 || bindings[1].ChatID != 2 {
		t.Fatalf("List() chat ids = [%d %d], want [1 2]", bindings[0].ChatID, bindings[1].ChatID)
	}
}

func TestLocalFileStorePrivilegedToolEventRoundTrip(t *testing.T) {
	t.Parallel()

	store, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileStore() error = %v", err)
	}

	first := PrivilegedToolEvent{
		ID:         "evt-1",
		ChatID:     42,
		UserID:     7,
		SessionID:  "copilot-session-123",
		ToolName:   "shell",
		EventType:  "approval",
		Outcome:    "approved",
		Summary:    "operator approved shell command",
		OccurredAt: time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC),
		Metadata: map[string]string{
			"command": "go test ./...",
		},
	}
	second := PrivilegedToolEvent{
		ID:         "evt-2",
		ToolName:   "shell",
		EventType:  "execution",
		Outcome:    "completed",
		OccurredAt: time.Date(2025, time.January, 1, 12, 1, 0, 0, time.UTC),
	}

	if err := store.Append(context.Background(), first); err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	if err := store.Append(context.Background(), second); err != nil {
		t.Fatalf("Append(second) error = %v", err)
	}

	events, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("Load() len = %d, want 2", len(events))
	}
	if !reflect.DeepEqual(events[0], first) {
		t.Fatalf("first event mismatch\nwant: %#v\ngot:  %#v", first, events[0])
	}
	if !reflect.DeepEqual(events[1], second) {
		t.Fatalf("second event mismatch\nwant: %#v\ngot:  %#v", second, events[1])
	}
}

func TestLocalFileStoreMonitorEventRoundTrip(t *testing.T) {
	t.Parallel()

	store, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileStore() error = %v", err)
	}

	first := MonitorEvent{
		ID:            "monitor-event-1",
		CheckID:       "api-health",
		IncidentID:    "incident-1",
		CorrelationID: "corr-1",
		EventType:     "incident_observed",
		Condition:     "status:500",
		Fingerprint:   "status:500",
		Summary:       "service returned 500",
		OccurredAt:    time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC),
		Metadata: map[string]string{
			"runner_mode": "notify-only",
		},
	}
	second := MonitorEvent{
		ID:            "monitor-event-2",
		CheckID:       "api-health",
		IncidentID:    "incident-1",
		CorrelationID: "corr-1",
		EventType:     "action_outcome",
		Outcome:       "alert_sent",
		SessionID:     "copilot-session-123",
		OccurredAt:    time.Date(2025, time.January, 1, 12, 1, 0, 0, time.UTC),
	}

	if err := store.AppendMonitorEvent(context.Background(), first); err != nil {
		t.Fatalf("AppendMonitorEvent(first) error = %v", err)
	}
	if err := store.AppendMonitorEvent(context.Background(), second); err != nil {
		t.Fatalf("AppendMonitorEvent(second) error = %v", err)
	}

	events, err := store.LoadMonitorEvents(context.Background())
	if err != nil {
		t.Fatalf("LoadMonitorEvents() error = %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("LoadMonitorEvents() len = %d, want 2", len(events))
	}
	if !reflect.DeepEqual(events[0], first) {
		t.Fatalf("first monitor event mismatch\nwant: %#v\ngot:  %#v", first, events[0])
	}
	if !reflect.DeepEqual(events[1], second) {
		t.Fatalf("second monitor event mismatch\nwant: %#v\ngot:  %#v", second, events[1])
	}
}

func TestLocalFileStoreMonitorEventAssignsDefaults(t *testing.T) {
	t.Parallel()

	store, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileStore() error = %v", err)
	}

	event := MonitorEvent{
		CheckID:   "api-health",
		EventType: "incident_observed",
	}
	if err := store.AppendMonitorEvent(context.Background(), event); err != nil {
		t.Fatalf("AppendMonitorEvent() error = %v", err)
	}

	events, err := store.LoadMonitorEvents(context.Background())
	if err != nil {
		t.Fatalf("LoadMonitorEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("LoadMonitorEvents() len = %d, want 1", len(events))
	}
	if events[0].ID == "" {
		t.Fatal("expected monitor event ID to be generated")
	}
	if events[0].OccurredAt.IsZero() {
		t.Fatal("expected monitor event OccurredAt to be set")
	}
}

func TestLocalFileStoreMonitorCorrelationRoundTrip(t *testing.T) {
	t.Parallel()

	store, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileStore() error = %v", err)
	}

	first := MonitorCorrelation{
		ID:            "monitor-correlation-1",
		CheckID:       "api-health",
		IncidentID:    "incident-1",
		CorrelationID: "corr-1",
		SourceType:    "monitor_incident",
		SourceID:      "incident-1",
		TargetType:    "monitor_event",
		TargetID:      "monitor-event-1",
		Relationship:  "observed_as",
		RecordedAt:    time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC),
	}
	second := MonitorCorrelation{
		ID:            "monitor-correlation-2",
		CheckID:       "api-health",
		IncidentID:    "incident-1",
		CorrelationID: "corr-1",
		SourceType:    "monitor_event",
		SourceID:      "monitor-event-1",
		TargetType:    "copilot_session",
		TargetID:      "copilot-session-123",
		Relationship:  "analyzed_by",
		RecordedAt:    time.Date(2025, time.January, 1, 12, 1, 0, 0, time.UTC),
		Metadata: map[string]string{
			"phase": "future",
		},
	}

	if err := store.AppendMonitorCorrelation(context.Background(), first); err != nil {
		t.Fatalf("AppendMonitorCorrelation(first) error = %v", err)
	}
	if err := store.AppendMonitorCorrelation(context.Background(), second); err != nil {
		t.Fatalf("AppendMonitorCorrelation(second) error = %v", err)
	}

	correlations, err := store.LoadMonitorCorrelations(context.Background())
	if err != nil {
		t.Fatalf("LoadMonitorCorrelations() error = %v", err)
	}

	if len(correlations) != 2 {
		t.Fatalf("LoadMonitorCorrelations() len = %d, want 2", len(correlations))
	}
	if !reflect.DeepEqual(correlations[0], first) {
		t.Fatalf("first monitor correlation mismatch\nwant: %#v\ngot:  %#v", first, correlations[0])
	}
	if !reflect.DeepEqual(correlations[1], second) {
		t.Fatalf("second monitor correlation mismatch\nwant: %#v\ngot:  %#v", second, correlations[1])
	}
}

func TestLocalFileStoreRejectsInvalidMonitorCorrelation(t *testing.T) {
	t.Parallel()

	store, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileStore() error = %v", err)
	}

	err = store.AppendMonitorCorrelation(context.Background(), MonitorCorrelation{
		CheckID:      "api-health",
		SourceType:   "monitor_incident",
		SourceID:     "incident-1",
		TargetType:   "monitor_event",
		TargetID:     "event-1",
		Relationship: "",
	})
	if err == nil {
		t.Fatal("AppendMonitorCorrelation() error = nil, want validation error")
	}
}

func TestLocalFileStoreMonitorCheckpointRoundTrip(t *testing.T) {
	t.Parallel()

	store, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileStore() error = %v", err)
	}

	want := MonitorCheckpoint{
		CheckID:           "api/health",
		LastSeenCondition: "unhealthy",
		LastAlertAt:       time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC),
		CooldownUntil:     time.Date(2025, time.January, 1, 12, 15, 0, 0, time.UTC),
		Fingerprint:       "fingerprint-123",
		Metadata: map[string]string{
			"cursor": "msg-123",
		},
	}

	if err := store.PutMonitorCheckpoint(context.Background(), want); err != nil {
		t.Fatalf("PutMonitorCheckpoint() error = %v", err)
	}

	got, ok, err := store.GetMonitorCheckpoint(context.Background(), want.CheckID)
	if err != nil {
		t.Fatalf("GetMonitorCheckpoint() error = %v", err)
	}
	if !ok {
		t.Fatal("GetMonitorCheckpoint() did not find stored checkpoint")
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt to be set")
	}

	got.UpdatedAt = time.Time{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetMonitorCheckpoint() mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestLocalFileStoreListMonitorCheckpointsLoadsExistingState(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewLocalFileStore(root)
	if err != nil {
		t.Fatalf("NewLocalFileStore() error = %v", err)
	}

	firstPath := filepath.Join(store.monitorStateDir, "check-api-one.json")
	secondPath := filepath.Join(store.monitorStateDir, "check-api-two.json")
	if err := os.WriteFile(secondPath, []byte(`{"check_id":"api-two","last_seen_condition":"healthy","updated_at":"2025-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", secondPath, err)
	}
	if err := os.WriteFile(firstPath, []byte(`{"check_id":"api-one","last_seen_condition":"unhealthy","updated_at":"2025-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", firstPath, err)
	}

	checkpoints, err := store.ListMonitorCheckpoints(context.Background())
	if err != nil {
		t.Fatalf("ListMonitorCheckpoints() error = %v", err)
	}

	if len(checkpoints) != 2 {
		t.Fatalf("ListMonitorCheckpoints() len = %d, want 2", len(checkpoints))
	}
	if checkpoints[0].CheckID != "api-one" || checkpoints[1].CheckID != "api-two" {
		t.Fatalf("ListMonitorCheckpoints() ids = [%s %s], want [api-one api-two]", checkpoints[0].CheckID, checkpoints[1].CheckID)
	}
}

func TestLocalFileStoreRejectsInvalidMonitorCheckpoint(t *testing.T) {
	t.Parallel()

	store, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileStore() error = %v", err)
	}

	err = store.PutMonitorCheckpoint(context.Background(), MonitorCheckpoint{
		CheckID:           "api-health",
		LastSeenCondition: "",
	})
	if err == nil {
		t.Fatal("PutMonitorCheckpoint() error = nil, want validation error")
	}
}

func TestLocalFileStoreResetAdvancesGenerationAndClearsSessionID(t *testing.T) {
	t.Parallel()

	store, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileStore() error = %v", err)
	}

	original := SessionBinding{
		Transport:  "telegram",
		ChatID:     42,
		UserID:     7,
		SessionID:  "copilot-session-123",
		Generation: 2,
		Metadata: TelegramChatMetadata{
			Username: "pretorius",
		},
	}
	if err := store.Put(context.Background(), original); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	resetBinding, err := store.Reset(context.Background(), SessionBinding{
		Transport: "telegram",
		ChatID:    42,
		UserID:    7,
	})
	if err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	if got := resetBinding.Generation; got != 3 {
		t.Fatalf("Reset() generation = %d, want 3", got)
	}
	if resetBinding.SessionID != "" {
		t.Fatalf("Reset() session id = %q, want empty", resetBinding.SessionID)
	}

	got, ok, err := store.Get(context.Background(), "telegram", 42)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() after reset did not find binding")
	}
	if got.Generation != 3 {
		t.Fatalf("stored generation = %d, want 3", got.Generation)
	}
	if got.SessionID != "" {
		t.Fatalf("stored session id = %q, want empty", got.SessionID)
	}
	if got.Metadata.Username != "pretorius" {
		t.Fatalf("stored username = %q, want %q", got.Metadata.Username, "pretorius")
	}
}

func TestNewLocalFileStoreNormalizesPathsAndCreatesLayout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	input := filepath.Join(root, ".", "nested", "..", "storage")

	store, err := NewLocalFileStore(input)
	if err != nil {
		t.Fatalf("NewLocalFileStore() error = %v", err)
	}

	wantRoot, err := filepath.Abs(filepath.Join(root, "storage"))
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}

	if store.rootDir != filepath.Clean(wantRoot) {
		t.Fatalf("rootDir = %q, want %q", store.rootDir, filepath.Clean(wantRoot))
	}
	if !strings.HasPrefix(store.sessionPath("telegram", 99), store.sessionsDir) {
		t.Fatalf("sessionPath(telegram, 99) = %q, want prefix %q", store.sessionPath("telegram", 99), store.sessionsDir)
	}
	if got := filepath.Base(store.privilegedToolLogPath()); got != privilegedToolLogName {
		t.Fatalf("privilegedToolLogPath() base = %q, want %q", got, privilegedToolLogName)
	}
	if got := filepath.Base(store.monitorEventLogPath()); got != monitorEventLogName {
		t.Fatalf("monitorEventLogPath() base = %q, want %q", got, monitorEventLogName)
	}
	if got := filepath.Base(store.monitorCorrelationLogPath()); got != monitorCorrelationName {
		t.Fatalf("monitorCorrelationLogPath() base = %q, want %q", got, monitorCorrelationName)
	}
	if got := filepath.Base(store.monitorCheckpointPath("api/health")); got != "check-YXBpL2hlYWx0aA.json" {
		t.Fatalf("monitorCheckpointPath(api/health) base = %q, want %q", got, "check-YXBpL2hlYWx0aA.json")
	}

	for _, dir := range []string{store.rootDir, store.sessionsDir, store.auditDir, store.monitorStateDir} {
		if info, err := os.Stat(dir); err != nil {
			t.Fatalf("Stat(%q) error = %v", dir, err)
		} else if !info.IsDir() {
			t.Fatalf("%q is not a directory", dir)
		}
	}
}
