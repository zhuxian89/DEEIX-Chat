package telegram

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type fakeSettings struct {
	mu     sync.Mutex
	values map[string]string
}

func (f *fakeSettings) RuntimeValuesByNamespace(_ context.Context, namespace string) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make(map[string]string, len(f.values))
	for key, value := range f.values {
		result[key] = value
	}
	return result, nil
}

func newTestServer(t *testing.T, captured *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/bot") || !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form failed: %v", err)
		}
		*captured = r.FormValue("text")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
}

func TestSendMessageRejectsTelegramAPIErrorWithHTTP200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"description":"chat not found"}`))
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL)
	err := client.SendMessage(context.Background(), Config{BotToken: "bot-token", ChatID: "12345"}, "hello")
	if err == nil || !strings.Contains(err.Error(), "chat not found") {
		t.Fatalf("expected Telegram API error, got %v", err)
	}
}

func TestSendMessagePostsChatIDAndText(t *testing.T) {
	var captured string
	server := newTestServer(t, &captured)
	defer server.Close()

	client := NewClient(server.Client(), server.URL)
	err := client.SendMessage(context.Background(), Config{BotToken: "bot-token", ChatID: "12345"}, "你好")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(captured, "你好") {
		t.Fatalf("expected text to contain message, got %q", captured)
	}
}

func TestSendMessageRequiresConfig(t *testing.T) {
	client := NewClient(nil, "")
	if err := client.SendMessage(context.Background(), Config{}, "x"); err == nil {
		t.Fatal("expected error when config is missing")
	}
}

func TestSendMessageDoesNotExposeBotTokenInTransportError(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	httpClient := server.Client()
	httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New(req.URL.String())
	})

	client := NewClient(httpClient, server.URL)
	const botToken = "secret-bot-token"
	err := client.SendMessage(context.Background(), Config{BotToken: botToken, ChatID: "12345"}, "hello")
	if err == nil {
		t.Fatal("expected transport error")
	}
	if strings.Contains(err.Error(), botToken) {
		t.Fatalf("expected bot token to be redacted from error, got %q", err)
	}
}

func TestNotifySkipsWhenDisabled(t *testing.T) {
	var captured string
	server := newTestServer(t, &captured)
	defer server.Close()

	settings := &fakeSettings{values: map[string]string{
		KeyEnabled:  "false",
		KeyBotToken: "bot-token",
		KeyChatID:   "12345",
	}}
	notifier := NewNotifier(settings, NewClient(server.Client(), server.URL), nil)
	notifier.NotifyRegistration(1, "alice", "alice@example.com")
	notifier.Close()
	if captured != "" {
		t.Fatalf("expected no send when disabled, got %q", captured)
	}
}

func TestNotifyDefaultsToEnabledWhenSwitchIsMissing(t *testing.T) {
	var captured string
	server := newTestServer(t, &captured)
	defer server.Close()

	settings := &fakeSettings{values: map[string]string{
		KeyBotToken: "bot-token",
		KeyChatID:   "12345",
	}}
	notifier := NewNotifier(settings, NewClient(server.Client(), server.URL), nil)
	notifier.NotifyRegistration(1, "alice", "alice@example.com")
	notifier.Close()
	if !strings.Contains(captured, "alice") {
		t.Fatalf("expected notification when enabled switch is missing, got %q", captured)
	}
}

func TestNotifySendsWhenEnabled(t *testing.T) {
	var captured string
	server := newTestServer(t, &captured)
	defer server.Close()

	settings := &fakeSettings{values: map[string]string{
		KeyEnabled:  "true",
		KeyBotToken: "bot-token",
		KeyChatID:   "12345",
	}}
	notifier := NewNotifier(settings, NewClient(server.Client(), server.URL), nil)
	notifier.NotifyRegistration(7, "alice", "alice@example.com")
	notifier.Close()
	if !strings.Contains(captured, "alice") || !strings.Contains(captured, "alice@example.com") {
		t.Fatalf("expected registration details in message, got %q", captured)
	}
}

func TestNotifyWeChatMessageMirrorsContent(t *testing.T) {
	var captured string
	server := newTestServer(t, &captured)
	defer server.Close()

	settings := &fakeSettings{values: map[string]string{
		KeyEnabled:  "true",
		KeyBotToken: "bot-token",
		KeyChatID:   "12345",
	}}
	notifier := NewNotifier(settings, NewClient(server.Client(), server.URL), nil)
	notifier.NotifyWeChatMessage("openid-1", "text", "你的专属注册码：REG-XXXX")
	notifier.Close()
	if !strings.Contains(captured, "openid-1") || !strings.Contains(captured, "REG-XXXX") {
		t.Fatalf("expected wechat message mirrored, got %q", captured)
	}
}

func TestNotifyWeChatReplyMirrorsReply(t *testing.T) {
	var captured string
	server := newTestServer(t, &captured)
	defer server.Close()

	settings := &fakeSettings{values: map[string]string{
		KeyEnabled:  "true",
		KeyBotToken: "bot-token",
		KeyChatID:   "12345",
	}}
	notifier := NewNotifier(settings, NewClient(server.Client(), server.URL), nil)
	notifier.NotifyWeChatReply("openid-1", "回复内容")
	notifier.Close()
	if !strings.Contains(captured, "微信公众号回复") || !strings.Contains(captured, "回复内容") {
		t.Fatalf("expected wechat reply mirrored, got %q", captured)
	}
}

func TestNotifySkipsWhenNotConfigured(t *testing.T) {
	var captured string
	server := newTestServer(t, &captured)
	defer server.Close()

	settings := &fakeSettings{values: map[string]string{KeyEnabled: "true"}}
	notifier := NewNotifier(settings, NewClient(server.Client(), server.URL), nil)
	notifier.NotifyRegistration(1, "alice", "alice@example.com")
	notifier.Close()
	if captured != "" {
		t.Fatalf("expected no send when bot token missing, got %q", captured)
	}
}
