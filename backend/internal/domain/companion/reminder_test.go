package companion

import (
	"testing"
	"time"
)

func TestReminderCalendarAndAnchoring(t *testing.T) {
	start := time.Date(2026, 10, 16, 7, 0, 0, 0, ReminderZone)
	daily := ReminderSchedule{Kind: "daily", StartAt: start, Times: []int{8 * 60}}
	first := daily.Next(start, 0)
	if got := ReminderTime(first); got != "2026-10-16 08:00" {
		t.Fatal(got)
	}
	if got := ReminderTime(daily.Next(first.Add(10*time.Minute), 1)); got != "2026-10-17 08:00" {
		t.Fatal(got)
	}
	weekly := ReminderSchedule{Kind: "weekly", StartAt: start, Times: []int{1200}, Weekdays: []int{1, 3, 5}}
	if got := ReminderTime(weekly.Next(time.Date(2026, 10, 16, 21, 0, 0, 0, ReminderZone), 1)); got != "2026-10-19 20:00" {
		t.Fatal(got)
	}
	monthly := ReminderSchedule{Kind: "monthly", StartAt: start, Times: []int{480}, Day: 31}
	if got := ReminderTime(monthly.Next(time.Date(2026, 10, 31, 9, 0, 0, 0, ReminderZone), 1)); got != "2026-12-31 08:00" {
		t.Fatal(got)
	}
	monthly.Day = -1
	if got := ReminderTime(monthly.Next(time.Date(2026, 10, 31, 9, 0, 0, 0, ReminderZone), 1)); got != "2026-11-30 08:00" {
		t.Fatal(got)
	}
	yearly := ReminderSchedule{Kind: "yearly", StartAt: start, Times: []int{480}, Month: 2, Day: 29}
	if got := ReminderTime(yearly.Next(start, 0)); got != "2028-02-29 08:00" {
		t.Fatal(got)
	}
	interval := ReminderSchedule{Kind: "interval", StartAt: start, IntervalMinutes: 60}
	if got := ReminderTime(interval.Next(start.Add(65*time.Minute), 1)); got != "2026-10-16 09:00" {
		t.Fatal(got)
	}
	daily.Limit = 2
	if !daily.Next(start, 2).IsZero() {
		t.Fatal("limit ignored")
	}
	daily.Limit = 0
	daily.Until = first
	if !daily.Next(first, 1).IsZero() {
		t.Fatal("end date ignored")
	}
}

func TestReminderTimePreservesSecondsAcrossBeijingMidnight(t *testing.T) {
	at := time.Date(2026, 10, 17, 15, 59, 50, 0, time.UTC).Add(30 * time.Second)
	if got := ReminderTime(at); got != "2026-10-18 00:00:20" {
		t.Fatal(got)
	}
}
