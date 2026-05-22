package timeparse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var digitsOnly = regexp.MustCompile(`^\d{1,4}$`)

// ParseWorkDuration parses convenient work-duration forms.
// Bare numbers such as "5" are interpreted as hours.
func ParseWorkDuration(input string) (time.Duration, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, fmt.Errorf("duration is required")
	}

	if duration, err := time.ParseDuration(input); err == nil {
		if duration <= 0 {
			return 0, fmt.Errorf("duration must be positive")
		}
		return duration, nil
	}

	if strings.Contains(input, ":") {
		parts := strings.Split(input, ":")
		if len(parts) != 2 {
			return 0, fmt.Errorf("invalid duration %q", input)
		}
		hours, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid hours in %q", input)
		}
		minutes, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("invalid minutes in %q", input)
		}
		if hours < 0 || minutes < 0 || minutes > 59 || hours == 0 && minutes == 0 {
			return 0, fmt.Errorf("invalid duration %q", input)
		}
		return time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute, nil
	}

	hours, err := strconv.ParseFloat(input, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", input)
	}
	if hours <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return time.Duration(hours * float64(time.Hour)), nil
}

// ParseStartTime parses convenient CLI time forms into a concrete timestamp.
// Bare times such as "800", "8", or "08:30" are interpreted for base's date.
func ParseStartTime(input string, base time.Time) (time.Time, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return base, nil
	}

	location := base.Location()
	date := time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, location)

	for _, layout := range []string{
		"2006-01-02 15:04",
		"2006-01-02 1504",
		"2006-01-02 15",
		time.RFC3339,
	} {
		if parsed, err := time.ParseInLocation(layout, input, location); err == nil {
			return parsed, nil
		}
	}

	if strings.Contains(input, ":") {
		parts := strings.Split(input, ":")
		if len(parts) != 2 {
			return time.Time{}, fmt.Errorf("invalid time %q", input)
		}
		hour, err := strconv.Atoi(parts[0])
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid hour in %q", input)
		}
		minute, err := strconv.Atoi(parts[1])
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid minute in %q", input)
		}
		return withClock(date, hour, minute, input)
	}

	if digitsOnly.MatchString(input) {
		n, err := strconv.Atoi(input)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid time %q", input)
		}

		var hour, minute int
		switch len(input) {
		case 1, 2:
			hour = n
		case 3:
			hour = n / 100
			minute = n % 100
		case 4:
			hour = n / 100
			minute = n % 100
		default:
			return time.Time{}, fmt.Errorf("invalid time %q", input)
		}
		return withClock(date, hour, minute, input)
	}

	return time.Time{}, fmt.Errorf("invalid time %q", input)
}

func withClock(date time.Time, hour, minute int, original string) (time.Time, error) {
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return time.Time{}, fmt.Errorf("invalid time %q", original)
	}
	return time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, date.Location()), nil
}
