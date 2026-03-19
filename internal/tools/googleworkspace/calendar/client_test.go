package calendar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SCWPretorius/CONTROL/internal/config"
)

func TestListCalendarsBuildsExpectedRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.URL.Path, "/users/me/calendarList"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := request.Header.Get("Authorization"), "Bearer token-123"; got != want {
			t.Fatalf("Authorization = %q, want %q", got, want)
		}
		if got, want := request.URL.Query().Get("minAccessRole"), "owner"; got != want {
			t.Fatalf("minAccessRole = %q, want %q", got, want)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"items":[{"id":"primary","summary":"Personal","timeZone":"UTC","accessRole":"owner","primary":true}],
			"nextSyncToken":"sync-1"
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	response, err := client.ListCalendars(context.Background(), ListCalendarsRequest{MinAccessRole: "owner"})
	if err != nil {
		t.Fatalf("ListCalendars() error = %v", err)
	}

	if got, want := len(response.Calendars), 1; got != want {
		t.Fatalf("len(Calendars) = %d, want %d", got, want)
	}
	if got, want := response.Calendars[0].ID, "primary"; got != want {
		t.Fatalf("Calendars[0].ID = %q, want %q", got, want)
	}
	if got, want := response.NextSyncToken, "sync-1"; got != want {
		t.Fatalf("NextSyncToken = %q, want %q", got, want)
	}
}

func TestCreateCalendarSendsExpectedPayload(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Method, http.MethodPost; got != want {
			t.Fatalf("method = %q, want %q", got, want)
		}
		if got, want := request.URL.Path, "/calendars"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}

		var payload map[string]string
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if got, want := payload["summary"], "CONTROL"; got != want {
			t.Fatalf("summary = %q, want %q", got, want)
		}
		if got, want := payload["timeZone"], "Africa/Johannesburg"; got != want {
			t.Fatalf("timeZone = %q, want %q", got, want)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"calendar-1","summary":"CONTROL","timeZone":"Africa/Johannesburg"}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	created, err := client.CreateCalendar(context.Background(), CreateCalendarRequest{
		Summary:  "CONTROL",
		TimeZone: "Africa/Johannesburg",
	})
	if err != nil {
		t.Fatalf("CreateCalendar() error = %v", err)
	}

	if got, want := created.ID, "calendar-1"; got != want {
		t.Fatalf("ID = %q, want %q", got, want)
	}
}

func TestListEventsParsesTimedAndAllDayEvents(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.URL.Path, "/calendars/primary/events"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := request.URL.Query().Get("singleEvents"), "true"; got != want {
			t.Fatalf("singleEvents = %q, want %q", got, want)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"items":[
				{
					"id":"evt-1",
					"summary":"Planning",
					"start":{"dateTime":"2025-01-02T09:00:00Z","timeZone":"UTC"},
					"end":{"dateTime":"2025-01-02T10:00:00Z","timeZone":"UTC"}
				},
				{
					"id":"evt-2",
					"summary":"Holiday",
					"start":{"date":"2025-01-03"},
					"end":{"date":"2025-01-04"}
				}
			]
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	response, err := client.ListEvents(context.Background(), ListEventsRequest{SingleEvents: true})
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}

	if got, want := len(response.Events), 2; got != want {
		t.Fatalf("len(Events) = %d, want %d", got, want)
	}
	if got, want := response.Events[0].Start.Time, time.Date(2025, time.January, 2, 9, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("Events[0].Start.Time = %v, want %v", got, want)
	}
	if !response.Events[1].Start.AllDay || response.Events[1].Start.Date != "2025-01-03" {
		t.Fatalf("Events[1].Start = %#v, want all-day event", response.Events[1].Start)
	}
}

func TestCreateEventSendsExpectedPayload(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Method, http.MethodPost; got != want {
			t.Fatalf("method = %q, want %q", got, want)
		}
		if got, want := request.URL.Path, "/calendars/team-calendar/events"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}

		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if got, want := payload["summary"], "Sprint review"; got != want {
			t.Fatalf("summary = %v, want %v", got, want)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id":"evt-9",
			"summary":"Sprint review",
			"start":{"dateTime":"2025-01-02T15:00:00Z","timeZone":"UTC"},
			"end":{"dateTime":"2025-01-02T16:00:00Z","timeZone":"UTC"},
			"attendees":[{"email":"lead@example.com","displayName":"Lead"}]
		}`))
	}))
	defer server.Close()

	start := time.Date(2025, time.January, 2, 15, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	client := newTestClient(t, server.URL)
	event, err := client.CreateEvent(context.Background(), CreateEventRequest{
		CalendarID: "team-calendar",
		Summary:    "Sprint review",
		Start:      start,
		End:        end,
		TimeZone:   "UTC",
		Attendees: []EventAttendee{
			{Email: "lead@example.com", DisplayName: "Lead"},
		},
	})
	if err != nil {
		t.Fatalf("CreateEvent() error = %v", err)
	}

	if got, want := event.ID, "evt-9"; got != want {
		t.Fatalf("ID = %q, want %q", got, want)
	}
	if got, want := len(event.Attendees), 1; got != want {
		t.Fatalf("len(Attendees) = %d, want %d", got, want)
	}
}

func TestCreateEventValidatesTimeRange(t *testing.T) {
	t.Parallel()

	start := time.Date(2025, time.January, 2, 15, 0, 0, 0, time.UTC)
	client := newTestClient(t, "https://example.invalid")
	_, err := client.CreateEvent(context.Background(), CreateEventRequest{
		Summary: "Invalid",
		Start:   start,
		End:     start,
	})
	if err == nil || !strings.Contains(err.Error(), "end time must be after start time") {
		t.Fatalf("CreateEvent() error = %v, want time-range validation error", err)
	}
}

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()

	client, err := NewClient(config.GoogleOAuthConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "http://127.0.0.1:8787/oauth/callback",
		Scopes:       []string{"scope-a"},
	}, staticTokenProvider("token-123"), http.DefaultClient, WithBaseURL(baseURL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	return client
}

type staticTokenProvider string

func (p staticTokenProvider) AccessToken(context.Context) (string, error) {
	return string(p), nil
}
