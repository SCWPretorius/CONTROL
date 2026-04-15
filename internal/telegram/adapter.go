package telegram

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/SCWPretorius/CONTROL/internal/router"
	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const TransportName = "telegram"

var (
	ErrAccessDenied    = errors.New("telegram: access denied")
	ErrAlertChatUnset  = errors.New("telegram: alert chat is not configured")
	ErrEmptyMessage    = errors.New("telegram: empty message")
	ErrMissingSender   = errors.New("telegram: message sender is required")
	ErrNilUpdate       = errors.New("telegram: update is required")
	ErrUnsupportedType = errors.New("telegram: unsupported update type")
)

type MessageHandler func(context.Context, router.Message) error

type BotClient interface {
	Start(context.Context)
	SendMessage(context.Context, *tgbot.SendMessageParams) (*models.Message, error)
	SendChatAction(context.Context, *tgbot.SendChatActionParams) (bool, error)
}

type botFactory func(string, ...tgbot.Option) (BotClient, error)

type options struct {
	botFactory    botFactory
	botOptions    []tgbot.Option
	errorHandler  func(error)
	initialOffset int64
	pollTimeout   time.Duration
}

type Option func(*options)

func WithBotFactory(factory botFactory) Option {
	return func(opts *options) {
		if factory != nil {
			opts.botFactory = factory
		}
	}
}

func WithBotOptions(botOptions ...tgbot.Option) Option {
	return func(opts *options) {
		opts.botOptions = append(opts.botOptions, botOptions...)
	}
}

func WithErrorHandler(handler func(error)) Option {
	return func(opts *options) {
		opts.errorHandler = handler
	}
}

func WithInitialOffset(offset int64) Option {
	return func(opts *options) {
		opts.initialOffset = offset
	}
}

func WithPollTimeout(timeout time.Duration) Option {
	return func(opts *options) {
		opts.pollTimeout = timeout
	}
}

// Adapter owns Telegram long polling plus inbound/outbound message conversion.
type Adapter struct {
	bot     BotClient
	access  AccessControl
	handler MessageHandler
}

func New(token string, access AccessControl, handler MessageHandler, opts ...Option) (*Adapter, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("telegram: bot token is required")
	}

	if handler == nil {
		return nil, errors.New("telegram: message handler is required")
	}

	cfg := options{
		botFactory: func(token string, botOptions ...tgbot.Option) (BotClient, error) {
			return tgbot.New(token, botOptions...)
		},
	}

	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	adapter := &Adapter{
		access:  access,
		handler: handler,
	}

	botOptions := []tgbot.Option{
		tgbot.WithAllowedUpdates(tgbot.AllowedUpdates{models.AllowedUpdateMessage}),
		tgbot.WithDefaultHandler(func(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
			if err := adapter.HandleUpdate(ctx, update); err != nil {
				if cfg.errorHandler != nil && !errors.Is(err, ErrAccessDenied) && !errors.Is(err, ErrEmptyMessage) && !errors.Is(err, ErrUnsupportedType) {
					cfg.errorHandler(err)
				}
			}
		}),
		tgbot.WithNotAsyncHandlers(),
	}

	if cfg.initialOffset != 0 {
		botOptions = append(botOptions, tgbot.WithInitialOffset(cfg.initialOffset))
	}

	if cfg.pollTimeout > 0 {
		botOptions = append(botOptions, tgbot.WithHTTPClient(cfg.pollTimeout, &http.Client{Timeout: cfg.pollTimeout}))
	}

	if cfg.errorHandler != nil {
		botOptions = append(botOptions, tgbot.WithErrorsHandler(func(err error) {
			cfg.errorHandler(fmt.Errorf("telegram polling: %w", err))
		}))
	}

	botOptions = append(botOptions, cfg.botOptions...)

	client, err := cfg.botFactory(token, botOptions...)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}

	adapter.bot = client

	return adapter, nil
}

// Start begins Telegram long polling and runs until the context is cancelled.
func (a *Adapter) Start(ctx context.Context) error {
	a.bot.Start(ctx)
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

// HandleUpdate authorizes and normalizes a Telegram update before forwarding it.
func (a *Adapter) HandleUpdate(ctx context.Context, update *models.Update) error {
	message, err := NormalizeUpdate(update)
	if err != nil {
		return err
	}

	if !a.access.Allows(message.UserID, message.ChatID) {
		return ErrAccessDenied
	}

	return a.handler(ctx, message)
}

// Reply sends a text reply back to Telegram for a normalized inbound message.
func (a *Adapter) Reply(ctx context.Context, message router.Message, text string) error {
	var replyParameters *models.ReplyParameters
	if message.MessageID != 0 {
		replyParameters = &models.ReplyParameters{
			MessageID:                message.MessageID,
			ChatID:                   message.ChatID,
			AllowSendingWithoutReply: true,
		}
	}

	return a.sendMessage(ctx, message.ChatID, text, replyParameters, "reply text")
}

// SendAlert sends a direct outbound message to the configured allowed chat.
func (a *Adapter) SendAlert(ctx context.Context, text string) error {
	if a.access.AllowedChatID == 0 {
		return ErrAlertChatUnset
	}

	return a.sendMessage(ctx, a.access.AllowedChatID, text, nil, "alert text")
}

// SendTyping emits Telegram's native typing indicator for the target chat.
func (a *Adapter) SendTyping(ctx context.Context, message router.Message) error {
	params := &tgbot.SendChatActionParams{
		ChatID: message.ChatID,
		Action: models.ChatActionTyping,
	}
	if _, err := a.bot.SendChatAction(ctx, params); err != nil {
		return fmt.Errorf("send telegram chat action: %w", err)
	}

	return nil
}

func (a *Adapter) sendMessage(ctx context.Context, chatID int64, text string, replyParameters *models.ReplyParameters, textLabel string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("telegram: %s is required", textLabel)
	}

	params := &tgbot.SendMessageParams{
		ChatID:          chatID,
		Text:            text,
		ReplyParameters: replyParameters,
	}

	if _, err := a.bot.SendMessage(ctx, params); err != nil {
		return fmt.Errorf("send telegram message: %w", err)
	}

	return nil
}
