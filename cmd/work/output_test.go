package work

import (
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	tests := map[string]struct {
		duration time.Duration
		want     string
	}{
		"negative":       {-time.Minute, "0m"},
		"zero":           {0, "0m"},
		"minutes":        {42 * time.Minute, "42m"},
		"rounds minutes": {90*time.Minute + 31*time.Second, "1h 31m"},
		"whole hours":    {2 * time.Hour, "2h"},
		"hours minutes":  {2*time.Hour + 15*time.Minute, "2h 15m"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := formatDuration(test.duration)
			if got != test.want {
				t.Fatalf("formatDuration(%s) = %q, want %q", test.duration, got, test.want)
			}
		})
	}
}
