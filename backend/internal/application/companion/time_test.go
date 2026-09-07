package companion

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCompanionBeijingClockAndGreetingBoundaries(t *testing.T) {
	cases := []struct {
		hour            int
		period, opening string
	}{
		{0, "深夜", "夜深了"}, {4, "深夜", "夜深了"}, {5, "早晨", "早呀"}, {8, "早晨", "早呀"},
		{9, "上午", "上午好"}, {10, "上午", "上午好"}, {11, "午间", "中午好"}, {13, "午间", "中午好"},
		{14, "下午", "下午好"}, {17, "下午", "下午好"}, {18, "晚上", "晚上好"}, {22, "晚上", "晚上好"}, {23, "深夜", "夜深了"},
	}
	for _, tc := range cases {
		now := time.Date(2026, 9, 8, tc.hour, 0, 0, 0, beijing).UTC()
		clock := clockAt(now)
		if clock.Period != tc.period || clock.Weekday != "星期二" || !strings.HasPrefix(clock.Now, "2026-09-08") || !strings.HasSuffix(clock.Now, "+08:00") {
			t.Fatalf("incorrect clock for Beijing hour %d: %+v", tc.hour, clock)
		}
		if greeting := Greeting(&Profile{}, now); !strings.HasPrefix(greeting, tc.opening) || !strings.Contains(greeting, "AI") {
			t.Fatalf("wrong first greeting: %s", greeting)
		}
		p := &Profile{UserID: 7, GreetingAt: now.Add(-24 * time.Hour)}
		first := Greeting(p, now)
		p.LastGreeting = first
		if Greeting(p, now) == first {
			t.Fatal("repeated greeting was not varied")
		}
	}
}

func TestCompanionContinuityUsesCalendarDaysAcrossMidnight(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 5, 0, 0, time.UTC)
	last := now.Add(-10 * time.Minute)
	prompt := continuityPrompt(now, last, nil)
	raw := strings.TrimSuffix(strings.TrimPrefix(prompt, "\n<conversation_context>\n"), "\n</conversation_context>")
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatal(err)
	}
	if data["calendar_days_since_last_user_message"] != float64(1) || data["minutes_since_last_user_message"] != float64(10) || data["last_user_message_beijing"] != "2026-09-07T23:55:00+08:00" {
		t.Fatalf("wrong cross-midnight continuity: %v", data)
	}
	if strings.Contains(continuityPrompt(now, now.Add(time.Hour), nil), "minutes_since_last") {
		t.Fatal("negative elapsed time exposed")
	}
}

func TestCompanionMemoryKeepsOriginalStatementTime(t *testing.T) {
	now := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	saidAt := time.Date(2026, 9, 7, 15, 55, 0, 0, time.UTC)
	facts := validatedFacts(7, []extractedFact{{Key: "面试", Value: "2026-09-08要面试", Evidence: "我明天要面试", SourceMessageID: 10, Days: 7}},
		map[uint]extractionSource{10: {Text: "我明天要面试", At: saidAt}}, now)
	if len(facts) != 1 || !facts[0].SourceAt.Equal(saidAt) {
		t.Fatalf("source timestamp lost: %+v", facts)
	}
	prompt := BuildPrompt(&Profile{}, facts, now)
	if !strings.Contains(prompt, "2026-09-07T23:55:00+08:00") || !strings.Contains(prompt, "2026-09-10T08:00:00+08:00") || strings.Contains(prompt, "today_utc") {
		t.Fatalf("source and current date missing: %s", prompt)
	}
}
