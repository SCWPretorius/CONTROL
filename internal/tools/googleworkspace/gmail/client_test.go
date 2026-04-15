package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/SCWPretorius/CONTROL/internal/config"
)

func TestSearchMessagesBuildsExpectedRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.URL.Path, "/users/me/messages"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := request.Header.Get("Authorization"), "Bearer token-123"; got != want {
			t.Fatalf("Authorization = %q, want %q", got, want)
		}

		query := request.URL.Query()
		if got, want := query.Get("q"), "from:boss newer_than:7d"; got != want {
			t.Fatalf("q = %q, want %q", got, want)
		}
		if got, want := query.Get("maxResults"), "10"; got != want {
			t.Fatalf("maxResults = %q, want %q", got, want)
		}
		if got, want := query.Get("includeSpamTrash"), "true"; got != want {
			t.Fatalf("includeSpamTrash = %q, want %q", got, want)
		}
		if got, want := request.URL.Query()["labelIds"], []string{"INBOX", "STARRED"}; strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("labelIds = %#v, want %#v", got, want)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"messages":[{"id":"m-1","threadId":"t-1"}],"nextPageToken":"next","resultSizeEstimate":99}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	response, err := client.SearchMessages(context.Background(), SearchMessagesRequest{
		Query:            "from:boss newer_than:7d",
		LabelIDs:         []string{"INBOX", "STARRED"},
		MaxResults:       10,
		IncludeSpamTrash: true,
	})
	if err != nil {
		t.Fatalf("SearchMessages() error = %v", err)
	}

	if got, want := len(response.Messages), 1; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}
	if got, want := response.Messages[0].ID, "m-1"; got != want {
		t.Fatalf("Messages[0].ID = %q, want %q", got, want)
	}
	if got, want := response.NextPageToken, "next"; got != want {
		t.Fatalf("NextPageToken = %q, want %q", got, want)
	}
	if got, want := response.ResultSizeEstimate, int64(99); got != want {
		t.Fatalf("ResultSizeEstimate = %d, want %d", got, want)
	}
}

func TestGetMessageParsesHeadersBodiesAndInternalDate(t *testing.T) {
	t.Parallel()

	plainText := base64.RawURLEncoding.EncodeToString([]byte("hello from plain text"))
	htmlText := base64.RawURLEncoding.EncodeToString([]byte("<p>hello from html</p>"))
	attachmentData := base64.RawURLEncoding.EncodeToString([]byte("PDF!"))

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.URL.Path, "/users/me/messages/msg-123"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := request.URL.Query().Get("format"), "full"; got != want {
			t.Fatalf("format = %q, want %q", got, want)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id":"msg-123",
			"threadId":"thread-9",
			"historyId":"77",
			"snippet":"assistant snippet",
			"labelIds":["INBOX"],
			"internalDate":"1735732800000",
			"payload":{
				"mimeType":"multipart/alternative",
				"headers":[
					{"name":"Subject","value":"Status update"},
					{"name":"From","value":"Boss <boss@example.com>"}
				],
				"parts":[
					{"mimeType":"text/plain","body":{"data":"` + plainText + `"}},
					{"mimeType":"text/html","body":{"data":"` + htmlText + `"}},
					{"partId":"3","mimeType":"application/pdf","filename":"report.pdf","body":{"attachmentId":"att-1","data":"` + attachmentData + `","size":4}}
				]
			}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	message, err := client.GetMessage(context.Background(), GetMessageRequest{MessageID: "msg-123"})
	if err != nil {
		t.Fatalf("GetMessage() error = %v", err)
	}

	if got, want := message.Headers["Subject"], "Status update"; got != want {
		t.Fatalf("Subject = %q, want %q", got, want)
	}
	if got, want := message.Headers["From"], "Boss <boss@example.com>"; got != want {
		t.Fatalf("From = %q, want %q", got, want)
	}
	if got, want := message.PlainTextBody, "hello from plain text"; got != want {
		t.Fatalf("PlainTextBody = %q, want %q", got, want)
	}
	if got, want := message.HTMLBody, "<p>hello from html</p>"; got != want {
		t.Fatalf("HTMLBody = %q, want %q", got, want)
	}
	if got, want := len(message.Attachments), 1; got != want {
		t.Fatalf("len(Attachments) = %d, want %d", got, want)
	}
	if got, want := message.Attachments[0].Filename, "report.pdf"; got != want {
		t.Fatalf("Attachments[0].Filename = %q, want %q", got, want)
	}
	if got, want := message.Attachments[0].AttachmentID, "att-1"; got != want {
		t.Fatalf("Attachments[0].AttachmentID = %q, want %q", got, want)
	}

	wantTime := time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)
	if got := message.InternalDate; !got.Equal(wantTime) {
		t.Fatalf("InternalDate = %v, want %v", got, wantTime)
	}
}

