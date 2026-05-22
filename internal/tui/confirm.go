package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type confirmModel struct {
	title    string
	content  string
	question string
	cursor   int
	answer   bool
	aborted  bool
	done     bool
}

func newConfirmModel(title, content, question string) confirmModel {
	return confirmModel{
		title:    title,
		content:  content,
		question: question,
	}
}

func (m confirmModel) Init() tea.Cmd { return nil }

func (m confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.aborted = true
			return m, tea.Quit
		case "left", "h":
			m.cursor = 0
		case "right", "l":
			m.cursor = 1
		case "tab":
			m.cursor = (m.cursor + 1) % 2
		case "y":
			m.answer = true
			m.done = true
			return m, tea.Quit
		case "n":
			m.answer = false
			m.done = true
			return m, tea.Quit
		case "enter":
			m.answer = m.cursor == 0
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m confirmModel) View() string {
	var b strings.Builder
	b.WriteString("\n")

	var card strings.Builder
	if m.title != "" {
		card.WriteString(confirmTitle(m.title))
		card.WriteString("\n\n")
	}
	card.WriteString(m.content)

	b.WriteString(confirmContentStyle.Render(card.String()))
	b.WriteString("\n\n")
	b.WriteString(m.question)
	b.WriteString("\n\n")

	yesButton, noButton := confirmButtons(m.cursor)
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Center, yesButton, "  ", noButton))
	b.WriteString("\n\n")
	b.WriteString(confirmHelpStyle.Render("<- -> toggle • enter submit • y Yes • n No"))
	b.WriteString("\n")
	return b.String()
}

func confirmButtons(cursor int) (string, string) {
	if cursor == 0 {
		return confirmYesActiveStyle.Render("Yes"), confirmNoDimStyle.Render("No")
	}
	return confirmYesDimStyle.Render("Yes"), confirmNoActiveStyle.Render("No")
}

func confirmTitle(title string) string {
	badge, detail, ok := strings.Cut(title, " ")
	if !ok || detail == "" {
		return confirmTitleBadge(title)
	}
	return confirmTitleBadge(badge) + "  " + confirmTitleDetail(detail)
}

func confirmTitleBadge(text string) string {
	if _, noColor := os.LookupEnv("NO_COLOR"); noColor {
		return "  " + text + "  "
	}
	return "\033[1m\033[48;5;203m\033[38;5;230m  " + text + "  \033[22m\033[48;5;235m\033[38;5;252m"
}

func confirmTitleDetail(text string) string {
	if _, noColor := os.LookupEnv("NO_COLOR"); noColor {
		return text
	}
	return "\033[1m\033[38;5;252m" + text + "\033[22m\033[39m\033[48;5;235m"
}

// ConfirmWithContent shows a Divekit-style confirmation dialog with content and Yes/No buttons.
func ConfirmWithContent(title, content, question string) (bool, error) {
	result, err := tea.NewProgram(newConfirmModel(title, content, question)).Run()
	if err != nil {
		return false, err
	}
	final, ok := result.(confirmModel)
	if !ok {
		return false, fmt.Errorf("unexpected confirmation result")
	}
	if final.aborted {
		return false, nil
	}
	return final.answer, nil
}

var (
	confirmContentStyle = lipgloss.NewStyle().
				Border(lipgloss.ThickBorder(), false, false, false, true).
				BorderForeground(lipgloss.Color("203")).
				Background(lipgloss.Color("235")).
				Foreground(lipgloss.Color("252")).
				Padding(1, 2, 1, 2)
	confirmYesActiveStyle = lipgloss.NewStyle().
				Padding(0, 2).
				Background(lipgloss.Color("30")).
				Foreground(lipgloss.Color("230"))
	confirmYesDimStyle = lipgloss.NewStyle().
				Padding(0, 2).
				Foreground(lipgloss.Color("30"))
	confirmNoActiveStyle = lipgloss.NewStyle().
				Padding(0, 2).
				Background(lipgloss.Color("203")).
				Foreground(lipgloss.Color("230"))
	confirmNoDimStyle = lipgloss.NewStyle().
				Padding(0, 2).
				Foreground(lipgloss.Color("203"))
	confirmHelpStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("246")).
				Italic(true)
)
