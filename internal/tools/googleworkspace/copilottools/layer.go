package copilottools

import (
	"errors"
	"fmt"
	"net/http"

	sdk "github.com/github/copilot-sdk/go"

	"github.com/SCWPretorius/CONTROL/internal/config"
	"github.com/SCWPretorius/CONTROL/internal/tools/googleworkspace"
	"github.com/SCWPretorius/CONTROL/internal/tools/googleworkspace/calendar"
	"github.com/SCWPretorius/CONTROL/internal/tools/googleworkspace/gmail"
)

// Layer owns the Google Workspace client set exposed to Copilot as custom tools.
type Layer struct {
	gmail    *gmail.Client
	calendar *calendar.Client
}

// NewLayer constructs a Google Workspace tool layer from existing config.
func NewLayer(cfg config.GoogleToolConfig, runtime config.ToolRuntimeConfig, tokenProvider googleworkspace.AccessTokenProvider, httpClient googleworkspace.HTTPDoer) (*Layer, error) {
	if !cfg.OAuth.Enabled() {
		return nil, errors.New("google workspace: oauth config is not enabled")
	}
	if runtime.HTTPTimeout <= 0 && httpClient == nil {
		return nil, errors.New("google workspace: http timeout must be greater than zero")
	}
	if tokenProvider == nil {
		var err error
		tokenProvider, err = googleworkspace.NewEnvAccessTokenProvider(cfg)
		if err != nil {
			return nil, err
		}
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: runtime.HTTPTimeout}
	}

	gmailClient, err := gmail.NewClient(cfg.OAuth, tokenProvider, httpClient)
	if err != nil {
		return nil, fmt.Errorf("google workspace: create gmail client: %w", err)
	}
	calendarClient, err := calendar.NewClient(cfg.OAuth, tokenProvider, httpClient)
	if err != nil {
		return nil, fmt.Errorf("google workspace: create calendar client: %w", err)
	}

	return &Layer{
		gmail:    gmailClient,
		calendar: calendarClient,
	}, nil
}

// Tools exposes the Google Workspace tool set without wiring it into the runtime.
func (l *Layer) Tools() []sdk.Tool {
	return []sdk.Tool{
		l.GmailSearchMessagesTool(),
		l.GmailGetMessageTool(),
		l.GmailSendMessageTool(),
		l.CalendarListCalendarsTool(),
		l.CalendarListEventsTool(),
		l.CalendarCreateEventTool(),
	}
}
