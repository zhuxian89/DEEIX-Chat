package speech

import (
	"context"
	"errors"
	"testing"
	"time"

	domainspeech "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/speech"
	speechport "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/speech"
)

type settingsStub map[string]string

func (s settingsStub) RuntimeValuesByNamespace(context.Context, string) (map[string]string, error) {
	return s, nil
}

type recognizerFunc func(context.Context, domainspeech.Config, []byte) (speechport.Transcript, error)

func (r recognizerFunc) Recognize(ctx context.Context, cfg domainspeech.Config, audio []byte) (speechport.Transcript, error) {
	return r(ctx, cfg, audio)
}
func configured() settingsStub {
	return settingsStub{"enabled": "true", "tencent_secret_id": "id", "tencent_secret_key": "key"}
}

func TestSpeechReadsCurrentSettingsForEachRequest(t *testing.T) {
	settings := configured()
	var seen []domainspeech.Config
	s := NewService(settings, recognizerFunc(func(_ context.Context, cfg domainspeech.Config, _ []byte) (speechport.Transcript, error) {
		seen = append(seen, cfg)
		return speechport.Transcript{Text: " recognized ", DurationMS: 1000}, nil
	}), nil)
	if result, err := s.Transcribe(context.Background(), 1, []byte("mp3")); err != nil || result.Text != "recognized" {
		t.Fatal(result, err)
	}
	settings["tencent_secret_key"] = "new-key"
	settings["engine"] = "16k_zh-PY"
	if _, err := s.Transcribe(context.Background(), 1, []byte("mp3")); err != nil {
		t.Fatal(err)
	}
	if seen[1].SecretKey != "new-key" || seen[1].Engine != "16k_zh-PY" {
		t.Fatal("stale configuration")
	}
	settings["enabled"] = "false"
	if s.Enabled(context.Background()) {
		t.Fatal("disabled config still enabled")
	}
	if _, err := s.Transcribe(context.Background(), 1, []byte("mp3")); !errors.Is(err, speechport.ErrUnavailable) {
		t.Fatal(err)
	}
	if len(seen) != 2 {
		t.Fatal("disabled request reached provider")
	}
}

func TestSpeechRejectsOverlapAndReleasesSlotOnCancellation(t *testing.T) {
	started := make(chan struct{})
	s := NewService(configured(), recognizerFunc(func(ctx context.Context, _ domainspeech.Config, _ []byte) (speechport.Transcript, error) {
		close(started)
		<-ctx.Done()
		return speechport.Transcript{}, ctx.Err()
	}), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); _, _ = s.Transcribe(ctx, 5, []byte("mp3")) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("provider did not start")
	}
	if _, err := s.Transcribe(context.Background(), 5, []byte("mp3")); !errors.Is(err, speechport.ErrBusy) {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation did not return")
	}
	s.mu.Lock()
	remaining := len(s.inFlight)
	s.mu.Unlock()
	if remaining != 0 {
		t.Fatal("slot leaked")
	}
}

func TestSpeechRejectsOversizedAudioAndEmptyRecognition(t *testing.T) {
	calls := 0
	s := NewService(configured(), recognizerFunc(func(context.Context, domainspeech.Config, []byte) (speechport.Transcript, error) {
		calls++
		return speechport.Transcript{Text: " "}, nil
	}), nil)
	for _, audio := range [][]byte{nil, make([]byte, speechport.MaxAudioBytes+1)} {
		if _, err := s.Transcribe(context.Background(), 1, audio); !errors.Is(err, speechport.ErrInvalidAudio) {
			t.Fatal(err)
		}
	}
	if calls != 0 {
		t.Fatal("invalid upload reached provider")
	}
	if _, err := s.Transcribe(context.Background(), 1, []byte("mp3")); !errors.Is(err, speechport.ErrNoSpeech) {
		t.Fatal(err)
	}
}
