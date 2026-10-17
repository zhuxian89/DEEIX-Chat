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

func TestReminderCommandsPreserveNamedTargets(t *testing.T) {
	now := time.Date(2026, 10, 17, 16, 22, 0, 0, domain.ReminderZone)
	for _, names := range []struct{ title, other string }{
		{"为了测试", "为测试"},
		{"宝宝的药", "宝宝药"},
		{"维修把手", "维修手"},
	} {
		t.Run(names.title, func(t *testing.T) {
			created := ParseReminderCommand("2分钟后提醒我"+names.title, domain.ReminderSnapshot{}, now)
			if created.Action != "create" || created.Title != names.title {
				t.Fatalf("create: %+v", created)
			}
			state := domain.ReminderSnapshot{
				Plans: []domain.Reminder{
					{ID: "target", Title: created.Title, Status: "active", Revision: 3, Schedule: created.Schedule},
					{ID: "other", Title: names.other, Status: "active", Revision: 7, Schedule: created.Schedule},
				},
				Turns: []domain.ReminderTurn{{Input: "2分钟后提醒我" + names.other, ReminderIDs: []string{"other"}, CreatedAt: now}},
			}
			for _, tt := range []struct{ input, action string }{
				{"把" + names.title + "提醒改到3分钟后", "update"},
				{"暂停" + names.title + "提醒", "pause"},
				{"恢复" + names.title + "提醒", "resume"},
				{"删除" + names.title + "提醒", "delete"},
				{"把" + names.title + "提醒推迟十分钟", "snooze"},
				{names.title + "提醒已完成", "complete"},
				{"查询" + names.title + "提醒", "list"},
			} {
				t.Run(tt.input, func(t *testing.T) {
					command := ParseReminderCommand(tt.input, state, now)
					if command.Action != tt.action || len(command.IDs) != 1 || command.IDs[0] != "target" {
						t.Fatalf("wrong target: %+v", command)
					}
					if tt.action != "list" && (command.ID != "target" || command.Revision != 3 || command.Title != names.title) {
						t.Fatalf("lost reminder identity: %+v", command)
					}
					if tt.action == "update" && (command.Schedule.Kind != "once" || !command.Schedule.StartAt.Equal(now.Add(3*time.Minute))) {
						t.Fatalf("wrong updated time: %+v", command)
					}
					if tt.action == "snooze" && command.DelayMinutes != 10 {
						t.Fatalf("wrong delay: %+v", command)
					}
				})
			}
		})
	}
}

func TestReminderNamedTargetsStillRequireDisambiguation(t *testing.T) {
	now := time.Date(2026, 10, 17, 16, 22, 0, 0, domain.ReminderZone)
	state := domain.ReminderSnapshot{Plans: []domain.Reminder{
		{ID: "first", Title: "为了测试", Status: "active"},
		{ID: "second", Title: "为了测试", Status: "paused"},
		{ID: "deleted", Title: "为了测试", Status: "deleted"},
	}}
	for _, input := range []string{"把为了测试提醒改到3分钟后", "删除为了测试提醒", "删除“为了测试”提醒"} {
		command := ParseReminderCommand(input, state, now)
		if command.Action != "clarify" || len(command.IDs) != 2 || command.IDs[0] != "first" || command.IDs[1] != "second" {
			t.Fatalf("%s: %+v", input, command)
		}
	}
}

func TestQuotedReminderTitles(t *testing.T) {
	now := time.Date(2026, 10, 17, 16, 22, 0, 0, domain.ReminderZone)
	for _, quotes := range [][2]string{{"“", "”"}, {`"`, `"`}, {"「", "」"}} {
		for _, title := range []string{"为了测试", "取消会员", "跳绳10分钟", "一个半小时复盘"} {
			quoted := quotes[0] + title + quotes[1]
			t.Run(quoted, func(t *testing.T) {
				created := ParseReminderCommand("2分钟后提醒我"+quoted, domain.ReminderSnapshot{}, now)
				if created.Action != "create" || created.Title != title || !created.Schedule.StartAt.Equal(now.Add(2*time.Minute)) {
					t.Fatalf("quoted creation: %+v", created)
				}
				state := domain.ReminderSnapshot{Plans: []domain.Reminder{
					{ID: "target", Title: title, Status: "active", Revision: 3, Schedule: created.Schedule},
					{ID: "partial", Title: "测试", Status: "active"},
				}}
				for _, tt := range []struct{ input, action string }{
					{"把" + quoted + "提醒改到3分钟后", "update"},
					{"暂停" + quoted + "提醒", "pause"},
					{"恢复" + quoted + "提醒", "resume"},
					{"删除" + quoted + "提醒", "delete"},
					{"把" + quoted + "提醒推迟十分钟", "snooze"},
					{quoted + "提醒已完成", "complete"},
					{"查询" + quoted + "提醒", "list"},
				} {
					command := ParseReminderCommand(tt.input, state, now)
					if command.Action != tt.action || len(command.IDs) != 1 || command.IDs[0] != "target" {
						t.Fatalf("%s: %+v", tt.input, command)
					}
					if tt.action == "update" && !command.Schedule.StartAt.Equal(now.Add(3*time.Minute)) {
						t.Fatalf("title affected new time: %+v", command)
					}
					if tt.action == "snooze" && command.DelayMinutes != 10 {
						t.Fatalf("title affected delay: %+v", command)
					}
				}
			})
		}
	}
}

func TestQuotedReminderTargetDoesNotFallBack(t *testing.T) {
	now := time.Date(2026, 10, 17, 16, 22, 0, 0, domain.ReminderZone)
	state := domain.ReminderSnapshot{
		Plans: []domain.Reminder{
			{ID: "medicine", Title: "吃药", Status: "active"},
			{ID: "deleted", Title: "为了测试", Status: "deleted"},
		},
		Turns: []domain.ReminderTurn{{Input: "2分钟后提醒我吃药", ReminderIDs: []string{"medicine"}, CreatedAt: now}},
	}
	for _, input := range []string{
		"把“并不存在”提醒改成3分钟后", "删除“药”提醒", "删除“喝药”提醒",
		"删除“为了测试”提醒", "查看“并不存在”提醒", "删除“”提醒",
		"删除“吃药提醒", `删除"吃药”提醒`, "删除“吃药”和“喝水”提醒",
	} {
		command := ParseReminderCommand(input, state, now)
		if command.Action != "clarify" || command.ID != "" || len(command.IDs) != 0 {
			t.Fatalf("%s selected a reminder: %+v", input, command)
		}
	}
	state.Turns = []domain.ReminderTurn{{Input: "提醒我“跳绳10分钟”", Reply: "还需要具体时间", CreatedAt: now}}
	command := ParseReminderCommand("半小时后", state, now)
	if command.Action != "create" || command.Title != "跳绳10分钟" || !command.Schedule.StartAt.Equal(now.Add(30*time.Minute)) {
		t.Fatalf("quoted clarification: %+v", command)
	}
	if command := ParseReminderCommand("帮我给“老板”写封邮件", state, now); command.Action != "" {
		t.Fatalf("intercepted ordinary chat: %+v", command)
	}
}
