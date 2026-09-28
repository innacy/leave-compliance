package handler

import (
	"testing"
	"time"
)

func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 10, 0, 0, 0, time.UTC)
}

func TestGetPreviousWeekWorkdays_Monday(t *testing.T) {
	// Mon Sep 28 2026 → previous week is Sep 21-25
	today := date(2026, time.September, 28)

	got := getPreviousWeekWorkdays(today)

	want := []time.Time{
		date(2026, time.September, 21),
		date(2026, time.September, 22),
		date(2026, time.September, 23),
		date(2026, time.September, 24),
		date(2026, time.September, 25),
	}

	if len(got) != 5 {
		t.Fatalf("got %d dates, want 5", len(got))
	}
	for i, w := range want {
		gotDate := got[i].Format("2006-01-02")
		wantDate := w.Format("2006-01-02")
		if gotDate != wantDate {
			t.Errorf("got[%d] = %s, want %s", i, gotDate, wantDate)
		}
	}
}

func TestGetPreviousWeekWorkdays_AllWeekdaysReturnSameWeek(t *testing.T) {
	// Running Mon-Fri of Sep 28 week should all return Sep 21-25
	wantMonday := "2026-09-21"
	wantFriday := "2026-09-25"

	for _, day := range []struct {
		name string
		date time.Time
	}{
		{"Monday", date(2026, time.September, 28)},
		{"Tuesday", date(2026, time.September, 29)},
		{"Wednesday", date(2026, time.September, 30)},
		{"Thursday", date(2026, time.October, 1)},
		{"Friday", date(2026, time.October, 2)},
	} {
		t.Run(day.name, func(t *testing.T) {
			got := getPreviousWeekWorkdays(day.date)
			if len(got) != 5 {
				t.Fatalf("got %d dates, want 5", len(got))
			}
			if got[0].Format("2006-01-02") != wantMonday {
				t.Errorf("first = %s, want %s", got[0].Format("2006-01-02"), wantMonday)
			}
			if got[4].Format("2006-01-02") != wantFriday {
				t.Errorf("last = %s, want %s", got[4].Format("2006-01-02"), wantFriday)
			}
		})
	}
}

func TestGetPreviousWeekWorkdays_Weekend(t *testing.T) {
	// Sat Sep 26 and Sun Sep 27 are part of the Sep 21 week
	// Previous week = Sep 14-18
	for _, day := range []struct {
		name string
		date time.Time
	}{
		{"Saturday", date(2026, time.September, 26)},
		{"Sunday", date(2026, time.September, 27)},
	} {
		t.Run(day.name, func(t *testing.T) {
			got := getPreviousWeekWorkdays(day.date)
			if len(got) != 5 {
				t.Fatalf("got %d dates, want 5", len(got))
			}
			if got[0].Format("2006-01-02") != "2026-09-14" {
				t.Errorf("first = %s, want 2026-09-14", got[0].Format("2006-01-02"))
			}
			if got[4].Format("2006-01-02") != "2026-09-18" {
				t.Errorf("last = %s, want 2026-09-18", got[4].Format("2006-01-02"))
			}
		})
	}
}

func TestGetPreviousWeekWorkdays_AllDatesAreWeekdays(t *testing.T) {
	today := date(2026, time.September, 30)
	got := getPreviousWeekWorkdays(today)

	for i, d := range got {
		wd := d.Weekday()
		if wd == time.Saturday || wd == time.Sunday {
			t.Errorf("got[%d] = %s (%s), expected a weekday", i, d.Format("2006-01-02"), wd)
		}
	}
}

func TestGetPreviousWeekWorkdays_CrossMonthBoundary(t *testing.T) {
	// Mon Oct 5 2026 → previous week is Sep 28 - Oct 2
	today := date(2026, time.October, 5)
	got := getPreviousWeekWorkdays(today)

	if got[0].Format("2006-01-02") != "2026-09-28" {
		t.Errorf("first = %s, want 2026-09-28", got[0].Format("2006-01-02"))
	}
	if got[4].Format("2006-01-02") != "2026-10-02" {
		t.Errorf("last = %s, want 2026-10-02", got[4].Format("2006-01-02"))
	}
}

func TestGetPreviousWeekWorkdays_CrossYearBoundary(t *testing.T) {
	// Mon Jan 5 2026 → previous week is Dec 29, 2025 - Jan 2, 2026
	today := date(2026, time.January, 5)
	got := getPreviousWeekWorkdays(today)

	if got[0].Format("2006-01-02") != "2025-12-29" {
		t.Errorf("first = %s, want 2025-12-29", got[0].Format("2006-01-02"))
	}
	if got[4].Format("2006-01-02") != "2026-01-02" {
		t.Errorf("last = %s, want 2026-01-02", got[4].Format("2006-01-02"))
	}
}
