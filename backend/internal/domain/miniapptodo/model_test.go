package miniapptodo

import (
	"testing"
	"time"
)

func TestNextDateKeepsMonthlyAnchorAndSkipsMissedOccurrences(t *testing.T) {
	task := Task{DueDate: "2026-01-31", Timezone: "Asia/Shanghai", RepeatKind: "monthly", RepeatDay: 31}
	now := time.Date(2026, 1, 31, 3, 0, 0, 0, time.UTC)
	next, err := NextDate(task, now)
	if err != nil || next != "2026-02-28" {
		t.Fatalf("next = %q, %v", next, err)
	}
	task.DueDate = next
	next, err = NextDate(task, time.Date(2026, 2, 28, 3, 0, 0, 0, time.UTC))
	if err != nil || next != "2026-03-31" {
		t.Fatalf("anchor lost: %q %v", next, err)
	}
	task.RepeatKind = "weekdays"
	task.DueDate = "2026-01-01"
	next, err = NextDate(task, time.Date(2026, 9, 11, 3, 0, 0, 0, time.UTC))
	if err != nil || next != "2026-09-14" {
		t.Fatalf("missed weekdays = %q %v", next, err)
	}
}

func TestNextDateWeeklyAndTimezone(t *testing.T) {
	task := Task{DueDate: "2026-09-07", Timezone: "Asia/Shanghai", RepeatKind: "weekly", RepeatDay: 1}
	next, err := NextDate(task, time.Date(2026, 9, 13, 18, 0, 0, 0, time.UTC))
	if err != nil || next != "2026-09-21" {
		t.Fatalf("next local Monday = %q %v", next, err)
	}
}
