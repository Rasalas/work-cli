package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/Rasalas/work-cli/internal/db"
)

var out io.Writer = os.Stdout

var (
	accentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("30")).
			Padding(0, 2)
	logDurationStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("30")).
				Padding(0, 2)
	mutedBlockStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("246")).
			Padding(0, 2)
	lineStyle = lipgloss.NewStyle().
			Padding(0, 2)
	keyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("246")).
			Width(outputKeyWidth).
			Align(lipgloss.Right)
	metaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("246"))
	noteKindStyle = metaStyle.
			Width(noteKindWidth)
	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
	sectionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("30")).
			Bold(true).
			Padding(0, 2)
)

const (
	outputKeyWidth = 11
	noteKindWidth  = 5
)

func printNotes(notes []db.Note) {
	for _, note := range notes {
		printLine(noteLine(note))
	}
	fmt.Fprintln(out)
}

func printBlock(lines ...string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintln(out)
	for _, text := range lines {
		fmt.Fprintln(out, lineStyle.Render(text))
	}
	fmt.Fprintln(out)
}

func printMuted(lines ...string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintln(out)
	for _, text := range lines {
		fmt.Fprintln(out, mutedBlockStyle.Render(text))
	}
	fmt.Fprintln(out)
}

func printSection(title string) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, sectionStyle.Render(title))
	fmt.Fprintln(out)
}

func printLine(lines ...string) {
	for _, text := range lines {
		fmt.Fprintln(out, lineStyle.Render(text))
	}
}

func line(key, value string) string {
	if key == "" {
		return valueStyle.Render(value)
	}
	return keyStyle.Render(key) + "  " + valueStyle.Render(value)
}

func badgeLine(badge, value string) string {
	return accentStyle.Render(badge) + "  " + valueStyle.Render(value)
}

func noteLine(note db.Note) string {
	return metaStyle.Render(formatClock(note.CreatedAt)) +
		"  " +
		noteKindStyle.Render(note.Kind) +
		"  " +
		valueStyle.Render(note.Body)
}

func formatDateTime(t time.Time) string {
	return t.Local().Format("2006-01-02 15:04")
}

func formatClock(t time.Time) string {
	return t.Local().Format("15:04")
}

func formatEnd(session *db.Session) string {
	if session.EndedAt.Valid {
		return formatDateTime(session.EndedAt.Time)
	}
	return "running"
}

func formatSessionDuration(session db.Session, now time.Time) string {
	end := now
	if session.EndedAt.Valid {
		end = session.EndedAt.Time
	}
	return formatDuration(end.Sub(session.StartedAt))
}

func formatDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	minutes := int(duration.Round(time.Minute).Minutes())
	hours := minutes / 60
	minutes = minutes % 60
	if hours == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}
