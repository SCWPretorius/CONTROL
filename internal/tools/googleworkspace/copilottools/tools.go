package copilottools

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/github/copilot-sdk/go"

	"github.com/SCWPretorius/CONTROL/internal/tools/googleworkspace/calendar"
	"github.com/SCWPretorius/CONTROL/internal/tools/googleworkspace/gmail"
)

const (
	ToolNameGmailSearchMessages   = "google_gmail_search_messages"
	ToolNameGmailGetMessage       = "google_gmail_get_message"
	ToolNameGmailSendMessage      = "google_gmail_send_message"
	ToolNameCalendarListCalendars = "google_calendar_list_calendars"
	ToolNameCalendarListEvents    = "google_calendar_list_events"
	ToolNameCalendarCreateEvent   = "google_calendar_create_event"
)

type gmailSearchMessagesInput struct {
	Query            string   `json:"query"`
	LabelIDs         []string `json:"label_ids,omitempty"`
	PageToken        string   `json:"page_token,omitempty"`
	MaxResults       int      `json:"max_results,omitempty"`
	IncludeSpamTrash bool     `json:"include_spam_trash,omitempty"`
}

type gmailGetMessageInput struct {
	MessageID string `json:"message_id"`
	Format    string `json:"format,omitempty"`
}

type emailRecipient struct {
	Name    string `json:"name,omitempty"`
	Address string `json:"address"`
}

type gmailSendMessageInput struct {
	To         []emailRecipient `json:"to"`
	Cc         []emailRecipient `json:"cc,omitempty"`
	Bcc        []emailRecipient `json:"bcc,omitempty"`
	Subject    string           `json:"subject,omitempty"`
	TextBody   string           `json:"text_body,omitempty"`
	HTMLBody   string           `json:"html_body,omitempty"`
	ThreadID   string           `json:"thread_id,omitempty"`
	InReplyTo  string           `json:"in_reply_to,omitempty"`
	References []string         `json:"references,omitempty"`
}

type calendarListCalendarsInput struct {
	MaxResults    int    `json:"max_results,omitempty"`
	PageToken     string `json:"page_token,omitempty"`
	MinAccessRole string `json:"min_access_role,omitempty"`
	ShowDeleted   bool   `json:"show_deleted,omitempty"`
	ShowHidden    bool   `json:"show_hidden,omitempty"`
}

type calendarListEventsInput struct {
	CalendarID   string `json:"calendar_id,omitempty"`
	MaxResults   int    `json:"max_results,omitempty"`
	PageToken    string `json:"page_token,omitempty"`
	Query        string `json:"query,omitempty"`
	SingleEvents bool   `json:"single_events,omitempty"`
	ShowDeleted  bool   `json:"show_deleted,omitempty"`
	TimeMin      string `json:"time_min,omitempty"`
	TimeMax      string `json:"time_max,omitempty"`
}

type calendarAttendee struct {
	Email          string `json:"email"`
	DisplayName    string `json:"display_name,omitempty"`
	Optional       bool   `json:"optional,omitempty"`
	ResponseStatus string `json:"response_status,omitempty"`
}

type calendarCreateEventInput struct {
	CalendarID  string             `json:"calendar_id,omitempty"`
	Summary     string             `json:"summary"`
	Description string             `json:"description,omitempty"`
	Location    string             `json:"location,omitempty"`
	Start       string             `json:"start"`
	End         string             `json:"end"`
	TimeZone    string             `json:"time_zone,omitempty"`
	Attendees   []calendarAttendee `json:"attendees,omitempty"`
}

func (l *Layer) GmailSearchMessagesTool() sdk.Tool {
	return sdk.DefineTool(ToolNameGmailSearchMessages,
		"Search Gmail messages using Gmail query syntax, optionally filtered by labels and page token.",
		func(input gmailSearchMessagesInput, invocation sdk.ToolInvocation) (gmail.ListMessagesResponse, error) {
			_ = invocation
			return l.gmail.SearchMessages(context.Background(), gmail.SearchMessagesRequest{
				Query:            input.Query,
				LabelIDs:         append([]string(nil), input.LabelIDs...),
				PageToken:        input.PageToken,
				MaxResults:       input.MaxResults,
				IncludeSpamTrash: input.IncludeSpamTrash,
			})
		},
	)
}

func (l *Layer) GmailGetMessageTool() sdk.Tool {
	return sdk.DefineTool(ToolNameGmailGetMessage,
		"Fetch a Gmail message by ID, returning headers, snippet, and parsed text/html body content when available.",
		func(input gmailGetMessageInput, invocation sdk.ToolInvocation) (gmail.Message, error) {
			_ = invocation
			return l.gmail.GetMessage(context.Background(), gmail.GetMessageRequest{
				MessageID: input.MessageID,
				Format:    gmail.MessageFormat(strings.TrimSpace(input.Format)),
			})
		},
	)
}

