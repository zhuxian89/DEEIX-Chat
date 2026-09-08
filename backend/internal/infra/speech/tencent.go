package speech

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	domainspeech "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/speech"
	speechport "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/speech"
)

const endpoint = "asr.tencentcloudapi.com"

type Tencent struct{ client *http.Client }

func NewTencent() *Tencent {
	return &Tencent{client: &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func (t *Tencent) Recognize(ctx context.Context, cfg domainspeech.Config, audio []byte) (speechport.Transcript, error) {
	if len(audio) == 0 || len(audio) > speechport.MaxAudioBytes {
		return speechport.Transcript{}, speechport.ErrInvalidAudio
	}
	payload, err := json.Marshal(struct {
		Engine string `json:"EngSerViceType"`
		Source int    `json:"SourceType"`
		Format string `json:"VoiceFormat"`
		Data   string `json:"Data"`
		Length int    `json:"DataLen"`
	}{
		Engine: cfg.Engine,
		Source: 1,
		Format: "mp3",
		Data:   base64.StdEncoding.EncodeToString(audio),
		Length: len(audio),
	})
	if err != nil {
		return speechport.Transcript{}, speechport.ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+endpoint+"/", bytes.NewReader(payload))
	if err != nil {
		return speechport.Transcript{}, speechport.ErrUnavailable
	}
	sign(req, cfg, payload, time.Now())
	resp, err := t.client.Do(req)
	if err != nil {
		return speechport.Transcript{}, speechport.ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return speechport.Transcript{}, speechport.ErrUnavailable
	}
	var body struct {
		Response struct {
			Result        string
			AudioDuration int
			Error         *struct{ Code string }
		}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128*1024)).Decode(&body); err != nil {
		return speechport.Transcript{}, speechport.ErrUnavailable
	}
	if body.Response.Error != nil {
		// Do not forward vendor messages: they can contain submitted audio or credentials.
		code := body.Response.Error.Code
		switch {
		case strings.Contains(code, "NoVoice"), strings.Contains(code, "NoSpeech"):
			return speechport.Transcript{}, speechport.ErrNoSpeech
		case strings.Contains(code, "ResourcePack"), strings.Contains(code, "Arrears"), strings.HasPrefix(code, "ResourcesSoldOut"):
			return speechport.Transcript{}, speechport.ErrQuota
		case strings.HasPrefix(code, "LimitExceeded"), strings.HasPrefix(code, "RequestLimitExceeded"):
			return speechport.Transcript{}, speechport.ErrBusy
		case strings.Contains(code, "Voicedata"), strings.Contains(code, "VoiceFormat"), strings.Contains(code, "Audio"):
			return speechport.Transcript{}, speechport.ErrInvalidAudio
		default:
			return speechport.Transcript{}, speechport.ErrUnavailable
		}
	}
	return speechport.Transcript{Text: body.Response.Result, DurationMS: body.Response.AudioDuration}, nil
}

// Tencent Cloud API v3 signing. The destination is fixed; clients cannot supply
// a URL, provider, engine or credentials in a transcription request.
func sign(req *http.Request, cfg domainspeech.Config, payload []byte, now time.Time) {
	date := now.UTC().Format("2006-01-02")
	timestamp := strconv.FormatInt(now.Unix(), 10)
	contentType := "application/json; charset=utf-8"
	canonical := "POST\n/\n\ncontent-type:" + contentType + "\nhost:" + endpoint + "\n\ncontent-type;host\n" + hash(payload)
	scope := date + "/asr/tc3_request"
	toSign := "TC3-HMAC-SHA256\n" + timestamp + "\n" + scope + "\n" + hash([]byte(canonical))
	dateKey := mac([]byte("TC3"+cfg.SecretKey), date)
	serviceKey := mac(dateKey, "asr")
	signingKey := mac(serviceKey, "tc3_request")
	authorization := fmt.Sprintf("TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=content-type;host, Signature=%s", cfg.SecretID, scope, hex.EncodeToString(mac(signingKey, toSign)))
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", authorization)
	req.Header.Set("X-TC-Action", "SentenceRecognition")
	req.Header.Set("X-TC-Version", "2019-06-14")
	req.Header.Set("X-TC-Timestamp", timestamp)
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func mac(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(value))
	return h.Sum(nil)
}
