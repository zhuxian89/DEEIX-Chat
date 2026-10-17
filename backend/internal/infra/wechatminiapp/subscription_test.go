package wechatminiapp

import (
	"context"
	"encoding/json"
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSubscriptionUsesBoundTemplateCachedTokenAndBeijingDate(t *testing.T) {
	tokens, sends := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Error("unexpected request method")
		}
		if r.URL.Path == "/cgi-bin/stable_token" {
			tokens++
			var req map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req["appid"] != "app" || req["force_refresh"] != false {
				t.Error("wrong token request")
			}
			_, _ = w.Write([]byte(`{"access_token":"token","expires_in":7200}`))
			return
		}
		if r.URL.Path != "/cgi-bin/message/subscribe/send" || r.URL.Query().Get("access_token") != "token" {
			t.Error("wrong delivery endpoint")
		}
		sends++
		var req struct {
			Owner    string `json:"touser"`
			Template string `json:"template_id"`
			State    string `json:"miniprogram_state"`
			Data     map[string]struct {
				Value string `json:"value"`
			} `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Owner != "bound-owner" || req.Template != "configured-template" || req.State != "trial" || req.Data["time2"].Value != "2026-10-17 06:30" || len([]rune(req.Data["thing1"].Value)) != 20 {
			t.Errorf("invalid subscription payload: %+v", req)
		}
		_, _ = w.Write([]byte(`{"errcode":0}`))
	}))
	defer server.Close()
	client, err := NewSubscriptionClient(server.Client(), server.URL, func() (string, string) { return "app", "secret" }, "thing1", "time2", "trial")
	if err != nil {
		t.Fatal(err)
	}
	due, _ := time.Parse(time.RFC3339, "2026-10-16T22:30:00Z")
	delivery := domain.NotificationDelivery{OpenID: "bound-owner", TemplateID: "configured-template", Title: strings.Repeat("字", 30), DueAt: due}
	for i := 0; i < 2; i++ {
		if err := client.Send(t.Context(), delivery); err != nil {
			t.Fatal(err)
		}
	}
	if tokens != 1 || sends != 2 {
		t.Fatal("token cache not used")
	}
}

func TestSubscriptionFailuresAreSanitizedAndNeverAutomaticallyResent(t *testing.T) {
	for _, payload := range []string{`{"errcode":43101,"errmsg":"private-token"}`, `{}`, `not-json`} {
		t.Run(payload, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/cgi-bin/stable_token" {
					_, _ = w.Write([]byte(`{"access_token":"private-token","expires_in":7200}`))
					return
				}
				calls++
				_, _ = w.Write([]byte(payload))
			}))
			defer server.Close()
			client, _ := NewSubscriptionClient(server.Client(), server.URL, func() (string, string) { return "app", "private-secret" }, "thing1", "time2", "")
			err := client.Send(t.Context(), domain.NotificationDelivery{OpenID: "owner", TemplateID: "template", Title: "事项", DueAt: time.Now()})
			if err == nil || strings.Contains(err.Error(), "private-") || calls != 1 {
				t.Fatalf("unsafe failure or retry: %v calls=%d", err, calls)
			}
		})
	}
	if _, err := NewSubscriptionClient(nil, "", func() (string, string) { return "app", "secret" }, "title1", "time2", ""); err == nil {
		t.Fatal("incompatible template field accepted")
	}
	client, _ := NewSubscriptionClient(nil, "", func() (string, string) { return "", "" }, "thing1", "time2", "")
	if err := client.Send(context.Background(), domain.NotificationDelivery{OpenID: "owner", TemplateID: "template", Title: "事项", DueAt: time.Now()}); err == nil {
		t.Fatal("missing credentials accepted")
	}
}

func TestSubscriptionRejectsChangedAppBeforeAnyNetworkRequest(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer server.Close()
	client, err := NewSubscriptionClient(server.Client(), server.URL, func() (string, string) { return "old-app", "secret" }, "thing5", "time30", "formal")
	if err != nil {
		t.Fatal(err)
	}
	err = client.Send(t.Context(), domain.NotificationDelivery{AppID: "new-app", OpenID: "owner", TemplateID: "template", Title: "reminder", DueAt: time.Now()})
	if err == nil || calls != 0 {
		t.Fatal("changed account reached notification transport")
	}
}
