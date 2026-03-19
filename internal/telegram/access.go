package telegram

// AccessControl is the future boundary for single-user/single-chat authorization checks.
type AccessControl struct {
	AllowedUserID int64
	AllowedChatID int64
}

func (a AccessControl) Allows(userID, chatID int64) bool {
	return userID == a.AllowedUserID && chatID == a.AllowedChatID
}
