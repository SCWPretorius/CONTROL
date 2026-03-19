package router

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	sdk "github.com/github/copilot-sdk/go"

	"github.com/SCWPretorius/CONTROL/internal/copilot"
	"github.com/SCWPretorius/CONTROL/internal/store"
)

func TestOrchestratorHandleMessageCreatesBindingAndReturnsReply(t *testing.T) {
	t.Parallel()

	session := &fakeSession{
		id: "session-123",
		event: &sdk.SessionEvent{
			Data: sdk.Data{
				Content: stringPtr(" Assistant reply "),
			},
		},
	}
	runtime := &fakeRuntime{session: session}
	sessionStore := &fakeChatSessionStore{}

	orchestrator, err := NewOrchestrator(runtime, sessionStore, WithReplyTimeout(time.Second))
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}

	reply, err := orchestrator.HandleMessage(context.Background(), Message{
		Transport: "telegram",
		MessageID: 9,
		ChatID:    77,
		UserID:    88,
		Text:      " hello ",
		Metadata: MessageMetadata{
			ChatTitle:    "CONTROL",
			Username:     "pretorius",
			FirstName:    "Pieter",
			LastName:     "Pretorius",
			LanguageCode: "en",
		},
	})
	if err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}
	if reply != "Assistant reply" {
		t.Fatalf("reply = %q, want %q", reply, "Assistant reply")
	}

	if runtime.key.Identifiers["chat_id"] != "77" || runtime.key.Identifiers["user_id"] != "88" {
		t.Fatalf("runtime key = %#v", runtime.key.Identifiers)
	}
	if got := runtime.key.Identifiers["transport"]; got != "telegram" {
		t.Fatalf("runtime key transport = %q, want %q", got, "telegram")
	}
	if got := session.prompt; got != "hello" {
		t.Fatalf("prompt = %q, want %q", got, "hello")
	}
	if sessionStore.puts != 1 {
		t.Fatalf("store.Put calls = %d, want 1", sessionStore.puts)
	}
	if got := sessionStore.binding.SessionID; got != "session-123" {
		t.Fatalf("binding.SessionID = %q, want %q", got, "session-123")
	}
	if got := sessionStore.binding.Transport; got != "telegram" {
		t.Fatalf("binding.Transport = %q, want %q", got, "telegram")
	}
	if got := sessionStore.binding.Metadata.Username; got != "pretorius" {
		t.Fatalf("binding.Metadata.Username = %q, want %q", got, "pretorius")
	}
	if sessionStore.binding.CreatedAt.IsZero() {
		t.Fatal("binding.CreatedAt was not set")
	}
	if sessionStore.binding.UpdatedAt.IsZero() {
		t.Fatal("binding.UpdatedAt was not set")
	}
}

func TestOrchestratorHandleMessagePreservesExistingBindingMetadata(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)
	sessionStore := &fakeChatSessionStore{
		binding: store.SessionBinding{
			Transport: "telegram",
			ChatID:    77,
			UserID:    88,
			SessionID: "old-session",
			Metadata: store.TelegramChatMetadata{
				ChatTitle: "CONTROL",
				Username:  "pretorius",
			},
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		},
		found: true,
	}
	orchestrator, err := NewOrchestrator(&fakeRuntime{
		session: &fakeSession{
			id: "new-session",
			event: &sdk.SessionEvent{
				Data: sdk.Data{Content: stringPtr("ok")},
			},
		},
	}, sessionStore)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}

	_, err = orchestrator.HandleMessage(context.Background(), Message{
		Transport: "telegram",
		ChatID:    77,
		UserID:    88,
		Text:      "hello",
		Metadata: MessageMetadata{
			FirstName: "Pieter",
		},
	})
	if err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}

	if got := sessionStore.binding.CreatedAt; !got.Equal(createdAt) {
		t.Fatalf("binding.CreatedAt = %v, want %v", got, createdAt)
	}
	if got := sessionStore.binding.Metadata.ChatTitle; got != "CONTROL" {
		t.Fatalf("binding.Metadata.ChatTitle = %q, want %q", got, "CONTROL")
	}
	if got := sessionStore.binding.Metadata.FirstName; got != "Pieter" {
		t.Fatalf("binding.Metadata.FirstName = %q, want %q", got, "Pieter")
	}
	if got := sessionStore.binding.SessionID; got != "new-session" {
		t.Fatalf("binding.SessionID = %q, want %q", got, "new-session")
	}
}

