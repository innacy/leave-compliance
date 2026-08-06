package main

import (
	"testing"
	"time"
)

func makeTime(year, month, day, hour, min int) time.Time {
	return time.Date(year, time.Month(month), day, hour, min, 0, 0, time.UTC)
}

func TestProcessAttendance_GroupsByDate(t *testing.T) {
	rows := []AttendanceRow{
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 20, 9, 0)},
		{BadgeNumber: "EMP002", CheckTime: makeTime(2026, 7, 20, 9, 30)},
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 21, 8, 45)},
	}

	result := ProcessAttendance(rows)

	if len(result) != 2 {
		t.Fatalf("got %d days, want 2", len(result))
	}
	if result[0].Date != "2026-07-20" {
		t.Errorf("first date = %q, want %q", result[0].Date, "2026-07-20")
	}
	if len(result[0].Present) != 2 {
		t.Errorf("day 1 present count = %d, want 2", len(result[0].Present))
	}
	if result[1].Date != "2026-07-21" {
		t.Errorf("second date = %q, want %q", result[1].Date, "2026-07-21")
	}
}

func TestProcessAttendance_DeduplicatesSameDay(t *testing.T) {
	rows := []AttendanceRow{
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 20, 9, 0)},
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 20, 13, 0)},
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 20, 18, 0)},
	}

	result := ProcessAttendance(rows)

	if len(result) != 1 {
		t.Fatalf("got %d days, want 1", len(result))
	}
	if len(result[0].Present) != 1 {
		t.Errorf("present count = %d, want 1", len(result[0].Present))
	}
	if result[0].Present[0] != "EMP001" {
		t.Errorf("present[0] = %q, want %q", result[0].Present[0], "EMP001")
	}
}

func TestProcessAttendance_TrimsWhitespace(t *testing.T) {
	rows := []AttendanceRow{
		{BadgeNumber: "  EMP001  ", CheckTime: makeTime(2026, 7, 20, 9, 0)},
	}

	result := ProcessAttendance(rows)

	if result[0].Present[0] != "EMP001" {
		t.Errorf("present[0] = %q, want %q", result[0].Present[0], "EMP001")
	}
}

func TestProcessAttendance_SkipsBlanks(t *testing.T) {
	rows := []AttendanceRow{
		{BadgeNumber: "", CheckTime: makeTime(2026, 7, 20, 9, 0)},
		{BadgeNumber: "   ", CheckTime: makeTime(2026, 7, 20, 9, 30)},
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 20, 10, 0)},
	}

	result := ProcessAttendance(rows)

	if len(result) != 1 {
		t.Fatalf("got %d days, want 1", len(result))
	}
	if len(result[0].Present) != 1 {
		t.Errorf("present count = %d, want 1", len(result[0].Present))
	}
}

func TestProcessAttendance_SortsDatesChronologically(t *testing.T) {
	rows := []AttendanceRow{
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 22, 9, 0)},
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 20, 9, 0)},
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 21, 9, 0)},
	}

	result := ProcessAttendance(rows)

	if result[0].Date != "2026-07-20" {
		t.Errorf("first date = %q, want %q", result[0].Date, "2026-07-20")
	}
	if result[1].Date != "2026-07-21" {
		t.Errorf("second date = %q, want %q", result[1].Date, "2026-07-21")
	}
	if result[2].Date != "2026-07-22" {
		t.Errorf("third date = %q, want %q", result[2].Date, "2026-07-22")
	}
}

func TestProcessAttendance_SortsEmpcodesAlphabetically(t *testing.T) {
	rows := []AttendanceRow{
		{BadgeNumber: "EMP003", CheckTime: makeTime(2026, 7, 20, 9, 0)},
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 20, 9, 30)},
		{BadgeNumber: "EMP002", CheckTime: makeTime(2026, 7, 20, 10, 0)},
	}

	result := ProcessAttendance(rows)

	want := []string{"EMP001", "EMP002", "EMP003"}
	for i, code := range result[0].Present {
		if code != want[i] {
			t.Errorf("present[%d] = %q, want %q", i, code, want[i])
		}
	}
}

func TestProcessAttendance_EmptyInput(t *testing.T) {
	result := ProcessAttendance(nil)

	if len(result) != 0 {
		t.Errorf("got %d days, want 0", len(result))
	}
}
