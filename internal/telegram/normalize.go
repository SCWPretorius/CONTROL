package telegram

import (
	"strings"

	"github.com/SCWPretorius/CONTROL/internal/router"
	"github.com/go-telegram/bot/models"
)

// NormalizeUpdate converts a supported Telegram update into the internal router contract.
func NormalizeUpdate(update *models.Update) (router.Message, error) {
	if update == nil {
		return router.Message{}, ErrNilUpdate
	}

	message := messageFromUpdate(update)
	if message == nil {
		return router.Message{}, ErrUnsupportedType
	}

	if message.From == nil {
		return router.Message{}, ErrMissingSender
	}

	text := strings.TrimSpace(message.Text)
	if text == "" {
		text = strings.TrimSpace(message.Caption)
	}
	if text == "" {
		return router.Message{}, ErrEmptyMessage
	}

	return router.Message{
		Transport: TransportName,
		MessageID: message.ID,
		ChatID:    message.Chat.ID,
		UserID:    message.From.ID,
		Text:      text,
		Metadata: router.MessageMetadata{
			ChatTitle:    strings.TrimSpace(message.Chat.Title),
			Username:     strings.TrimSpace(message.From.Username),
			FirstName:    strings.TrimSpace(message.From.FirstName),
			LastName:     strings.TrimSpace(message.From.LastName),
			LanguageCode: strings.TrimSpace(message.From.LanguageCode),
		},
	}, nil
}

func messageFromUpdate(update *models.Update) *models.Message {
	switch {
	case update.Message != nil:
		return update.Message
	case update.EditedMessage != nil:
		return update.EditedMessage
	case update.BusinessMessage != nil:
		return update.BusinessMessage
	case update.EditedBusinessMessage != nil:
		return update.EditedBusinessMessage
	default:
		return nil
	}
}
