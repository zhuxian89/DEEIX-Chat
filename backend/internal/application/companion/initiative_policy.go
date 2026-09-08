package companion

import (
	"strings"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	domainchat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
)

func proactivityMode(p *Profile, pace *domain.InitiativeState) string {
	if p.Quiet || pace.Mode == "off" {
		return "off"
	}
	if pace.Mode == "less" {
		return "less"
	}
	return "normal"
}

func closingConversation(text string) bool {
	return containsAny(strings.ToLower(text), []string{
		"先这样", "晚点聊", "回头聊", "改天聊", "不聊了", "不想聊", "不用回", "别回复", "别打扰", "别主动", "不要主动", "别推荐", "不想看推荐", "我先忙", "我要忙", "忙去了", "去忙了", "我在忙", "先去忙", "睡了", "晚安", "再见", "拜拜", "good night", "talk later", "leave me alone", "don't reply", "do not reply",
	})
}

func awaitingAnswer(text string) bool {
	return containsAny(text, []string{"?", "？", "要不要", "愿不愿意", "想不想", "怎么样", "告诉我", "说说看"}) ||
		strings.HasSuffix(strings.TrimRight(strings.TrimSpace(text), "。！!～~…"), "吗") ||
		strings.HasSuffix(strings.TrimRight(strings.TrimSpace(text), "。！!～~…"), "呢")
}

func latestTurn(items []domainchat.Message, role string) domainchat.Message {
	var latest domainchat.Message
	for _, item := range items {
		if item.Role == role && item.ID > latest.ID {
			latest = item
		}
	}
	return latest
}

func initiativeAllowed(p *Profile, pace *domain.InitiativeState, kind, visit string, after uint, items []domainchat.Message, now time.Time, accepting bool) bool {
	mode := proactivityMode(p, pace)
	if mode == "off" || p.LeaseUntil > now.Unix() {
		return false
	}
	user, assistant := latestTurn(items, "user"), latestTurn(items, "assistant")
	_, latest := messageBoundary(items)
	if latest > p.ReadThroughID {
		return false
	}
	if latest > 0 && (assistant.ID != latest || assistant.Status != "success") {
		return false
	}
	if kind == "idle" {
		if after == 0 || after != latest || user.ID <= p.ForgetThroughID || user.Status != "success" ||
			assistant.CreatedAt.IsZero() || now.Sub(assistant.CreatedAt) < 45*time.Second || now.Sub(assistant.CreatedAt) > 30*time.Minute ||
			closingConversation(user.Content) || awaitingAnswer(assistant.Content) || strings.TrimSpace(assistant.Content) == "" {
			return false
		}
		if !accepting && (pace.IdleVisitID == visit || pace.IdleAfterMessageID >= latest || now.Sub(pace.IdleAttemptAt) < 5*time.Minute) {
			return false
		}
		cooldown := 10 * time.Minute
		if mode == "less" {
			cooldown = time.Hour
		}
		return now.Sub(pace.LastIdleAt) >= cooldown
	}
	if kind != "home" {
		return false
	}
	if !accepting && now.Sub(pace.LastAttemptAt) < 5*time.Minute {
		return false
	}
	if !user.CreatedAt.IsZero() && now.Sub(user.CreatedAt) < 6*time.Hour {
		return false
	}
	lastHome, lastUser := pace.LastHomeAt, pace.HomeAfterUserID
	if p.GreetingAt.After(lastHome) {
		lastHome, lastUser = p.GreetingAt, p.GreetingAfterUserID
	}
	if lastHome.IsZero() {
		return true
	}
	dailyLimit := uint(2)
	interval := 12 * time.Hour
	if mode == "less" {
		dailyLimit, interval = 1, 24*time.Hour
	}
	if pace.HomeDay == now.In(beijing).Format("2006-01-02") && pace.HomeCount >= dailyLimit {
		return false
	}
	if user.ID <= lastUser {
		for i := uint(1); i < pace.UnansweredHomes && interval < 7*24*time.Hour; i++ {
			interval *= 2
		}
		if interval > 7*24*time.Hour {
			interval = 7 * 24 * time.Hour
		}
	}
	return now.Sub(lastHome) >= interval
}

func acceptedPacing(pace domain.InitiativeState, item domain.Initiative) domain.InitiativeState {
	if item.Kind == "idle" {
		pace.LastIdleAt = item.AcceptedAt
		return pace
	}
	day := item.AcceptedAt.In(beijing).Format("2006-01-02")
	if pace.HomeDay != day {
		pace.HomeDay, pace.HomeCount = day, 0
	}
	pace.HomeCount++
	if item.AfterUserID > pace.HomeAfterUserID {
		pace.UnansweredHomes = 0
	}
	if pace.UnansweredHomes < 8 {
		pace.UnansweredHomes++
	}
	pace.LastHomeAt, pace.HomeAfterUserID = item.AcceptedAt, item.AfterUserID
	return pace
}
