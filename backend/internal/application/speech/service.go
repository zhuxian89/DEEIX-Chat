package speech

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	domainspeech "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/speech"
	speechport "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/speech"
)

type Settings interface {
	RuntimeValuesByNamespace(context.Context, string) (map[string]string, error)
}

type Limiter interface {
	AllowSlidingWindow(context.Context, string, int, time.Duration, time.Duration) (bool, error)
}

type Service struct {
	settings   Settings
	recognizer speechport.Recognizer
	limiter    Limiter
	mu         sync.Mutex
	inFlight   map[uint]bool
}

func NewService(settings Settings, recognizer speechport.Recognizer, limiter Limiter) *Service {
	return &Service{
		settings:   settings,
		recognizer: recognizer,
		limiter:    limiter,
		inFlight:   make(map[uint]bool),
	}
}

func (s *Service) config(ctx context.Context) (domainspeech.Config, error) {
	values, err := s.settings.RuntimeValuesByNamespace(ctx, domainspeech.Namespace)
	if err != nil {
		return domainspeech.Config{}, speechport.ErrUnavailable
	}
	cfg, err := domainspeech.Parse(values)
	if err != nil {
		return domainspeech.Config{}, speechport.ErrUnavailable
	}
	return cfg, nil
}

func (s *Service) Enabled(ctx context.Context) bool {
	cfg, err := s.config(ctx)
	return err == nil && cfg.Enabled
}

// Recognition has no conversation, message, memory or billing writes. Only text
// explicitly sent by the user enters the existing chat/image generation flows.
func (s *Service) Transcribe(ctx context.Context, userID uint, audio []byte) (speechport.Transcript, error) {
	if userID == 0 || len(audio) == 0 || len(audio) > speechport.MaxAudioBytes {
		return speechport.Transcript{}, speechport.ErrInvalidAudio
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cfg, err := s.config(ctx)
	if err != nil || !cfg.Enabled || s.recognizer == nil {
		return speechport.Transcript{}, speechport.ErrUnavailable
	}
	s.mu.Lock()
	if s.inFlight[userID] || len(s.inFlight) >= 4 {
		s.mu.Unlock()
		return speechport.Transcript{}, speechport.ErrBusy
	}
	s.inFlight[userID] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.inFlight, userID)
		s.mu.Unlock()
	}()
	if s.limiter != nil {
		for _, limit := range []struct {
			key    string
			count  int
			window time.Duration
		}{
			{fmt.Sprintf("speech:user:%d", userID), 10, time.Minute},
			{"speech:global", 20, time.Second},
		} {
			allowed, err := s.limiter.AllowSlidingWindow(ctx, limit.key, limit.count, limit.window, limit.window*2)
			if err != nil {
				return speechport.Transcript{}, speechport.ErrUnavailable
			}
			if !allowed {
				return speechport.Transcript{}, speechport.ErrBusy
			}
		}
	}
	result, err := s.recognizer.Recognize(ctx, cfg, audio)
	if err != nil {
		return speechport.Transcript{}, err
	}
	result.Text = strings.TrimSpace(result.Text)
	if result.Text == "" {
		return speechport.Transcript{}, speechport.ErrNoSpeech
	}
	if result.DurationMS > 60000 || len([]rune(result.Text)) > 8000 {
		return speechport.Transcript{}, speechport.ErrInvalidAudio
	}
	return result, nil
}
