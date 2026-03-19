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

	for _, dir := range []string{store.rootDir, store.sessionsDir, store.auditDir} {
		if info, err := os.Stat(dir); err != nil {
			t.Fatalf("Stat(%q) error = %v", dir, err)
		} else if !info.IsDir() {
			t.Fatalf("%q is not a directory", dir)
		}
	}
}
