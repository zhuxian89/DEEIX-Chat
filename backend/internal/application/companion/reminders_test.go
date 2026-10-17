package companion

import (
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"testing"
	"time"
)

func TestConversationalReminderCreation(t *testing.T) {
	now := time.Date(2026, 10, 16, 7, 0, 0, 0, domain.ReminderZone)
	for _, tt := range []struct{ input, title, kind, at string }{
		{"我1个小时候需要喝药", "喝药", "once", "2026-10-16 08:00"},
		{"一个小时后提醒我吃药", "吃药", "once", "2026-10-16 08:00"},
		{"半小时后提醒我跳绳", "跳绳", "once", "2026-10-16 07:30"},
		{"一个半小时后提醒我喝药", "喝药", "once", "2026-10-16 08:30"},
		{"一小时三十分钟后提醒我跳绳十分钟", "跳绳十分钟", "once", "2026-10-16 08:30"},
		{"每隔两小时提醒我活动十分钟", "活动十分钟", "interval", "2026-10-16 09:00"},
		{"每天早晚八点提醒我喝水", "喝水", "daily", "2026-10-16 08:00"},
		{"每周一至周五早上八点提醒我上班", "上班", "weekly", "2026-10-16 08:00"},
		{"每天8点提醒我陪伴孩子写作业", "陪伴孩子写作业", "daily", "2026-10-16 08:00"},
		{"每天晚上八点提醒我陪孩子写作业", "陪孩子写作业", "daily", "2026-10-16 20:00"},
		{"明天上午九点半提醒我开会", "开会", "once", "2026-10-17 09:30"},
		{"每周一、三、五晚上八点提醒我跳绳", "跳绳", "weekly", "2026-10-16 20:00"},
		{"每隔两小时提醒我喝水", "喝水", "interval", "2026-10-16 09:00"},
		{"每月31日早上八点提醒我交账单", "交账单", "monthly", "2026-10-31 08:00"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			c := ParseReminderCommand(tt.input, domain.ReminderSnapshot{}, now)
			if c.Action != "create" || c.Title != tt.title || c.Schedule.Kind != tt.kind || domain.ReminderTime(c.Schedule.Next(now, 0)) != tt.at {
				t.Fatalf("%+v next=%s", c, domain.ReminderTime(c.Schedule.Next(now, 0)))
			}
		})
	}
}

func TestReminderConversationLifecycleAndClarification(t *testing.T) {
	now := time.Date(2026, 10, 16, 7, 0, 0, 0, domain.ReminderZone)
	state := domain.ReminderSnapshot{Plans: []domain.Reminder{{ID: "medicine", Title: "吃药", Status: "active", Revision: 2, Schedule: domain.ReminderSchedule{Kind: "daily", StartAt: now, Times: []int{480}}}}}
	for _, tt := range []struct{ input, action string }{{"吃药还有多久", "list"}, {"查看所有提醒", "list"}, {"删除吃药提醒", "delete"}, {"暂停吃药提醒", "pause"}, {"恢复吃药提醒", "resume"}, {"把吃药提醒推迟十分钟", "snooze"}, {"我已经吃药了", "complete"}, {"把吃药提醒改到晚上九点", "update"}, {"今天跳过吃药提醒", "skip"}, {"今天吃什么", ""}, {"比如每天八点提醒我吃药", ""}, {"提醒我喝水", "clarify"}} {
		c := ParseReminderCommand(tt.input, state, now)
		if c.Action != tt.action {
			t.Errorf("%s: %+v", tt.input, c)
		}
		if c.Action == "update" && (c.Schedule.Kind != "daily" || len(c.Schedule.Times) != 1 || c.Schedule.Times[0] != 1260) {
			t.Errorf("lost recurrence: %+v", c)
		}
	}
	state.Plans = append(state.Plans, domain.Reminder{ID: "second", Title: "吃药", Status: "active"})
	if c := ParseReminderCommand("删除吃药提醒", state, now); c.Action != "clarify" {
		t.Fatalf("ambiguous delete: %+v", c)
	}
	state = domain.ReminderSnapshot{Turns: []domain.ReminderTurn{{Input: "提醒我喝水", Reply: "还需要具体时间", CreatedAt: now}}}
	if c := ParseReminderCommand("半小时后", state, now); c.Action != "create" || c.Title != "喝水" {
		t.Fatalf("clarification: %+v", c)
	}
}
