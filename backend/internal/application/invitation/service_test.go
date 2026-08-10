package invitation

import (
	"strings"
	"testing"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
)

// TestBuildInviteLinkPointsToLoginRoute 锁定邀请链接指向 /login（注册是其 mode），
// 并携带 invite + mode=register，避免回归到不存在的 /register 路由。
func TestBuildInviteLinkPointsToLoginRoute(t *testing.T) {
	tests := []struct {
		name     string
		webBase  string
		code     string
		wantHas  []string
		notWant  string
	}{
		{
			name:    "with public web base url",
			webBase: "https://chat.example.com",
			code:    "INV-ABCD123",
			wantHas: []string{"https://chat.example.com/login?invite=INV-ABCD123&mode=register"},
			notWant: "/register",
		},
		{
			name:    "without public web base url (relative)",
			webBase: "",
			code:    "INV-XYZ789",
			wantHas: []string{"/login?invite=INV-XYZ789&mode=register"},
			notWant: "/register",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewRuntime(config.Config{PublicWebBaseURL: tt.webBase})
			s := NewService(nil, cfg)
			got := s.buildInviteLink(tt.code)
			for _, want := range tt.wantHas {
				if got != want {
					t.Fatalf("buildInviteLink() = %q, want %q", got, want)
				}
			}
			if strings.Contains(got, tt.notWant) {
				t.Fatalf("buildInviteLink() = %q, must not contain %q (no /register route)", got, tt.notWant)
			}
		})
	}
}
