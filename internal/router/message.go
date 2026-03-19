package router

// MessageMetadata carries lightweight sender/chat details that transports can pass
// through for persistence and observability without leaking transport SDK types
// into the orchestration layer.
type MessageMetadata struct {
	ChatTitle    string
	Username     string
	FirstName    string
	LastName     string
	LanguageCode string
}

// Message is the normalized input contract between transports and Copilot sessions.
type Message struct {
	Transport string
	MessageID int
	ChatID    int64
	UserID    int64
	Text      string
	Metadata  MessageMetadata
}
