package calendar

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/SCWPretorius/CONTROL/internal/config"
	"github.com/SCWPretorius/CONTROL/internal/tools/googleworkspace"
)

const defaultCalendarID = "primary"

// Calendar contains typed calendar metadata.
type Calendar struct {
	ID          string
	Summary     string
	Description string
	Location    string
	TimeZone    string
	AccessRole  string
	Primary     bool
	Selected    bool
	Hidden      bool
}

// ListCalendarsRequest lists available calendars.
type ListCalendarsRequest struct {
	MaxResults    int
	PageToken     string
	MinAccessRole string
	ShowDeleted   bool
	ShowHidden    bool
}

// ListCalendarsResponse contains paginated calendar results.
type ListCalendarsResponse struct {
	Calendars     []Calendar
	NextPageToken string
	NextSyncToken string
}

// CreateCalendarRequest creates a new Google Calendar.
type CreateCalendarRequest struct {
	Summary     string
	Description string
	Location    string
	TimeZone    string
}

// EventAttendee is a typed event attendee.
type EventAttendee struct {
	Email          string
	DisplayName    string
	Optional       bool
	ResponseStatus string
}

// EventTime captures either a timed or all-day Google Calendar timestamp.
type EventTime struct {
	Time     time.Time
	Date     string
	TimeZone string
	AllDay   bool
}

// Event is a typed Google Calendar event.
type Event struct {
	ID          string
	Status      string
	HTMLLink    string
	Summary     string
	Description string
	Location    string
	CalendarID  string
	Start       EventTime
	End         EventTime
	Attendees   []EventAttendee
}

// ListEventsRequest lists events for a calendar.
type ListEventsRequest struct {
	CalendarID   string
	MaxResults   int
	PageToken    string
	Query        string
	SingleEvents bool
	ShowDeleted  bool
	TimeMin      *time.Time
	TimeMax      *time.Time
}

// ListEventsResponse contains paginated event results.
type ListEventsResponse struct {
	Events        []Event
	NextPageToken string
	NextSyncToken string
}

// CreateEventRequest creates a timed event on a calendar.
type CreateEventRequest struct {
	CalendarID  string
	Summary     string
	Description string
	Location    string
	Start       time.Time
	End         time.Time
	TimeZone    string
	Attendees   []EventAttendee
}

type clientOptions struct {
	baseURL string
}

// Option customizes a Calendar client.
type Option func(*clientOptions)

// WithBaseURL overrides the Calendar API base URL, primarily for tests.
func WithBaseURL(baseURL string) Option {
	return func(options *clientOptions) {
		options.baseURL = baseURL
	}
}

// Client is a typed Google Calendar API client.
type Client struct {
	api *googleworkspace.APIClient
}

