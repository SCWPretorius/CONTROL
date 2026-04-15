package gmail

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/mail"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/SCWPretorius/CONTROL/internal/config"
	"github.com/SCWPretorius/CONTROL/internal/tools/googleworkspace"
)

const defaultUserID = "me"

// MessageFormat controls Gmail get-message verbosity.
type MessageFormat string

const (
	MessageFormatFull     MessageFormat = "full"
	MessageFormatMetadata MessageFormat = "metadata"
	MessageFormatMinimal  MessageFormat = "minimal"
	MessageFormatRaw      MessageFormat = "raw"
)

// Recipient is a typed email recipient.
type Recipient struct {
	Name    string
	Address string
}

// MessageReference identifies a Gmail message.
type MessageReference struct {
	ID       string
	ThreadID string
}

// ListMessagesRequest lists messages without forcing a search query.
type ListMessagesRequest struct {
	LabelIDs         []string
	PageToken        string
	Query            string
	MaxResults       int
	IncludeSpamTrash bool
}

// SearchMessagesRequest performs a filtered Gmail search.
type SearchMessagesRequest struct {
	LabelIDs         []string
	PageToken        string
	Query            string
	MaxResults       int
	IncludeSpamTrash bool
}

// ListMessagesResponse contains Gmail list/search results.
type ListMessagesResponse struct {
	Messages           []MessageReference
	NextPageToken      string
	ResultSizeEstimate int64
}

// GetMessageRequest fetches a specific Gmail message.
type GetMessageRequest struct {
	MessageID string
	Format    MessageFormat
}

// Message is a parsed Gmail message.
type Message struct {
	ID            string
	ThreadID      string
	HistoryID     string
	Snippet       string
	LabelIDs      []string
	InternalDate  time.Time
	Headers       map[string]string
	PlainTextBody string
	HTMLBody      string
	Attachments   []Attachment
	Raw           string
}

// Attachment describes a Gmail message attachment.
type Attachment struct {
	PartID       string
	AttachmentID string
	Filename     string
	MimeType     string
	Size         int64
	bodyData     string
}

// DownloadAttachmentRequest fetches attachment content for a Gmail message part.
type DownloadAttachmentRequest struct {
	MessageID  string
	Attachment Attachment
}

// SendMessageRequest sends a composed email through Gmail.
type SendMessageRequest struct {
	To         []Recipient
	Cc         []Recipient
	Bcc        []Recipient
	Subject    string
	TextBody   string
	HTMLBody   string
	ThreadID   string
	InReplyTo  string
	References []string
}

// SentMessage is the typed Gmail send response.
type SentMessage struct {
	ID       string
	ThreadID string
	LabelIDs []string
}

type clientOptions struct {
	baseURL string
	userID  string
}

// Option customizes a Gmail client.
type Option func(*clientOptions)

// WithBaseURL overrides the Gmail API base URL, primarily for tests.
func WithBaseURL(baseURL string) Option {
	return func(options *clientOptions) {
		options.baseURL = baseURL
	}
}

// WithUserID overrides the Gmail user identifier. The default is "me".
func WithUserID(userID string) Option {
	return func(options *clientOptions) {
		options.userID = userID
	}
}

// Client is a typed Gmail API client.
type Client struct {
	api    *googleworkspace.APIClient
	userID string
}

