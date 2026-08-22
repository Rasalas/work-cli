// Package calendar provides shared local-time date helpers used by both
// the CLI output layer and the database balance calculations. Keeping a
// single implementation ensures week boundaries are computed identically
// everywhere.
package calendar

import "time"

// DayStart returns midnight of t's local day, in t's location.
func DayStart(t time.Time) time.Time {
	local := t.Local()
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
}

// WeekStart returns midnight of the Monday of t's local week.
func WeekStart(t time.Time) time.Time {
	start := DayStart(t)
	offset := (int(start.Weekday()) + 6) % 7
	return start.AddDate(0, 0, -offset)
}