// NewClient builds a Google Calendar client using shared OAuth settings.
func NewClient(oauth config.GoogleOAuthConfig, tokenProvider googleworkspace.AccessTokenProvider, httpClient googleworkspace.HTTPDoer, opts ...Option) (*Client, error) {
	options := clientOptions{
		baseURL: googleworkspace.CalendarBaseURL,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	api, err := googleworkspace.NewAPIClient(options.baseURL, oauth, tokenProvider, httpClient)
	if err != nil {
		return nil, err
	}

	return &Client{api: api}, nil
}

// ListCalendars lists calendars available to the authenticated user.
func (c *Client) ListCalendars(ctx context.Context, request ListCalendarsRequest) (ListCalendarsResponse, error) {
	if request.MaxResults < 0 {
		return ListCalendarsResponse{}, errors.New("calendar: max results must be zero or greater")
	}

	query := url.Values{}
	if request.MaxResults > 0 {
		query.Set("maxResults", strconv.Itoa(request.MaxResults))
	}
	if pageToken := strings.TrimSpace(request.PageToken); pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	if role := strings.TrimSpace(request.MinAccessRole); role != "" {
		query.Set("minAccessRole", role)
	}
	if request.ShowDeleted {
		query.Set("showDeleted", "true")
	}
	if request.ShowHidden {
		query.Set("showHidden", "true")
	}

	var response calendarListResource
	err := c.api.DoJSON(ctx, http.MethodGet, "/users/me/calendarList", query, nil, &response)
	if err != nil {
		return ListCalendarsResponse{}, err
	}

	calendars := make([]Calendar, 0, len(response.Items))
	for _, item := range response.Items {
		calendars = append(calendars, Calendar{
			ID:          item.ID,
			Summary:     item.Summary,
			Description: item.Description,
			Location:    item.Location,
			TimeZone:    item.TimeZone,
			AccessRole:  item.AccessRole,
			Primary:     item.Primary,
			Selected:    item.Selected,
			Hidden:      item.Hidden,
		})
	}

	return ListCalendarsResponse{
		Calendars:     calendars,
		NextPageToken: response.NextPageToken,
		NextSyncToken: response.NextSyncToken,
	}, nil
}

// CreateCalendar creates a new calendar.
func (c *Client) CreateCalendar(ctx context.Context, request CreateCalendarRequest) (Calendar, error) {
	if strings.TrimSpace(request.Summary) == "" {
		return Calendar{}, errors.New("calendar: summary is required")
	}

	body := map[string]string{
		"summary":     strings.TrimSpace(request.Summary),
		"description": strings.TrimSpace(request.Description),
		"location":    strings.TrimSpace(request.Location),
		"timeZone":    strings.TrimSpace(request.TimeZone),
	}

	var response calendarResource
	err := c.api.DoJSON(ctx, http.MethodPost, "/calendars", nil, body, &response)
	if err != nil {
		return Calendar{}, err
	}

	return Calendar{
		ID:          response.ID,
		Summary:     response.Summary,
		Description: response.Description,
		Location:    response.Location,
		TimeZone:    response.TimeZone,
	}, nil
}

// ListEvents lists events from a calendar. An empty CalendarID targets the
// primary calendar.
func (c *Client) ListEvents(ctx context.Context, request ListEventsRequest) (ListEventsResponse, error) {
	calendarID := strings.TrimSpace(request.CalendarID)
	if calendarID == "" {
		calendarID = defaultCalendarID
	}
	if request.MaxResults < 0 {
		return ListEventsResponse{}, errors.New("calendar: max results must be zero or greater")
	}
	if request.TimeMin != nil && request.TimeMax != nil && request.TimeMax.Before(*request.TimeMin) {
		return ListEventsResponse{}, errors.New("calendar: time max must not be before time min")
	}

	query := url.Values{}
	if request.MaxResults > 0 {
		query.Set("maxResults", strconv.Itoa(request.MaxResults))
	}
	if pageToken := strings.TrimSpace(request.PageToken); pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	if searchQuery := strings.TrimSpace(request.Query); searchQuery != "" {
		query.Set("q", searchQuery)
	}
	if request.SingleEvents {
		query.Set("singleEvents", "true")
	}
	if request.ShowDeleted {
		query.Set("showDeleted", "true")
	}
	if request.TimeMin != nil {
		query.Set("timeMin", request.TimeMin.UTC().Format(time.RFC3339))
	}
	if request.TimeMax != nil {
		query.Set("timeMax", request.TimeMax.UTC().Format(time.RFC3339))
	}

	var response eventsListResource
	err := c.api.DoJSON(ctx, http.MethodGet,
		fmt.Sprintf("/calendars/%s/events", url.PathEscape(calendarID)),
		query,
		nil,
		&response,
	)
	if err != nil {
		return ListEventsResponse{}, err
	}

	events := make([]Event, 0, len(response.Items))
	for _, item := range response.Items {
		event, err := toEvent(calendarID, item)
		if err != nil {
			return ListEventsResponse{}, err
		}
		events = append(events, event)
	}

	return ListEventsResponse{
		Events:        events,
		NextPageToken: response.NextPageToken,
		NextSyncToken: response.NextSyncToken,
	}, nil
}

// CreateEvent creates a timed event. An empty CalendarID targets the primary
// calendar.
func (c *Client) CreateEvent(ctx context.Context, request CreateEventRequest) (Event, error) {
	if strings.TrimSpace(request.Summary) == "" {
		return Event{}, errors.New("calendar: summary is required")
	}
	if request.Start.IsZero() || request.End.IsZero() {
		return Event{}, errors.New("calendar: start and end times are required")
	}
	if !request.End.After(request.Start) {
		return Event{}, errors.New("calendar: end time must be after start time")
	}

	calendarID := strings.TrimSpace(request.CalendarID)
	if calendarID == "" {
		calendarID = defaultCalendarID
	}

	body := createEventResource{
		Summary:     strings.TrimSpace(request.Summary),
		Description: strings.TrimSpace(request.Description),
		Location:    strings.TrimSpace(request.Location),
		Start: eventTimeResource{
			DateTime: request.Start.Format(time.RFC3339),
			TimeZone: eventTimeZone(request.TimeZone, request.Start),
		},
		End: eventTimeResource{
			DateTime: request.End.Format(time.RFC3339),
			TimeZone: eventTimeZone(request.TimeZone, request.End),
		},
		Attendees: make([]attendeeResource, 0, len(request.Attendees)),
	}

	for _, attendee := range request.Attendees {
		if _, err := mail.ParseAddress(strings.TrimSpace(attendee.Email)); err != nil {
			return Event{}, fmt.Errorf("calendar: invalid attendee %q: %w", attendee.Email, err)
		}
		body.Attendees = append(body.Attendees, attendeeResource{
			Email:          strings.TrimSpace(attendee.Email),
			DisplayName:    strings.TrimSpace(attendee.DisplayName),
			Optional:       attendee.Optional,
			ResponseStatus: strings.TrimSpace(attendee.ResponseStatus),
		})
	}

	var response eventResource
	err := c.api.DoJSON(ctx, http.MethodPost,
		fmt.Sprintf("/calendars/%s/events", url.PathEscape(calendarID)),
		nil,
		body,
		&response,
	)
	if err != nil {
		return Event{}, err
	}

	return toEvent(calendarID, response)
}

func eventTimeZone(explicit string, fallback time.Time) string {
	if zone := strings.TrimSpace(explicit); zone != "" {
		return zone
	}

	location := fallback.Location()
	if location == nil || location.String() == "" {
		return ""
	}

	if location.String() == "Local" {
		return ""
	}

	return location.String()
}

func toEvent(calendarID string, resource eventResource) (Event, error) {
	start, err := parseEventTime(resource.Start)
	if err != nil {
		return Event{}, fmt.Errorf("calendar: parse start time: %w", err)
	}
	end, err := parseEventTime(resource.End)
	if err != nil {
		return Event{}, fmt.Errorf("calendar: parse end time: %w", err)
	}

	attendees := make([]EventAttendee, 0, len(resource.Attendees))
	for _, attendee := range resource.Attendees {
		attendees = append(attendees, EventAttendee{
			Email:          attendee.Email,
			DisplayName:    attendee.DisplayName,
			Optional:       attendee.Optional,
			ResponseStatus: attendee.ResponseStatus,
		})
	}

	return Event{
		ID:          resource.ID,
		Status:      resource.Status,
		HTMLLink:    resource.HTMLLink,
		Summary:     resource.Summary,
		Description: resource.Description,
		Location:    resource.Location,
		CalendarID:  calendarID,
		Start:       start,
		End:         end,
		Attendees:   attendees,
	}, nil
}

func parseEventTime(resource eventTimeResource) (EventTime, error) {
	if date := strings.TrimSpace(resource.Date); date != "" {
		return EventTime{
			Date:     date,
			TimeZone: strings.TrimSpace(resource.TimeZone),
			AllDay:   true,
		}, nil
	}

	dateTime := strings.TrimSpace(resource.DateTime)
	if dateTime == "" {
		return EventTime{}, nil
	}

	parsed, err := time.Parse(time.RFC3339, dateTime)
	if err != nil {
		return EventTime{}, err
	}

	return EventTime{
		Time:     parsed,
		TimeZone: strings.TrimSpace(resource.TimeZone),
	}, nil
}

type calendarListResource struct {
	Items         []calendarResource `json:"items"`
	NextPageToken string             `json:"nextPageToken"`
	NextSyncToken string             `json:"nextSyncToken"`
}

type calendarResource struct {
	ID          string `json:"id"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Location    string `json:"location"`
	TimeZone    string `json:"timeZone"`
	AccessRole  string `json:"accessRole"`
	Primary     bool   `json:"primary"`
	Selected    bool   `json:"selected"`
	Hidden      bool   `json:"hidden"`
}

type eventsListResource struct {
	Items         []eventResource `json:"items"`
	NextPageToken string          `json:"nextPageToken"`
	NextSyncToken string          `json:"nextSyncToken"`
}

type eventResource struct {
	ID          string             `json:"id"`
	Status      string             `json:"status"`
	HTMLLink    string             `json:"htmlLink"`
	Summary     string             `json:"summary"`
	Description string             `json:"description"`
	Location    string             `json:"location"`
	Start       eventTimeResource  `json:"start"`
	End         eventTimeResource  `json:"end"`
	Attendees   []attendeeResource `json:"attendees"`
}

type createEventResource struct {
	Summary     string             `json:"summary"`
	Description string             `json:"description,omitempty"`
	Location    string             `json:"location,omitempty"`
	Start       eventTimeResource  `json:"start"`
	End         eventTimeResource  `json:"end"`
	Attendees   []attendeeResource `json:"attendees,omitempty"`
}

type eventTimeResource struct {
	Date     string `json:"date,omitempty"`
	DateTime string `json:"dateTime,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}

type attendeeResource struct {
	Email          string `json:"email"`
	DisplayName    string `json:"displayName,omitempty"`
	Optional       bool   `json:"optional,omitempty"`
	ResponseStatus string `json:"responseStatus,omitempty"`
}