func (l *Layer) GmailSendMessageTool() sdk.Tool {
	return sdk.DefineTool(ToolNameGmailSendMessage,
		"Send an email through Gmail with to/cc/bcc recipients, subject, and text and/or HTML body content.",
		func(input gmailSendMessageInput, invocation sdk.ToolInvocation) (gmail.SentMessage, error) {
			_ = invocation
			return l.gmail.SendMessage(context.Background(), gmail.SendMessageRequest{
				To:         toGmailRecipients(input.To),
				Cc:         toGmailRecipients(input.Cc),
				Bcc:        toGmailRecipients(input.Bcc),
				Subject:    input.Subject,
				TextBody:   input.TextBody,
				HTMLBody:   input.HTMLBody,
				ThreadID:   input.ThreadID,
				InReplyTo:  input.InReplyTo,
				References: append([]string(nil), input.References...),
			})
		},
	)
}

func (l *Layer) CalendarListCalendarsTool() sdk.Tool {
	return sdk.DefineTool(ToolNameCalendarListCalendars,
		"List Google Calendars available to the authenticated account.",
		func(input calendarListCalendarsInput, invocation sdk.ToolInvocation) (calendar.ListCalendarsResponse, error) {
			_ = invocation
			return l.calendar.ListCalendars(context.Background(), calendar.ListCalendarsRequest{
				MaxResults:    input.MaxResults,
				PageToken:     input.PageToken,
				MinAccessRole: input.MinAccessRole,
				ShowDeleted:   input.ShowDeleted,
				ShowHidden:    input.ShowHidden,
			})
		},
	)
}

func (l *Layer) CalendarListEventsTool() sdk.Tool {
	return sdk.DefineTool(ToolNameCalendarListEvents,
		"List Google Calendar events for a calendar and optional time range.",
		func(input calendarListEventsInput, invocation sdk.ToolInvocation) (calendar.ListEventsResponse, error) {
			_ = invocation
			timeMin, err := parseOptionalRFC3339(input.TimeMin, "time_min")
			if err != nil {
				return calendar.ListEventsResponse{}, err
			}
			timeMax, err := parseOptionalRFC3339(input.TimeMax, "time_max")
			if err != nil {
				return calendar.ListEventsResponse{}, err
			}

			return l.calendar.ListEvents(context.Background(), calendar.ListEventsRequest{
				CalendarID:   input.CalendarID,
				MaxResults:   input.MaxResults,
				PageToken:    input.PageToken,
				Query:        input.Query,
				SingleEvents: input.SingleEvents,
				ShowDeleted:  input.ShowDeleted,
				TimeMin:      timeMin,
				TimeMax:      timeMax,
			})
		},
	)
}

func (l *Layer) CalendarCreateEventTool() sdk.Tool {
	return sdk.DefineTool(ToolNameCalendarCreateEvent,
		"Create a timed Google Calendar event using RFC3339 start/end timestamps and optional attendees.",
		func(input calendarCreateEventInput, invocation sdk.ToolInvocation) (calendar.Event, error) {
			_ = invocation
			start, err := parseRequiredRFC3339(input.Start, "start")
			if err != nil {
				return calendar.Event{}, err
			}
			end, err := parseRequiredRFC3339(input.End, "end")
			if err != nil {
				return calendar.Event{}, err
			}

			return l.calendar.CreateEvent(context.Background(), calendar.CreateEventRequest{
				CalendarID:  input.CalendarID,
				Summary:     input.Summary,
				Description: input.Description,
				Location:    input.Location,
				Start:       start,
				End:         end,
				TimeZone:    input.TimeZone,
				Attendees:   toCalendarAttendees(input.Attendees),
			})
		},
	)
}

func toGmailRecipients(recipients []emailRecipient) []gmail.Recipient {
	if len(recipients) == 0 {
		return nil
	}

	converted := make([]gmail.Recipient, 0, len(recipients))
	for _, recipient := range recipients {
		converted = append(converted, gmail.Recipient{
			Name:    recipient.Name,
			Address: recipient.Address,
		})
	}
	return converted
}

func toCalendarAttendees(attendees []calendarAttendee) []calendar.EventAttendee {
	if len(attendees) == 0 {
		return nil
	}

	converted := make([]calendar.EventAttendee, 0, len(attendees))
	for _, attendee := range attendees {
		converted = append(converted, calendar.EventAttendee{
			Email:          attendee.Email,
			DisplayName:    attendee.DisplayName,
			Optional:       attendee.Optional,
			ResponseStatus: attendee.ResponseStatus,
		})
	}
	return converted
}

func parseOptionalRFC3339(value, field string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("google workspace: %s must be RFC3339: %w", field, err)
	}
	return &parsed, nil
}

func parseRequiredRFC3339(value, field string) (time.Time, error) {
	parsed, err := parseOptionalRFC3339(value, field)
	if err != nil {
		return time.Time{}, err
	}
	if parsed == nil {
		return time.Time{}, fmt.Errorf("google workspace: %s is required", field)
	}
	return *parsed, nil
}
