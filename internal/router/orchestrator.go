package router

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	sdk "github.com/github/copilot-sdk/go"

	"github.com/SCWPretorius/CONTROL/internal/copilot"
	"github.com/SCWPretorius/CONTROL/internal/store"
)

const defaultReplyTimeout = 90 * time.Second

var ErrEmptyAssistantReply = errors.New("router: assistant reply was empty")

// Session is the minimal Copilot session surface the orchestration layer needs.
type Session interface {
	ID() string
	SendAndWait(context.Context, sdk.MessageOptions) (*sdk.SessionEvent, error)
}

// SessionResolver resolves or creates the Copilot session for an external chat key.
type SessionResolver interface {
	EnsureSession(context.Context, copilot.ExternalSessionKey) (Session, error)
}

// Orchestrator wires normalized inbound messages to Copilot sessions plus local persistence.
type Orchestrator struct {
	runtime       SessionResolver
	store         store.ChatSessionStore
	auditStore    store.PrivilegedToolEventStore
	runtimeStatus RuntimeStatusProvider
	replyTimeout  time.Duration
}

// Option customizes Orchestrator construction.
type Option func(*Orchestrator)

// WithReplyTimeout sets the maximum time to wait for a Copilot response when the
// caller did not already provide a deadline.
func WithReplyTimeout(timeout time.Duration) Option {
	return func(o *Orchestrator) {
		o.replyTimeout = timeout
	}
}

// WithPrivilegedToolEventStore enables admin audit inspection commands.
func WithPrivilegedToolEventStore(auditStore store.PrivilegedToolEventStore) Option {
	return func(o *Orchestrator) {
		o.auditStore = auditStore
	}
}

// WithRuntimeStatusProvider enables runtime health inspection commands.
func WithRuntimeStatusProvider(provider RuntimeStatusProvider) Option {
	return func(o *Orchestrator) {
		o.runtimeStatus = provider
	}
}

func NewOrchestrator(runtime SessionResolver, sessionStore store.ChatSessionStore, opts ...Option) (*Orchestrator, error) {
	if runtime == nil {
		return nil, errors.New("router: session resolver is required")
	}
	if sessionStore == nil {
		return nil, errors.New("router: chat session store is required")
	}

	orchestrator := &Orchestrator{
		runtime:      runtime,
		store:        sessionStore,
		replyTimeout: defaultReplyTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(orchestrator)
		}
	}

	return orchestrator, nil
}

// HandleMessage resolves the session for a normalized inbound message, persists
// the chat/session binding, sends the prompt to Copilot, and returns the
// assistant's final text reply.
func (o *Orchestrator) HandleMessage(ctx context.Context, message Message) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	prompt := strings.TrimSpace(message.Text)
	if prompt == "" {
		return "", errors.New("router: message text is required")
	}
	if message.ChatID == 0 {
		return "", errors.New("router: chat id is required")
	}
	if message.UserID == 0 {
		return "", errors.New("router: user id is required")
	}
	if strings.TrimSpace(message.Transport) == "" {
		return "", errors.New("router: transport is required")
	}

	existing, found, err := o.store.Get(ctx, message.Transport, message.ChatID)
	if err != nil {
		return "", fmt.Errorf("load session binding for transport=%s chat=%d: %w", message.Transport, message.ChatID, err)
	}
	if found && existing.UserID != 0 && existing.UserID != message.UserID {
		return "", fmt.Errorf("chat %d is already bound to user %d", message.ChatID, existing.UserID)
	}
	if handled, reply, err := o.handleAdminCommand(ctx, prompt, message, existing, found); handled || err != nil {
		return reply, err
	}

	session, err := o.runtime.EnsureSession(ctx, externalSessionKey(message, bindingGeneration(existing, found)))
	if err != nil {
		return "", fmt.Errorf("ensure copilot session for chat %d: %w", message.ChatID, err)
	}

	binding := mergeBinding(existing, found, message, session.ID())
	if err := o.store.Put(ctx, binding); err != nil {
		return "", fmt.Errorf("persist session binding for chat %d: %w", message.ChatID, err)
	}

	replyCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && o.replyTimeout > 0 {
		var cancel context.CancelFunc
		replyCtx, cancel = context.WithTimeout(ctx, o.replyTimeout)
		defer cancel()
	}

	event, err := session.SendAndWait(replyCtx, sdk.MessageOptions{Prompt: prompt})
	if err != nil {
		return "", fmt.Errorf("send message to copilot session %q: %w", session.ID(), err)
	}

	reply := extractAssistantReply(event)
	if reply == "" {
		return "", ErrEmptyAssistantReply
	}

	return reply, nil
}

func externalSessionKey(message Message, generation int) copilot.ExternalSessionKey {
	identifiers := map[string]string{
		"transport": strings.TrimSpace(message.Transport),
		"chat_id":   strconv.FormatInt(message.ChatID, 10),
		"user_id":   strconv.FormatInt(message.UserID, 10),
	}
	if generation > 0 {
		identifiers["generation"] = strconv.Itoa(generation)
	}
	return copilot.ExternalSessionKey{Identifiers: identifiers}
}

func mergeBinding(existing store.SessionBinding, found bool, message Message, sessionID string) store.SessionBinding {
	now := time.Now().UTC()
	binding := store.SessionBinding{
		Transport:  message.Transport,
		ChatID:     message.ChatID,
		UserID:     message.UserID,
		SessionID:  sessionID,
		Generation: existing.Generation,
		Metadata:   mergeMetadata(existing.Metadata, message.Metadata),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if found {
		binding.CreatedAt = existing.CreatedAt
	}
	return binding
}

func bindingGeneration(existing store.SessionBinding, found bool) int {
	if !found || existing.Generation < 0 {
		return 0
	}
	return existing.Generation
}

func mergeMetadata(existing store.TelegramChatMetadata, message MessageMetadata) store.TelegramChatMetadata {
	metadata := store.TelegramChatMetadata{
		ChatTitle:    existing.ChatTitle,
		Username:     existing.Username,
		FirstName:    existing.FirstName,
		LastName:     existing.LastName,
		LanguageCode: existing.LanguageCode,
	}

	if value := strings.TrimSpace(message.ChatTitle); value != "" {
		metadata.ChatTitle = value
	}
	if value := strings.TrimSpace(message.Username); value != "" {
		metadata.Username = value
	}
	if value := strings.TrimSpace(message.FirstName); value != "" {
		metadata.FirstName = value
	}
	if value := strings.TrimSpace(message.LastName); value != "" {
		metadata.LastName = value
	}
	if value := strings.TrimSpace(message.LanguageCode); value != "" {
		metadata.LanguageCode = value
	}

	return metadata
}

func extractAssistantReply(event *sdk.SessionEvent) string {
	if event == nil {
		return ""
	}
	for _, value := range []*string{
		event.Data.Content,
		event.Data.SummaryContent,
		event.Data.Message,
	} {
		if value != nil {
			if text := strings.TrimSpace(*value); text != "" {
				return text
			}
		}
	}
	return ""
}
