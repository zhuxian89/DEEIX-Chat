package settings

import "testing"

func TestSanitizePatchItemsRedactsTelegramBotToken(t *testing.T) {
	items := sanitizePatchItemsForAudit([]PatchItem{{
		Namespace: "notify",
		Key:       "bot_token",
		Value:     "secret-bot-token",
	}})
	if len(items) != 1 || items[0].Value != "[REDACTED]" {
		t.Fatalf("expected Telegram Bot Token to be redacted, got %#v", items)
	}
}
