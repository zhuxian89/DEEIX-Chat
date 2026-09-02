package wechatminiapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExchangeReturnsOpenIDAndUnionID(t *testing.T) {
	var query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"openid":"openid-1","unionid":"union-1","session_key":"must-not-return"}`))
	}))
	defer server.Close()

	identity, err := NewClient(server.Client(), server.URL).Exchange(context.Background(), "wx-app", "secret-value", "code-value")
	if err != nil {
		t.Fatalf("Exchange() error = %v", err)
	}
	if identity.OpenID != "openid-1" || identity.UnionID != "union-1" {
		t.Fatalf("identity = %+v", identity)
	}
	if !strings.Contains(query, "appid=wx-app") || !strings.Contains(query, "grant_type=authorization_code") {
		t.Fatalf("query = %q", query)
	}
}

func TestExchangeSanitizesTransportError(t *testing.T) {
	client := NewClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, &secretError{text: "secret-value code-value"}
	})}, "https://api.weixin.qq.com")
	_, err := client.Exchange(context.Background(), "wx-app", "secret-value", "code-value")
	if err == nil || strings.Contains(err.Error(), "secret-value") || strings.Contains(err.Error(), "code-value") {
		t.Fatalf("error was not sanitized: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type secretError struct{ text string }

func (e *secretError) Error() string { return e.text }
