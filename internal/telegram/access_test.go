package telegram

import "testing"

func TestAccessControlAllowsOnlyConfiguredUserAndChat(t *testing.T) {
	t.Parallel()

	access := AccessControl{
		AllowedUserID: 42,
		AllowedChatID: -1001,
	}

	if !access.Allows(42, -1001) {
		t.Fatal("Allows() = false, want true for configured user/chat")
	}

	for _, tc := range []struct {
		name   string
		userID int64
		chatID int64
	}{
		{name: "wrong user", userID: 7, chatID: -1001},
		{name: "wrong chat", userID: 42, chatID: 99},
		{name: "wrong user and chat", userID: 7, chatID: 99},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if access.Allows(tc.userID, tc.chatID) {
				t.Fatalf("Allows(%d, %d) = true, want false", tc.userID, tc.chatID)
			}
		})
	}
}
