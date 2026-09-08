package settings

import (
	"context"
	"errors"
	"strings"

	domainspeech "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/speech"
)

func validateSpeechSetting(key, value string) error {
	switch key {
	case "enabled":
		// Credentials are checked against effective settings in validateSpeechSettings.
		_, err := domainspeech.Parse(map[string]string{
			"enabled":            value,
			"tencent_secret_id":  "configured",
			"tencent_secret_key": "configured",
		})
		return err
	case "engine":
		if !domainspeech.ValidEngine(strings.TrimSpace(value)) {
			return errors.New("speech:engine is not supported")
		}
	case "tencent_secret_id", "tencent_secret_key":
		return validateStringMax(value, 256, "speech:"+key)
	}
	return nil
}

func (s *Service) validateSpeechSettings(ctx context.Context, patches []PatchItem) error {
	touched := false
	for _, item := range patches {
		if item.Namespace == domainspeech.Namespace {
			touched = true
			break
		}
	}
	if !touched {
		return nil
	}
	next, err := s.loadEffectiveSettings(ctx, domainspeech.Namespace)
	if err != nil {
		return err
	}
	applyPatchesToEffectiveSettings(next, patches, domainspeech.Namespace)
	values := make(map[string]string)
	for key, value := range next {
		values[strings.TrimPrefix(key, domainspeech.Namespace+":")] = value
	}
	_, err = domainspeech.Parse(values)
	return err
}
