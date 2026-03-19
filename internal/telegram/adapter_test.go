package telegram

import (
	"context"
	"errors"
	"testing"

	"github.com/SCWPretorius/CONTROL/internal/router"
	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func TestNormalizeUpdate(t *testing.T) {
	t.Parallel()

	update := &models.Update{
		ID: 100,
		Message: &models.Message{
			ID:   55,
			Text: " hello ",
			Chat: models.Chat{ID: 77, Title: "CONTROL"},
			From: &models.User{
				ID:           88,
				Username:     "pretorius",
				FirstName:    "Pieter",
				LastName:     "Pretorius",
				LanguageCode: "en",
			},
		},
	}

	message, err := NormalizeUpdate(update)
	if err != nil {
		t.Fatalf("NormalizeUpdate() error = %v", err)
	}

	if got, want := message.Transport, TransportName; got != want {
		t.Fatalf("Transport = %q, want %q", got, want)
	}

	if got, want := message.MessageID, 55; got != want {
		t.Fatalf("MessageID = %d, want %d", got, want)
	}

	if got, want := message.ChatID, int64(77); got != want {
		t.Fatalf("ChatID = %d, want %d", got, want)
	}

	if got, want := message.UserID, int64(88); got != want {
		t.Fatalf("UserID = %d, want %d", got, want)
	}

	if got, want := message.Text, "hello"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
	if got, want := message.Metadata.ChatTitle, "CONTROL"; got != want {
		t.Fatalf("Metadata.ChatTitle = %q, want %q", got, want)
	}
	if got, want := message.Metadata.Username, "pretorius"; got != want {
		t.Fatalf("Metadata.Username = %q, want %q", got, want)
	}
}

func TestNormalizeUpdateUsesCaptionWhenTextMissing(t *testing.T) {
	t.Parallel()

	message, err := NormalizeUpdate(&models.Update{
		EditedMessage: &models.Message{
			ID:      1,
			Caption: " caption ",
			Chat:    models.Chat{ID: 2},
			From:    &models.User{ID: 3},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeUpdate() error = %v", err)
	}

	if got, want := message.Text, "caption"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
}

func TestNormalizeUpdateRejectsUnsupportedUpdates(t *testing.T) {
	t.Parallel()

	_, err := NormalizeUpdate(&models.Update{})
	if !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("NormalizeUpdate() error = %v, want %v", err, ErrUnsupportedType)
	}
}

func TestHandleUpdateBlocksUnauthorizedMessages(t *testing.T) {
	t.Parallel()

	adapter := &Adapter{
		access: AccessControl{
			AllowedUserID: 1,
			AllowedChatID: 2,
		},
		handler: func(context.Context, router.Message) error {
			t.Fatal("handler should not be called")
			return nil
		},
	}

	err := adapter.HandleUpdate(context.Background(), &models.Update{
		Message: &models.Message{
			ID:   10,
			Text: "hello",
			Chat: models.Chat{ID: 2},
			From: &models.User{ID: 9},
		},
	})
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("HandleUpdate() error = %v, want %v", err, ErrAccessDenied)
	}
}

func TestHandleUpdatePassesNormalizedMessageToHandler(t *testing.T) {
	t.Parallel()

	called := false
	adapter := &Adapter{
		access: AccessControl{
			AllowedUserID: 7,
			AllowedChatID: 8,
		},
		handler: func(_ context.Context, message router.Message) error {
			called = true
			if got, want := message.Transport, TransportName; got != want {
				t.Fatalf("Transport = %q, want %q", got, want)
			}
			if got, want := message.MessageID, 9; got != want {
				t.Fatalf("MessageID = %d, want %d", got, want)
			}
			if got, want := message.Text, "ping"; got != want {
				t.Fatalf("Text = %q, want %q", got, want)
			}
			return nil
		},
	}

	if err := adapter.HandleUpdate(context.Background(), &models.Update{
		Message: &models.Message{
			ID:   9,
			Text: "ping",
			Chat: models.Chat{ID: 8},
			From: &models.User{ID: 7},
		},
	}); err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if !called {
		t.Fatal("handler was not called")
	}
}

func TestReplySendsReplyParameters(t *testing.T) {
	t.Parallel()

	client := &fakeBotClient{}
	adapter := &Adapter{bot: client}

	err := adapter.Reply(context.Background(), router.Message{
		ChatID:    99,
		MessageID: 123,
	}, " reply ")
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	if client.sent == nil {
		t.Fatal("Reply() did not send a message")
	}

	if got, want := client.sent.ChatID, any(int64(99)); got != want {
		t.Fatalf("ChatID = %#v, want %#v", got, want)
	}

	if got, want := client.sent.Text, "reply"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}

	if client.sent.ReplyParameters == nil {
		t.Fatal("ReplyParameters = nil, want populated reply metadata")
	}

	if got, want := client.sent.ReplyParameters.MessageID, 123; got != want {
		t.Fatalf("ReplyParameters.MessageID = %d, want %d", got, want)
	}
}

func TestSendTypingSendsTypingChatAction(t *testing.T) {
	t.Parallel()

	client := &fakeBotClient{}
	adapter := &Adapter{bot: client}

	err := adapter.SendTyping(context.Background(), router.Message{ChatID: 99})
	if err != nil {
		t.Fatalf("SendTyping() error = %v", err)
	}

	if client.chatAction == nil {
		t.Fatal("SendTyping() did not send a chat action")
	}

	if got, want := client.chatAction.ChatID, any(int64(99)); got != want {
		t.Fatalf("ChatID = %#v, want %#v", got, want)
	}
	if got, want := client.chatAction.Action, models.ChatActionTyping; got != want {
		t.Fatalf("Action = %q, want %q", got, want)
	}
}

type fakeBotClient struct {
	sent       *tgbot.SendMessageParams
	chatAction *tgbot.SendChatActionParams
}

func (f *fakeBotClient) Start(context.Context) {}

func (f *fakeBotClient) SendMessage(_ context.Context, params *tgbot.SendMessageParams) (*models.Message, error) {
	f.sent = params
	return &models.Message{}, nil
}

func (f *fakeBotClient) SendChatAction(_ context.Context, params *tgbot.SendChatActionParams) (bool, error) {
	f.chatAction = params
	return true, nil
}
