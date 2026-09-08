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

	domainspeech "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/speech"
	speechport "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/speech"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (r roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return r(req) }

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
