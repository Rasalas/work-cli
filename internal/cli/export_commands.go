package cli

import (
	"context"
	"encoding/csv"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/Rasalas/work-cli/internal/db"
)

type exportDay struct {
	Date         time.Time
	OvertimeUsed time.Duration
}

func exportCmd() *cobra.Command {
	var opts struct {
		project      string
		date         string
		month        string
		showOvertime bool
	}
	cmd := &cobra.Command{
		Use:   "export [project]",
		Short: "Export employer work-time records as CSV",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.project != "" && len(args) > 0 {
				return fmt.Errorf("use either --project or positional project")
			}
			if opts.date != "" && opts.month != "" {
				return fmt.Errorf("use either --date or --month")
			}

			from, to, err := exportRange(opts.date, opts.month, time.Now())
			if err != nil {
				return err
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			ctx := context.Background()
			projectArgs := args
			if opts.project != "" {
				projectArgs = []string{opts.project}
			}
			project, err := resolveProjectCommandProject(ctx, store, projectArgs, "work export <projectname>")
			if err != nil {
				return err
			}
			settings, err := store.ProjectExportSettings(ctx, project.ID)
			if err != nil {
				return err
			}
			if settings == nil {
				return fmt.Errorf("project %q has no reporting window; configure it with `work project set %s --report-start <time> --report-end <time>`", project.Name, project.Name)
			}

			days, err := loadExportDays(ctx, store, project, from, to)
			if err != nil {
				return err
			}
			writer := csv.NewWriter(out)
			if err := writer.Write([]string{"date", "start", "end", "type", "duration"}); err != nil {
				return err
			}
			for _, day := range days {
				if err := writeExportDay(writer, day, *settings, opts.showOvertime); err != nil {
					return err
				}
			}
			writer.Flush()
			return writer.Error()
		},
	}
	cmd.Flags().StringVarP(&opts.project, "project", "p", "", "project name")
	cmd.Flags().StringVar(&opts.date, "date", "", "export YYYY-MM-DD")
	cmd.Flags().StringVar(&opts.month, "month", "", "export YYYY-MM (defaults to current month)")
	cmd.Flags().BoolVar(&opts.showOvertime, "show-overtime", false, "split reported time into work and overtime-use rows")
	return cmd
}

func exportRange(dateInput, monthInput string, now time.Time) (time.Time, time.Time, error) {
	if dateInput != "" {
		from, err := parseLogDate(dateInput, now.Location())
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		return from, from.AddDate(0, 0, 1), nil
	}
	if monthInput == "" {
		monthInput = now.Format("2006-01")
	}
	from, err := time.ParseInLocation("2006-01", monthInput, now.Location())
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid month %q; use YYYY-MM", monthInput)
	}
	return from, from.AddDate(0, 1, 0), nil
}

func loadExportDays(ctx context.Context, store *db.Store, project db.Project, from, to time.Time) ([]exportDay, error) {
	sessions, err := store.LogSessions(ctx, &from, &to, project.Name)
	if err != nil {
		return nil, err
	}
	usages, err := store.OvertimeUsages(ctx, project.ID, &from, &to)
	if err != nil {
		return nil, err
	}

	days := make(map[string]*exportDay)
	for _, session := range sessions {
		date := dayStart(session.StartedAt)
		key := date.Format("2006-01-02")
		day := days[key]
		if day == nil {
			day = &exportDay{Date: date}
			days[key] = day
		}
	}
	for _, usage := range usages {
		key := usage.UsedOn.Format("2006-01-02")
		day := days[key]
		if day == nil {
			day = &exportDay{Date: usage.UsedOn}
			days[key] = day
		}
		day.OvertimeUsed += usage.Duration
	}

	keys := make([]string, 0, len(days))
	for key := range days {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]exportDay, 0, len(keys))
	for _, key := range keys {
		result = append(result, *days[key])
	}
	return result, nil
}

func writeExportDay(writer *csv.Writer, day exportDay, settings db.ProjectExportSettings, showOvertime bool) error {
	reportStart, reportEnd, err := reportWindowForDate(day.Date, settings)
	if err != nil {
		return err
	}
	window := reportEnd.Sub(reportStart)
	if !showOvertime {
		return writer.Write(exportRecord(day.Date, reportStart, reportEnd, "work", window))
	}

	cursor := reportStart
	used := roundedMinutes(day.OvertimeUsed)
	if used > window {
		return fmt.Errorf("overtime used on %s exceeds reporting window", day.Date.Format("2006-01-02"))
	}
	worked := window - used
	if worked > 0 {
		end := cursor.Add(worked)
		if err := writer.Write(exportRecord(day.Date, cursor, end, "work", worked)); err != nil {
			return err
		}
		cursor = end
	}
	if used > 0 {
		if err := writer.Write(exportRecord(day.Date, cursor, reportEnd, "overtime_use", used)); err != nil {
			return err
		}
	}
	return nil
}

func reportWindowForDate(date time.Time, settings db.ProjectExportSettings) (time.Time, time.Time, error) {
	startHour, startMinute, err := parseStoredClock(settings.ReportStart)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	endHour, endMinute, err := parseStoredClock(settings.ReportEnd)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	start := time.Date(date.Year(), date.Month(), date.Day(), startHour, startMinute, 0, 0, date.Location())
	end := time.Date(date.Year(), date.Month(), date.Day(), endHour, endMinute, 0, 0, date.Location())
	if !end.After(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid reporting window %s - %s", settings.ReportStart, settings.ReportEnd)
	}
	return start, end, nil
}

func parseStoredClock(value string) (int, int, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, 0, err
	}
	return parsed.Hour(), parsed.Minute(), nil
}

func exportRecord(date, start, end time.Time, kind string, duration time.Duration) []string {
	return []string{
		date.Format("2006-01-02"),
		start.Format("15:04"),
		end.Format("15:04"),
		kind,
		formatExportDuration(duration),
	}
}

func formatExportDuration(duration time.Duration) string {
	minutes := int(roundedMinutes(duration) / time.Minute)
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

func roundedMinutes(duration time.Duration) time.Duration {
	return duration.Round(time.Minute)
}
