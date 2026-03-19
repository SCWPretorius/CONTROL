package copilottools

import (
	"testing"
	"time"

	"github.com/SCWPretorius/CONTROL/internal/config"
)

func TestLayerToolsExposeExpectedNames(t *testing.T) {
	t.Parallel()

	layer, err := NewLayer(config.GoogleToolConfig{
		OAuth: config.GoogleOAuthConfig{
			ClientID:     "client-id",
			ClientSecret: "client-secret",
			RedirectURL:  "http://127.0.0.1:8787/oauth/callback",
			Scopes:       []string{"scope-a"},
		},
		AccessToken: "access-token",
	}, config.ToolRuntimeConfig{HTTPTimeout: time.Second}, nil, nil)
	if err != nil {
		t.Fatalf("NewLayer() error = %v", err)
	}

	got := []string{}
	for _, tool := range layer.Tools() {
		got = append(got, tool.Name)
	}
	want := []string{
		ToolNameGmailSearchMessages,
		ToolNameGmailGetMessage,
		ToolNameGmailSendMessage,
		ToolNameCalendarListCalendars,
		ToolNameCalendarListEvents,
		ToolNameCalendarCreateEvent,
	}
	if len(got) != len(want) {
		t.Fatalf("tool count = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tool[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
