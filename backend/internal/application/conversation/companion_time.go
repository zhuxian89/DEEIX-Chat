package conversation

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
)

var companionTimeMarker = regexp.MustCompile(`\[历史消息时间：北京时间 \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\][ \t]*(?:\r?\n)?`)

func cleanCompanionTimeMarkers(text string) string {
	return companionTimeMarker.ReplaceAllString(text, "")
}

type companionHistoryTime struct {
	Role        string `json:"role"`
	TextExcerpt string `json:"text_excerpt"`
	AtBeijing   string `json:"at_beijing"`
}

// CompanionHistoryTimePrompt keeps dates in system context, never in dialogue text.
// Excerpts identify the dated messages; the ordinary prompt budget still applies.
func CompanionHistoryTimePrompt(messages []model.Message, forgetThrough uint) string {
	if len(messages) > CompanionRecentMessageLimit {
		messages = messages[len(messages)-CompanionRecentMessageLimit:]
	}
	entries := make([]companionHistoryTime, 0, len(messages))
	beijing := time.FixedZone("Asia/Shanghai", 8*60*60)
	for _, message := range messages {
		if message.ID <= forgetThrough || strings.EqualFold(message.Status, "blocked") || message.CreatedAt.IsZero() || (message.Role != "user" && message.Role != "assistant") {
			continue
		}
		text := message.Content
		if message.Role == "assistant" {
			text = cleanCompanionTimeMarkers(text)
		}
		excerpt := []rune(strings.TrimSpace(text))
		if len(excerpt) > 120 {
			excerpt = excerpt[:120]
		}
		entries = append(entries, companionHistoryTime{
			Role: message.Role, TextExcerpt: string(excerpt), AtBeijing: message.CreatedAt.In(beijing).Format(time.RFC3339),
		})
	}
	if len(entries) == 0 {
		return ""
	}
	data, _ := json.Marshal(entries)
	return "\n<history_message_times>\n" + string(data) + "\n</history_message_times>"
}