func TestOrchestratorHandleMessageReturnsErrorWhenAssistantReplyIsEmpty(t *testing.T) {
	t.Parallel()

	orchestrator, err := NewOrchestrator(&fakeRuntime{
		session: &fakeSession{
			id:    "session-123",
			event: &sdk.SessionEvent{},
		},
	}, &fakeChatSessionStore{})
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}

	_, err = orchestrator.HandleMessage(context.Background(), Message{
		Transport: "telegram",
		ChatID:    1,
		UserID:    2,
		Text:      "hello",
	})
	if !errors.Is(err, ErrEmptyAssistantReply) {
		t.Fatalf("HandleMessage() error = %v, want %v", err, ErrEmptyAssistantReply)
	}
}

func TestOrchestratorHandleMessageFailsWhenChatIsBoundToDifferentUser(t *testing.T) {
	t.Parallel()

	orchestrator, err := NewOrchestrator(&fakeRuntime{
		session: &fakeSession{
			id:    "session-123",
			event: &sdk.SessionEvent{Data: sdk.Data{Content: stringPtr("ok")}},
		},
	}, &fakeChatSessionStore{
		binding: store.SessionBinding{
			ChatID:    1,
			UserID:    999,
			SessionID: "session-999",
		},
		found: true,
	})
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}

	_, err = orchestrator.HandleMessage(context.Background(), Message{
		ChatID: 1,
		UserID: 2,
		Text:   "hello",
	})
	if err == nil {
		t.Fatal("HandleMessage() error = nil, want error")
	}
}

func TestOrchestratorHandleMessageReturnsStatusWithoutCallingRuntime(t *testing.T) {
	t.Parallel()

	sessionStore := &fakeChatSessionStore{
		bindings: []store.SessionBinding{
			{
				Transport:  "telegram",
				ChatID:     77,
				UserID:     88,
				SessionID:  "session-123",
				Generation: 1,
				UpdatedAt:  time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC),
			},
		},
		binding: store.SessionBinding{
			Transport:  "telegram",
			ChatID:     77,
			UserID:     88,
			SessionID:  "session-123",
			Generation: 1,
			UpdatedAt:  time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC),
		},
		found: true,
	}
	runtime := &fakeRuntime{
		session: &fakeSession{id: "session-123", event: &sdk.SessionEvent{Data: sdk.Data{Content: stringPtr("ok")}}},
	}
	auditStore := &fakeAuditStore{
		events: []store.PrivilegedToolEvent{{EventType: "runtime.started", ToolName: "runtime"}},
	}

	orchestrator, err := NewOrchestrator(
		runtime,
		sessionStore,
		WithPrivilegedToolEventStore(auditStore),
		WithRuntimeStatusProvider(fakeRuntimeStatusProvider{
			snapshot: RuntimeStatus{
				Running:            true,
				LastEventKind:      "runtime.started",
				LastEventAt:        time.Date(2025, time.January, 1, 11, 59, 0, 0, time.UTC),
				PermissionRequests: 2,
				ToolCalls:          3,
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}

	reply, err := orchestrator.HandleMessage(context.Background(), Message{
		Transport: "telegram",
		ChatID:    77,
		UserID:    88,
		Text:      "/status",
	})
	if err != nil {
		t.Fatalf("HandleMessage(/status) error = %v", err)
	}
	if runtime.ensureCalls != 0 {
		t.Fatalf("EnsureSession() calls = %d, want 0", runtime.ensureCalls)
	}
	for _, want := range []string{"persisted sessions: 1", "audit events: 1", "runtime: running", "current chat: transport=telegram chat=77"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply %q does not contain %q", reply, want)
		}
	}
}

func TestOrchestratorHandleMessageResetAdvancesGeneration(t *testing.T) {
	t.Parallel()

	sessionStore := &fakeChatSessionStore{
		binding: store.SessionBinding{
			Transport:  "telegram",
			ChatID:     77,
			UserID:     88,
			SessionID:  "session-123",
			Generation: 1,
			CreatedAt:  time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC),
			UpdatedAt:  time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC),
		},
		found: true,
	}
	runtime := &fakeRuntime{
		session: &fakeSession{id: "session-456", event: &sdk.SessionEvent{Data: sdk.Data{Content: stringPtr("ok")}}},
	}
	orchestrator, err := NewOrchestrator(runtime, sessionStore)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}

	reply, err := orchestrator.HandleMessage(context.Background(), Message{
		Transport: "telegram",
		ChatID:    77,
		UserID:    88,
		Text:      "/reset",
	})
	if err != nil {
		t.Fatalf("HandleMessage(/reset) error = %v", err)
	}
	if !strings.Contains(reply, "generation is now 2") {
		t.Fatalf("reply = %q, want generation confirmation", reply)
	}
	if sessionStore.binding.Generation != 2 || sessionStore.binding.SessionID != "" {
		t.Fatalf("reset binding = %#v", sessionStore.binding)
	}

	_, err = orchestrator.HandleMessage(context.Background(), Message{
		Transport: "telegram",
		ChatID:    77,
		UserID:    88,
		Text:      "hello again",
	})
	if err != nil {
		t.Fatalf("HandleMessage(post-reset) error = %v", err)
	}
	if got := runtime.key.Identifiers["generation"]; got != "2" {
		t.Fatalf("runtime generation key = %q, want %q", got, "2")
	}
	if got := sessionStore.binding.SessionID; got != "session-456" {
		t.Fatalf("binding.SessionID = %q, want %q", got, "session-456")
	}
}

