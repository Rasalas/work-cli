package timeparse

import (
	"testing"
	"time"
)

func TestParseStartTime(t *testing.T) {
	base := time.Date(2026, 5, 22, 14, 30, 0, 0, time.Local)

	tests := map[string]string{
		"8":                "2026-05-22 08:00",
		"800":              "2026-05-22 08:00",
		"0800":             "2026-05-22 08:00",
		"8:30":             "2026-05-22 08:30",
		"2026-05-21 09:15": "2026-05-21 09:15",
	}

	for input, want := range tests {
		got, err := ParseStartTime(input, base)
		if err != nil {
			t.Fatalf("ParseStartTime(%q): %v", input, err)
		}
		if got.Format("2006-01-02 15:04") != want {
			t.Fatalf("ParseStartTime(%q) = %s, want %s", input, got.Format("2006-01-02 15:04"), want)
		}
	}
}
