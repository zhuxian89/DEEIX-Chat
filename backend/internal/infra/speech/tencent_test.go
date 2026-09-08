package speech

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	domainspeech "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/speech"
	speechport "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/speech"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/security"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (r roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return r(req) }

func TestTencentRejectsPrivateOutboundTargets(t *testing.T) {
	client := NewTencent().client
	t.Cleanup(client.CloseIdleConnections)
	if client.Timeout != 20*time.Second {
		t.Fatal("speech requests must retain their timeout")
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.DialContext == nil {
		t.Fatal("speech requests must use the managed outbound transport")
	}
	if transport.Proxy != nil {
		t.Fatal("speech requests must not bypass the outbound policy through an environment proxy")
	}
	for _, address := range []string{"127.0.0.1:443", "10.0.0.1:443", "[::1]:443", "169.254.169.254:80"} {
		t.Run(address, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			conn, err := transport.DialContext(ctx, "tcp", address)
			if conn != nil {
				conn.Close()
			}
			if !errors.Is(err, security.ErrUnsafeOutboundURL) {
				t.Fatalf("unsafe address was not rejected by the outbound policy: %v", err)
			}
		})
	}
}

func TestTencentDoesNotFollowRedirects(t *testing.T) {
	for _, status := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := NewTencent()
			calls := 0
			client.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.String() != "https://asr.tencentcloudapi.com/" {
					t.Fatal("speech request followed a redirect to another endpoint")
				}
				return &http.Response{
					StatusCode: status,
					Header:     http.Header{"Location": {"https://redirect.example/"}},
					Body:       io.NopCloser(strings.NewReader("")),
					Request:    req,
				}, nil
			})
			_, err := client.Recognize(context.Background(), domainspeech.Config{}, []byte("recording"))
			if calls != 1 || !errors.Is(err, speechport.ErrUnavailable) {
				t.Fatalf("redirect must fail without retrying: calls=%d, err=%v", calls, err)
			}
		})
	}
}

func TestTencentUsesSignedPrivateAudioUpload(t *testing.T) {
	client := NewTencent()
	client.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://asr.tencentcloudapi.com/" || req.Header.Get("X-TC-Action") != "SentenceRecognition" || req.Header.Get("X-TC-Version") != "2019-06-14" {
			t.Fatal("wrong API")
		}
		if !strings.HasPrefix(req.Header.Get("Authorization"), "TC3-HMAC-SHA256 Credential=test-id/") || strings.Contains(req.Header.Get("Authorization"), "test-secret") {
			t.Fatal("invalid authorization")
		}
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["Data"] != base64.StdEncoding.EncodeToString([]byte("recording")) || payload["DataLen"] != float64(9) || payload["VoiceFormat"] != "mp3" || payload["SourceType"] != float64(1) || payload["EngSerViceType"] != "16k_zh" {
			t.Fatal("incorrect audio encoding")
		}
		if _, ok := payload["Url"]; ok {
			t.Fatal("audio exposed through URL")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"Response":{"Result":"hello","AudioDuration":1000}}`))}, nil
	})
	result, err := client.Recognize(context.Background(), domainspeech.Config{SecretID: "test-id", SecretKey: "test-secret", Engine: "16k_zh"}, []byte("recording"))
	if err != nil || result.Text != "hello" {
		t.Fatal(result, err)
	}
}

func TestTencentDoesNotExposeVendorErrors(t *testing.T) {
	for _, code := range []string{"AuthFailure.SecretIdNotFound", "FailedOperation.UserHasNoValidResource", "LimitExceeded", "InvalidParameterValue.ErrorInvalidVoicedata"} {
		t.Run(code, func(t *testing.T) {
			client := NewTencent()
			client.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"Response":{"Error":{"Code":"` + code + `","Message":"private-audio private-secret"}}}`))}, nil
			})
			_, err := client.Recognize(context.Background(), domainspeech.Config{}, []byte("mp3"))
			if err == nil || strings.Contains(err.Error(), "private") {
				t.Fatal("vendor details exposed")
			}
			if code == "LimitExceeded" && !errors.Is(err, speechport.ErrBusy) {
				t.Fatal(err)
			}
		})
	}
}