func TestOrchestratorHandleMessageShowsAuditEntries(t *testing.T) {
	t.Parallel()

	orchestrator, err := NewOrchestrator(
		&fakeRuntime{},
		&fakeChatSessionStore{},
		WithPrivilegedToolEventStore(&fakeAuditStore{
			events: []store.PrivilegedToolEvent{
				{OccurredAt: time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC), EventType: "runtime.started", ToolName: "runtime"},
				{OccurredAt: time.Date(2025, time.January, 1, 12, 1, 0, 0, time.UTC), EventType: "hook.pre_tool_use", ToolName: "shell", Outcome: "started", ChatID: 77},
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}

	reply, err := orchestrator.HandleMessage(context.Background(), Message{
		Transport: "telegram",
		ChatID:    77,
		UserID:    88,
		Text:      "/audit 1",
	})
	if err != nil {
		t.Fatalf("HandleMessage(/audit) error = %v", err)
	}
	if strings.Count(reply, "\n- ") != 1 {
		t.Fatalf("reply = %q, want exactly one audit event", reply)
	}
	if !strings.Contains(reply, "hook.pre_tool_use") {
		t.Fatalf("reply = %q, want latest audit event", reply)
	}
}

type fakeRuntime struct {
	key         copilot.ExternalSessionKey
	session     Session
	err         error
	ensureCalls int
}

func (f *fakeRuntime) EnsureSession(_ context.Context, key copilot.ExternalSessionKey) (Session, error) {
	f.ensureCalls++
	f.key = key
	if f.err != nil {
		return nil, f.err
	}
	return f.session, nil
}

type fakeSession struct {
	id     string
	prompt string
	event  *sdk.SessionEvent
	err    error
}

func (f *fakeSession) ID() string {
	return f.id
}

func (f *fakeSession) SendAndWait(_ context.Context, options sdk.MessageOptions) (*sdk.SessionEvent, error) {
	f.prompt = options.Prompt
	if f.err != nil {
		return nil, f.err
	}
	return f.event, nil
}

type fakeChatSessionStore struct {
	binding  store.SessionBinding
	bindings []store.SessionBinding
	found    bool
	puts     int
	getErr   error
	putErr   error
	resetErr error
}

func (f *fakeChatSessionStore) Get(context.Context, string, int64) (store.SessionBinding, bool, error) {
	if f.getErr != nil {
		return store.SessionBinding{}, false, f.getErr
	}
	return f.binding, f.found, nil
}

func (f *fakeChatSessionStore) Put(_ context.Context, binding store.SessionBinding) error {
	if f.putErr != nil {
		return f.putErr
	}
	f.binding = binding
	f.found = true
	f.puts++
	return nil
}

func (f *fakeChatSessionStore) List(context.Context) ([]store.SessionBinding, error) {
	return append([]store.SessionBinding(nil), f.bindings...), nil
}

func (f *fakeChatSessionStore) Reset(_ context.Context, binding store.SessionBinding) (store.SessionBinding, error) {
	if f.resetErr != nil {
		return store.SessionBinding{}, f.resetErr
	}
	binding.Generation++
	binding.SessionID = ""
	binding.CreatedAt = time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)
	binding.UpdatedAt = time.Date(2025, time.January, 1, 12, 1, 0, 0, time.UTC)
	f.binding = binding
	f.bindings = []store.SessionBinding{binding}
	f.found = true
	return binding, nil
}

func stringPtr(value string) *string {
	return &value
}

type fakeAuditStore struct {
	events []store.PrivilegedToolEvent
}

func (f *fakeAuditStore) Append(_ context.Context, event store.PrivilegedToolEvent) error {
	f.events = append(f.events, event)
	return nil
}

func (f *fakeAuditStore) Load(context.Context) ([]store.PrivilegedToolEvent, error) {
	return append([]store.PrivilegedToolEvent(nil), f.events...), nil
}

type fakeRuntimeStatusProvider struct {
	snapshot RuntimeStatus
}

func (f fakeRuntimeStatusProvider) Snapshot() RuntimeStatus {
	return f.snapshot
}