// NewClient builds a Gmail client using the shared Google OAuth environment
// config and a runtime-provided token source.
func NewClient(oauth config.GoogleOAuthConfig, tokenProvider googleworkspace.AccessTokenProvider, httpClient googleworkspace.HTTPDoer, opts ...Option) (*Client, error) {
	options := clientOptions{
		baseURL: googleworkspace.GmailBaseURL,
		userID:  defaultUserID,
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

	userID := strings.TrimSpace(options.userID)
	if userID == "" {
		userID = defaultUserID
	}

	return &Client{
		api:    api,
		userID: userID,
	}, nil
}

// ListMessages returns basic Gmail message identifiers.
func (c *Client) ListMessages(ctx context.Context, request ListMessagesRequest) (ListMessagesResponse, error) {
	return c.listMessages(ctx, request)
}

// SearchMessages searches Gmail using the provided query string.
func (c *Client) SearchMessages(ctx context.Context, request SearchMessagesRequest) (ListMessagesResponse, error) {
	if strings.TrimSpace(request.Query) == "" {
		return ListMessagesResponse{}, errors.New("gmail: search query is required")
	}

	return c.listMessages(ctx, ListMessagesRequest(request))
}

// GetMessage fetches and parses a Gmail message.
func (c *Client) GetMessage(ctx context.Context, request GetMessageRequest) (Message, error) {
	if strings.TrimSpace(request.MessageID) == "" {
		return Message{}, errors.New("gmail: message ID is required")
	}

	format := request.Format
	if format == "" {
		format = MessageFormatFull
	}
	if !isSupportedFormat(format) {
		return Message{}, fmt.Errorf("gmail: unsupported message format %q", format)
	}

	var response messageResource
	err := c.api.DoJSON(ctx, http.MethodGet,
		fmt.Sprintf("/users/%s/messages/%s", url.PathEscape(c.userID), url.PathEscape(strings.TrimSpace(request.MessageID))),
		url.Values{"format": []string{string(format)}},
		nil,
		&response,
	)
	if err != nil {
		return Message{}, err
	}

	return toMessage(response)
}

// DownloadAttachment fetches and decodes attachment bytes for a Gmail message part.
func (c *Client) DownloadAttachment(ctx context.Context, request DownloadAttachmentRequest) ([]byte, error) {
	if strings.TrimSpace(request.MessageID) == "" {
		return nil, errors.New("gmail: message ID is required")
	}
	if strings.TrimSpace(request.Attachment.AttachmentID) == "" && strings.TrimSpace(request.Attachment.bodyData) == "" {
		return nil, errors.New("gmail: attachment body or attachment ID is required")
	}
	if strings.TrimSpace(request.Attachment.bodyData) != "" {
		return decodeBodyBytes(request.Attachment.bodyData)
	}

	var response attachmentResource
	err := c.api.DoJSON(ctx, http.MethodGet,
		fmt.Sprintf(
			"/users/%s/messages/%s/attachments/%s",
			url.PathEscape(c.userID),
			url.PathEscape(strings.TrimSpace(request.MessageID)),
			url.PathEscape(strings.TrimSpace(request.Attachment.AttachmentID)),
		),
		nil,
		nil,
		&response,
	)
	if err != nil {
		return nil, err
	}

	return decodeBodyBytes(response.Data)
}

// SendMessage composes and sends an email through Gmail.
func (c *Client) SendMessage(ctx context.Context, request SendMessageRequest) (SentMessage, error) {
	if err := validateSendMessageRequest(request); err != nil {
		return SentMessage{}, err
	}

	rawMessage, err := buildRawMessage(request)
	if err != nil {
		return SentMessage{}, err
	}

	body := map[string]any{
		"raw": rawMessage,
	}
	if threadID := strings.TrimSpace(request.ThreadID); threadID != "" {
		body["threadId"] = threadID
	}

	var response sentMessageResource
	err = c.api.DoJSON(ctx, http.MethodPost,
		fmt.Sprintf("/users/%s/messages/send", url.PathEscape(c.userID)),
		nil,
		body,
		&response,
	)
	if err != nil {
		return SentMessage{}, err
	}

	return SentMessage{
		ID:       response.ID,
		ThreadID: response.ThreadID,
		LabelIDs: append([]string(nil), response.LabelIDs...),
	}, nil
}

func (c *Client) listMessages(ctx context.Context, request ListMessagesRequest) (ListMessagesResponse, error) {
	if request.MaxResults < 0 {
		return ListMessagesResponse{}, errors.New("gmail: max results must be zero or greater")
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
	for _, labelID := range request.LabelIDs {
		labelID = strings.TrimSpace(labelID)
		if labelID == "" {
			continue
		}
		query.Add("labelIds", labelID)
	}
	if request.IncludeSpamTrash {
		query.Set("includeSpamTrash", "true")
	}

	var response listMessagesResource
	err := c.api.DoJSON(ctx, http.MethodGet,
		fmt.Sprintf("/users/%s/messages", url.PathEscape(c.userID)),
		query,
		nil,
		&response,
	)
	if err != nil {
		return ListMessagesResponse{}, err
	}

	messages := make([]MessageReference, 0, len(response.Messages))
	for _, message := range response.Messages {
		messages = append(messages, MessageReference{
			ID:       message.ID,
			ThreadID: message.ThreadID,
		})
	}

	return ListMessagesResponse{
		Messages:           messages,
		NextPageToken:      response.NextPageToken,
		ResultSizeEstimate: response.ResultSizeEstimate,
	}, nil
}

func validateSendMessageRequest(request SendMessageRequest) error {
	if len(request.To) == 0 && len(request.Cc) == 0 && len(request.Bcc) == 0 {
		return errors.New("gmail: at least one recipient is required")
	}
	if strings.TrimSpace(request.TextBody) == "" && strings.TrimSpace(request.HTMLBody) == "" {
		return errors.New("gmail: text or html body is required")
	}

	for _, recipients := range [][]Recipient{request.To, request.Cc, request.Bcc} {
		for _, recipient := range recipients {
			if _, err := recipient.mailAddress(); err != nil {
				return fmt.Errorf("gmail: invalid recipient %q: %w", recipient.Address, err)
			}
		}
	}

	return nil
}

func buildRawMessage(request SendMessageRequest) (string, error) {
	var buffer bytes.Buffer

	writeHeader := func(name, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		fmt.Fprintf(&buffer, "%s: %s\r\n", name, value)
	}

	writeHeader("To", formatRecipients(request.To))
	writeHeader("Cc", formatRecipients(request.Cc))
	writeHeader("Bcc", formatRecipients(request.Bcc))
	writeHeader("Subject", strings.TrimSpace(request.Subject))
	writeHeader("In-Reply-To", strings.TrimSpace(request.InReplyTo))
	if len(request.References) > 0 {
		writeHeader("References", strings.Join(compactStrings(request.References), " "))
	}
	writeHeader("MIME-Version", "1.0")

	hasText := strings.TrimSpace(request.TextBody) != ""
	hasHTML := strings.TrimSpace(request.HTMLBody) != ""

	if hasText && hasHTML {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)

		textPart, err := writer.CreatePart(textproto.MIMEHeader{
			"Content-Type":              []string{"text/plain; charset=UTF-8"},
			"Content-Transfer-Encoding": []string{"8bit"},
		})
		if err != nil {
			return "", fmt.Errorf("gmail: create text part: %w", err)
		}
		if _, err := textPart.Write([]byte(request.TextBody)); err != nil {
			return "", fmt.Errorf("gmail: write text part: %w", err)
		}

		htmlPart, err := writer.CreatePart(textproto.MIMEHeader{
			"Content-Type":              []string{"text/html; charset=UTF-8"},
			"Content-Transfer-Encoding": []string{"8bit"},
		})
		if err != nil {
			return "", fmt.Errorf("gmail: create html part: %w", err)
		}
		if _, err := htmlPart.Write([]byte(request.HTMLBody)); err != nil {
			return "", fmt.Errorf("gmail: write html part: %w", err)
		}

		if err := writer.Close(); err != nil {
			return "", fmt.Errorf("gmail: close multipart body: %w", err)
		}

		writeHeader("Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", writer.Boundary()))
		buffer.WriteString("\r\n")
		buffer.Write(body.Bytes())

		return base64.RawURLEncoding.EncodeToString(buffer.Bytes()), nil
	}

	contentType := "text/plain; charset=UTF-8"
	body := request.TextBody
	if !hasText {
		contentType = "text/html; charset=UTF-8"
		body = request.HTMLBody
	}
	writeHeader("Content-Type", contentType)
	buffer.WriteString("\r\n")
	buffer.WriteString(body)

	return base64.RawURLEncoding.EncodeToString(buffer.Bytes()), nil
}

