package calendar

import (
	"testing"
	"time"
)

func TestDayStart(t *testing.T) {
	in := time.Date(2026, 5, 21, 18, 27, 43, 0, time.Local)
	got := DayStart(in)
	want := time.Date(2026, 5, 21, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("DayStart() = %s, want %s", got, want)
	}
	if got.Location() != want.Location() {
		t.Fatalf("DayStart() location = %s, want %s", got.Location(), want.Location())
	}
}

func TestWeekStartReturnsMondayMidnight(t *testing.T) {
	tests := []struct {
		name string
		day  time.Time
		want time.Time
	}{
		{"monday", time.Date(2026, 5, 18, 9, 0, 0, 0, time.Local), time.Date(2026, 5, 18, 0, 0, 0, 0, time.Local)},
		{"wednesday", time.Date(2026, 5, 20, 23, 59, 0, 0, time.Local), time.Date(2026, 5, 18, 0, 0, 0, 0, time.Local)},
		{"sunday", time.Date(2026, 5, 24, 12, 0, 0, 0, time.Local), time.Date(2026, 5, 18, 0, 0, 0, 0, time.Local)},
		// ISO week boundary across a year: 2027-01-01 is a Friday and
		// belongs to the week starting Monday 2026-12-28.
		{"new-year-friday", time.Date(2027, 1, 1, 8, 0, 0, 0, time.Local), time.Date(2026, 12, 28, 0, 0, 0, 0, time.Local)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WeekStart(tt.day); !got.Equal(tt.want) {
				t.Fatalf("WeekStart(%s) = %s, want %s", tt.day, got, tt.want)
			}
		})
	}
}
