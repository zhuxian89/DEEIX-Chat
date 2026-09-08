package speech

import (
	"context"
	"errors"

	domainspeech "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/speech"
)

const MaxAudioBytes = 512 * 1024

var (
	ErrUnavailable  = errors.New("speech recognition is unavailable")
	ErrInvalidAudio = errors.New("invalid speech audio")
	ErrNoSpeech     = errors.New("no speech was recognized")
	ErrQuota        = errors.New("speech recognition quota is exhausted")
	ErrBusy         = errors.New("speech recognition is busy")
)

type Transcript struct {
	Text       string
	DurationMS int
}

type Recognizer interface {
	Recognize(context.Context, domainspeech.Config, []byte) (Transcript, error)
}
