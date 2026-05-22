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

func TestParseWorkDuration(t *testing.T) {
	tests := map[string]time.Duration{
		"5":    5 * time.Hour,
		"5h":   5 * time.Hour,
		"5:30": 5*time.Hour + 30*time.Minute,
		"90m":  90 * time.Minute,
		"7.5":  7*time.Hour + 30*time.Minute,
	}

	for input, want := range tests {
		got, err := ParseWorkDuration(input)
		if err != nil {
			t.Fatalf("ParseWorkDuration(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseWorkDuration(%q) = %s, want %s", input, got, want)
		}
	}
}

func TestParseWorkDurationRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{"", "0", "-1h", "5:99", "nope"} {
		t.Run(input, func(t *testing.T) {
			if _, err := ParseWorkDuration(input); err == nil {
				t.Fatalf("ParseWorkDuration(%q) error = nil", input)
			}
		})
	}
}