func formatRecipients(recipients []Recipient) string {
	formatted := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		address, err := recipient.mailAddress()
		if err != nil {
			continue
		}
		formatted = append(formatted, address.String())
	}

	return strings.Join(formatted, ", ")
}

func (r Recipient) mailAddress() (*mail.Address, error) {
	return mail.ParseAddress((&mail.Address{
		Name:    strings.TrimSpace(r.Name),
		Address: strings.TrimSpace(r.Address),
	}).String())
}

func compactStrings(values []string) []string {
	compacted := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		compacted = append(compacted, value)
	}

	return compacted
}

func isSupportedFormat(format MessageFormat) bool {
	switch format {
	case MessageFormatFull, MessageFormatMetadata, MessageFormatMinimal, MessageFormatRaw:
		return true
	default:
		return false
	}
}

func toMessage(resource messageResource) (Message, error) {
	message := Message{
		ID:        resource.ID,
		ThreadID:  resource.ThreadID,
		HistoryID: resource.HistoryID,
		Snippet:   resource.Snippet,
		LabelIDs:  append([]string(nil), resource.LabelIDs...),
		Headers:   headerMap(resource.Payload),
		Raw:       resource.Raw,
	}

	if strings.TrimSpace(resource.InternalDate) != "" {
		milliseconds, err := strconv.ParseInt(strings.TrimSpace(resource.InternalDate), 10, 64)
		if err != nil {
			return Message{}, fmt.Errorf("gmail: parse internal date: %w", err)
		}
		message.InternalDate = time.UnixMilli(milliseconds).UTC()
	}

	if err := extractBodies(resource.Payload, &message); err != nil {
		return Message{}, err
	}
	extractAttachments(resource.Payload, &message)

	return message, nil
}

