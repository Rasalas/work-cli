package cli

import (
	"bytes"
	"encoding/csv"
	"testing"
	"time"

	"github.com/Rasalas/work-cli/internal/db"
)

func TestFormatExportDurationRoundsToMinutes(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "00:00"},
		{30 * time.Second, "00:01"},
		{-time.Second, "00:00"},
		{8 * time.Hour, "08:00"},
		{8*time.Hour + 30*time.Second, "08:01"},
		{7*time.Hour + 59*time.Minute + 40*time.Second, "08:00"},
	}
	for _, tt := range tests {
		if got := formatExportDuration(tt.in); got != tt.want {
			t.Fatalf("formatExportDuration(%s) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestExportRange(t *testing.T) {
	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.Local)

	from, to, err := exportRange("2026-05-18", "", now)
	if err != nil {
		t.Fatalf("exportRange(date) error = %v", err)
	}
	if want := time.Date(2026, 5, 18, 0, 0, 0, 0, time.Local); !from.Equal(want) || !to.Equal(want.AddDate(0, 0, 1)) {
		t.Fatalf("exportRange(date) = %s..%s, want %s..%s", from, to, want, want.AddDate(0, 0, 1))
	}

	from, to, err = exportRange("", "2026-02", now)
	if err != nil {
		t.Fatalf("exportRange(month) error = %v", err)
	}
	if want := time.Date(2026, 2, 1, 0, 0, 0, 0, time.Local); !from.Equal(want) || !to.Equal(want.AddDate(0, 1, 0)) {
		t.Fatalf("exportRange(month) = %s..%s, want %s..%s", from, to, want, want.AddDate(0, 1, 0))
	}

	if _, _, err = exportRange("", "", now); err != nil {
		t.Fatalf("exportRange(default) error = %v", err)
	}
	if _, _, err = exportRange("not-a-date", "", now); err == nil {
		t.Fatal("exportRange(invalid date) should fail")
	}
	if _, _, err = exportRange("", "2026-13", now); err == nil {
		t.Fatal("exportRange(invalid month) should fail")
	}
}

func csvRows(t *testing.T, buf bytes.Buffer) [][]string {
	t.Helper()
	reader := csv.NewReader(&buf)
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	return rows
}

func TestWriteExportDaySplitsWorkAndOvertimeUse(t *testing.T) {
	day := exportDay{
		Date:         time.Date(2026, 5, 20, 0, 0, 0, 0, time.Local),
		OvertimeUsed: 90 * time.Minute,
	}
	settings := db.ProjectExportSettings{ReportStart: "09:00", ReportEnd: "17:00"}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writeExportDay(writer, day, settings, true); err != nil {
		t.Fatalf("writeExportDay() error = %v", err)
	}
	writer.Flush()

	rows := csvRows(t, buf)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %v", len(rows), rows)
	}
	if rows[0][3] != "work" || rows[0][4] != "06:30" {
		t.Fatalf("work row = %v, want work 06:30", rows[0])
	}
	if rows[1][3] != "overtime_use" || rows[1][4] != "01:30" {
		t.Fatalf("overtime row = %v, want overtime_use 01:30", rows[1])
	}
	if rows[1][0] != "2026-05-20" {
		t.Fatalf("date field = %q", rows[1][0])
	}
}

func TestWriteExportDayRejectsOvertimeBeyondWindow(t *testing.T) {
	day := exportDay{
		Date:         time.Date(2026, 5, 20, 0, 0, 0, 0, time.Local),
		OvertimeUsed: 9 * time.Hour,
	}
	settings := db.ProjectExportSettings{ReportStart: "09:00", ReportEnd: "17:00"}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writeExportDay(writer, day, settings, true); err == nil {
		t.Fatal("writeExportDay() with overtime beyond window should fail")
	}
}
