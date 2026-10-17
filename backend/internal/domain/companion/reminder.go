package companion

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

var ReminderZone = time.FixedZone("UTC+8", 8*3600)

// ReminderSchedule uses local calendar times; an interval stays anchored to StartAt.
type ReminderSchedule struct {
	Kind                               string
	StartAt, Until                     time.Time
	Times, Weekdays                    []int
	IntervalMinutes, Day, Month, Limit int
}

type Reminder struct {
	ID                                   string
	UserID                               uint
	Title, Status                        string
	Schedule                             ReminderSchedule
	Cursor, NextAt, CreatedAt, UpdatedAt time.Time
	Revision                             uint
	ScheduleVersion                      uint
	Generated                            int
}

type ReminderOccurrence struct {
	ID, ReminderID                              string
	UserID                                      uint
	Title, Status, NotificationStatus           string
	ScheduledAt, DueAt, NotifiedAt, CompletedAt time.Time
	Revision                                    uint
	ScheduleVersion                             uint
}

type ReminderTurn struct {
	ID, ConversationID, Input, Reply string
	UserID                           uint
	ReminderIDs                      []string
	CreatedAt                        time.Time
}

type ReminderCommand struct {
	RequestID, ConversationID, Input, Action, ID, OccurrenceID, Title, Reply string
	Schedule                                                                 ReminderSchedule
	Revision                                                                 uint
	DelayMinutes                                                             int
	DelaySeconds                                                             int
	IDs                                                                      []string
}

type ReminderSnapshot struct {
	Plans       []Reminder
	Occurrences []ReminderOccurrence
	Turns       []ReminderTurn
	ServerTime  time.Time
}

type ReminderResult struct {
	Handled  bool
	Turn     ReminderTurn
	Snapshot ReminderSnapshot
}

func (s ReminderSchedule) Validate() bool {
	if s.StartAt.IsZero() || s.Limit < 0 || s.Limit > 10000 || (!s.Until.IsZero() && s.Until.Before(s.StartAt)) {
		return false
	}
	if s.Kind == "once" {
		return true
	}
	if s.Kind == "interval" {
		return s.IntervalMinutes >= 1 && s.IntervalMinutes <= 366*24*60
	}
	if s.Kind != "daily" && s.Kind != "weekly" && s.Kind != "monthly" && s.Kind != "yearly" {
		return false
	}
	if len(s.Times) == 0 || len(s.Times) > 8 {
		return false
	}
	seen := map[int]bool{}
	for _, minute := range s.Times {
		if minute < 0 || minute >= 1440 || seen[minute] {
			return false
		}
		seen[minute] = true
	}
	if s.Kind == "weekly" {
		if len(s.Weekdays) == 0 || len(s.Weekdays) > 7 {
			return false
		}
		seen = map[int]bool{}
		for _, day := range s.Weekdays {
			if day < 0 || day > 6 || seen[day] {
				return false
			}
			seen[day] = true
		}
	}
	if (s.Kind == "monthly" || s.Kind == "yearly") && s.Day != -1 && (s.Day < 1 || s.Day > 31) {
		return false
	}
	if s.Kind == "yearly" && (s.Month < 1 || s.Month > 12) {
		return false
	}
	return true
}

// Next returns the first schedule time strictly after after. Nonexistent dates are skipped.
func (s ReminderSchedule) Next(after time.Time, generated int) time.Time {
	if !s.Validate() || (s.Limit > 0 && generated >= s.Limit) {
		return time.Time{}
	}
	accept := func(t time.Time) time.Time {
		if !s.Until.IsZero() && t.After(s.Until) {
			return time.Time{}
		}
		return t.UTC()
	}
	if s.Kind == "once" {
		if generated == 0 && s.StartAt.After(after) {
			return accept(s.StartAt)
		}
		return time.Time{}
	}
	if s.Kind == "interval" {
		step := time.Duration(s.IntervalMinutes) * time.Minute
		t := s.StartAt
		if !t.After(after) {
			t = t.Add((after.Sub(t)/step + 1) * step)
		}
		return accept(t)
	}
	base := after.In(ReminderZone)
	if s.StartAt.After(after) {
		base = s.StartAt.In(ReminderZone)
	}
	day := time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, ReminderZone)
	times := append([]int(nil), s.Times...)
	sort.Ints(times)
	for i := 0; i < 366*9; i++ {
		d := day.AddDate(0, 0, i)
		if !s.Until.IsZero() && d.After(s.Until) {
			break
		}
		match := s.Kind == "daily"
		if s.Kind == "weekly" {
			for _, w := range s.Weekdays {
				if int(d.Weekday()) == w {
					match = true
				}
			}
		}
		if s.Kind == "monthly" || s.Kind == "yearly" {
			last := time.Date(d.Year(), d.Month()+1, 0, 0, 0, 0, 0, ReminderZone).Day()
			match = d.Day() == s.Day || s.Day == -1 && d.Day() == last
			if s.Kind == "yearly" {
				match = match && int(d.Month()) == s.Month
			}
		}
		if !match {
			continue
		}
		for _, m := range times {
			t := d.Add(time.Duration(m) * time.Minute)
			if t.After(after) && !t.Before(s.StartAt) {
				return accept(t)
			}
		}
	}
	return time.Time{}
}

func ReminderTime(t time.Time) string {
	if t.IsZero() {
		return "暂无下次提醒"
	}
	if t.Second() != 0 {
		return t.In(ReminderZone).Format("2006-01-02 15:04:05")
	}
	return t.In(ReminderZone).Format("2006-01-02 15:04")
}

func (s ReminderSchedule) Description() string {
	if s.Kind == "once" {
		return "单次提醒"
	}
	if s.Kind == "interval" {
		if s.IntervalMinutes%1440 == 0 {
			return fmt.Sprintf("每隔%d天", s.IntervalMinutes/1440)
		}
		if s.IntervalMinutes%60 == 0 {
			return fmt.Sprintf("每隔%d小时", s.IntervalMinutes/60)
		}
		return fmt.Sprintf("每隔%d分钟", s.IntervalMinutes)
	}
	label := "每天"
	if s.Kind == "weekly" {
		days := []string{}
		for _, w := range s.Weekdays {
			if w >= 0 && w <= 6 {
				days = append(days, []string{"日", "一", "二", "三", "四", "五", "六"}[w])
			}
		}
		label = "每周" + strings.Join(days, "、")
	}
	if s.Kind == "monthly" {
		label = fmt.Sprintf("每月%d日", s.Day)
		if s.Day == -1 {
			label = "每月月底"
		}
	}
	if s.Kind == "yearly" {
		label = fmt.Sprintf("每年%d月%d日", s.Month, s.Day)
	}
	parts := []string{}
	for _, m := range s.Times {
		parts = append(parts, fmt.Sprintf("%02d:%02d", m/60, m%60))
	}
	return label + " " + strings.Join(parts, "、")
}