func TestDownloadAttachmentFetchesAttachmentBody(t *testing.T) {
	t.Parallel()

	attachmentData := base64.RawURLEncoding.EncodeToString([]byte("file-bytes"))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.URL.Path, "/users/me/messages/msg-123/attachments/att-9"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":"` + attachmentData + `","size":10}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	content, err := client.DownloadAttachment(context.Background(), DownloadAttachmentRequest{
		MessageID:  "msg-123",
		Attachment: Attachment{AttachmentID: "att-9"},
	})
	if err != nil {
		t.Fatalf("DownloadAttachment() error = %v", err)
	}
	if got, want := string(content), "file-bytes"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestDownloadAttachmentUsesInlineAttachmentData(t *testing.T) {
	t.Parallel()

	attachmentData := base64.RawURLEncoding.EncodeToString([]byte("inline"))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id":"msg-inline",
			"threadId":"thread-1",
			"labelIds":["INBOX"],
			"payload":{
				"mimeType":"multipart/mixed",
				"parts":[
					{"partId":"2","mimeType":"application/octet-stream","filename":"payload.bin","body":{"data":"` + attachmentData + `","size":6}}
				]
			}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	message, err := client.GetMessage(context.Background(), GetMessageRequest{MessageID: "msg-inline"})
	if err != nil {
		t.Fatalf("GetMessage() error = %v", err)
	}
	if got, want := len(message.Attachments), 1; got != want {
		t.Fatalf("len(Attachments) = %d, want %d", got, want)
	}

	content, err := client.DownloadAttachment(context.Background(), DownloadAttachmentRequest{
		MessageID:  message.ID,
		Attachment: message.Attachments[0],
	})
	if err != nil {
		t.Fatalf("DownloadAttachment() error = %v", err)
	}
	if got, want := string(content), "inline"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestSendMessageBuildsRawRFC822Payload(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Method, http.MethodPost; got != want {
			t.Fatalf("method = %q, want %q", got, want)
		}
		if got, want := request.URL.Path, "/users/me/messages/send"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}

		var payload map[string]string
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		raw := payload["raw"]
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			t.Fatalf("DecodeString() error = %v", err)
		}
		message := string(decoded)
		if !strings.Contains(message, "To: \"Product Lead\" <lead@example.com>\r\n") {
			t.Fatalf("raw message missing To header: %q", message)
		}
		if !strings.Contains(message, "Subject: Weekly review\r\n") {
			t.Fatalf("raw message missing Subject header: %q", message)
		}
		if !strings.Contains(message, "multipart/alternative") {
			t.Fatalf("raw message missing multipart content type: %q", message)
		}
		if !strings.Contains(message, "Plain body") || !strings.Contains(message, "<p>HTML body</p>") {
			t.Fatalf("raw message missing bodies: %q", message)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"sent-1","threadId":"thread-22","labelIds":["SENT"]}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	sent, err := client.SendMessage(context.Background(), SendMessageRequest{
		To: []Recipient{
			{Name: "Product Lead", Address: "lead@example.com"},
		},
		Subject:  "Weekly review",
		TextBody: "Plain body",
		HTMLBody: "<p>HTML body</p>",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	if got, want := sent.ID, "sent-1"; got != want {
		t.Fatalf("ID = %q, want %q", got, want)
	}
	if got, want := sent.ThreadID, "thread-22"; got != want {
		t.Fatalf("ThreadID = %q, want %q", got, want)
	}
}

func TestSendMessageValidatesRequiredFields(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, "https://example.invalid")
	_, err := client.SendMessage(context.Background(), SendMessageRequest{
		To: []Recipient{{Address: "lead@example.com"}},
	})
	if err == nil || !strings.Contains(err.Error(), "text or html body is required") {
		t.Fatalf("SendMessage() error = %v, want body validation error", err)
	}
}

func TestSearchMessagesRequiresQuery(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, "https://example.invalid")
	_, err := client.SearchMessages(context.Background(), SearchMessagesRequest{})
	if err == nil || !strings.Contains(err.Error(), "search query is required") {
		t.Fatalf("SearchMessages() error = %v, want query validation error", err)
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

func TestRecipientFormatsAddress(t *testing.T) {
	t.Parallel()

	address, err := (Recipient{Name: "Name", Address: "name@example.com"}).mailAddress()
	if err != nil {
		t.Fatalf("mailAddress() error = %v", err)
	}
	if got, want := address.String(), "\"Name\" <name@example.com>"; got != want {
		t.Fatalf("address.String() = %q, want %q", got, want)
	}
}

func TestBuildRawMessageCompactsReferences(t *testing.T) {
	t.Parallel()

	raw, err := buildRawMessage(SendMessageRequest{
		To:       []Recipient{{Address: "lead@example.com"}},
		TextBody: "body",
		References: []string{
			"  <one@example.com> ",
			"",
			"<two@example.com>",
		},
	})
	if err != nil {
		t.Fatalf("buildRawMessage() error = %v", err)
	}

	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	if got := string(decoded); !strings.Contains(got, "References: <one@example.com> <two@example.com>\r\n") {
		t.Fatalf("decoded raw = %q, want compacted References header", got)
	}
}

func TestListMessagesRejectsNegativeMaxResults(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, (&url.URL{Scheme: "https", Host: "example.invalid"}).String())
	_, err := client.ListMessages(context.Background(), ListMessagesRequest{MaxResults: -1})
	if err == nil || !strings.Contains(err.Error(), "max results must be zero or greater") {
		t.Fatalf("ListMessages() error = %v, want max-results validation error", err)
	}
}
