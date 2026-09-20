package telegram

import (
	"strings"
	"testing"
)

func TestRegistrationNotificationSource(t *testing.T) {
	for _, tt := range []struct {
		name       string
		email      string
		miniapp    bool
		wantHeader string
	}{
		{name: "website", email: "alice@example.com", wantHeader: "🔔 Web 新用户注册 (#7)"},
		{name: "website without email", wantHeader: "🔔 Web 新用户注册 (#7)"},
		{name: "miniapp", miniapp: true, wantHeader: "🔔 小程序 新用户注册 (#7)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var captured string
			server := newTestServer(t, &captured)
			defer server.Close()
			settings := &fakeSettings{values: map[string]string{KeyEnabled: "true", KeyBotToken: "bot-token", KeyChatID: "12345"}}
			notifier := NewNotifier(settings, NewClient(server.Client(), server.URL), nil)
			if tt.miniapp {
				notifier.NotifyMiniAppRegistration(7, "alice")
			} else {
				notifier.NotifyRegistration(7, "alice", tt.email)
			}
			notifier.Close()
			if !strings.HasPrefix(captured, tt.wantHeader+"\n") || !strings.Contains(captured, "用户名: alice\n") || !strings.Contains(captured, "时间: ") {
				t.Fatalf("unexpected registration notification: %q", captured)
			}
			if tt.email != "" && !strings.Contains(captured, "邮箱: "+tt.email+"\n") {
				t.Fatalf("missing website email: %q", captured)
			}
			if tt.email == "" && strings.Contains(captured, "邮箱:") {
				t.Fatalf("empty email should be omitted: %q", captured)
			}
		})
	}
}