func headerMap(payload *messagePart) map[string]string {
	if payload == nil {
		return map[string]string{}
	}

	headers := make(map[string]string, len(payload.Headers))
	for _, header := range payload.Headers {
		name := strings.TrimSpace(header.Name)
		if name == "" {
			continue
		}
		headers[name] = strings.TrimSpace(header.Value)
	}

	return headers
}

func extractBodies(part *messagePart, message *Message) error {
	if part == nil {
		return nil
	}

	mimeType := strings.ToLower(strings.TrimSpace(part.MimeType))
	if mimeType == "text/plain" || mimeType == "text/html" {
		decoded, err := decodeBody(part.Body.Data)
		if err != nil {
			return err
		}
		if mimeType == "text/plain" && message.PlainTextBody == "" {
			message.PlainTextBody = decoded
		}
		if mimeType == "text/html" && message.HTMLBody == "" {
			message.HTMLBody = decoded
		}
	}

	for _, child := range part.Parts {
		if err := extractBodies(child, message); err != nil {
			return err
		}
	}

	return nil
}

func extractAttachments(part *messagePart, message *Message) {
	if part == nil {
		return
	}

	if filename := strings.TrimSpace(part.Filename); filename != "" {
		message.Attachments = append(message.Attachments, Attachment{
			PartID:       strings.TrimSpace(part.PartID),
			AttachmentID: strings.TrimSpace(part.Body.AttachmentID),
			Filename:     filename,
			MimeType:     strings.TrimSpace(part.MimeType),
			Size:         part.Body.Size,
			bodyData:     strings.TrimSpace(part.Body.Data),
		})
	}

	for _, child := range part.Parts {
		extractAttachments(child, message)
	}
}

func decodeBody(encoded string) (string, error) {
	decoded, err := decodeBodyBytes(encoded)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}

func decodeBodyBytes(encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, nil
	}

	if decoded, err := base64.RawURLEncoding.DecodeString(encoded); err == nil {
		return decoded, nil
	}
	decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("gmail: decode body: %w", err)
	}

	return decoded, nil
}

type listMessagesResource struct {
	Messages           []messageReferenceResource `json:"messages"`
	NextPageToken      string                     `json:"nextPageToken"`
	ResultSizeEstimate int64                      `json:"resultSizeEstimate"`
}

type messageReferenceResource struct {
	ID       string `json:"id"`
	ThreadID string `json:"threadId"`
}

type messageResource struct {
	ID           string       `json:"id"`
	ThreadID     string       `json:"threadId"`
	LabelIDs     []string     `json:"labelIds"`
	Snippet      string       `json:"snippet"`
	HistoryID    string       `json:"historyId"`
	InternalDate string       `json:"internalDate"`
	Raw          string       `json:"raw"`
	Payload      *messagePart `json:"payload"`
}

type sentMessageResource struct {
	ID       string   `json:"id"`
	ThreadID string   `json:"threadId"`
	LabelIDs []string `json:"labelIds"`
}

type messagePart struct {
	PartID   string          `json:"partId"`
	MimeType string          `json:"mimeType"`
	Filename string          `json:"filename"`
	Headers  []messageHeader `json:"headers"`
	Body     messageBody     `json:"body"`
	Parts    []*messagePart  `json:"parts"`
}

type messageHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type messageBody struct {
	Data         string `json:"data"`
	AttachmentID string `json:"attachmentId"`
	Size         int64  `json:"size"`
}

type attachmentResource struct {
	Data string `json:"data"`
	Size int64  `json:"size"`
}
