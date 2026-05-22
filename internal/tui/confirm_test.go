package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestConfirmModelConfirmsWithEnterOnDefaultYes(t *testing.T) {
	model := newConfirmModel("DELETE Session #1", "time  08:00 - 09:00", "Delete this session?")

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	final := updated.(confirmModel)

	if !final.done {
		t.Fatal("done = false, want true")
	}
	if !final.answer {
		t.Fatal("answer = false, want true")
	}
}

func TestConfirmModelSelectsNoWithRightAndEnter(t *testing.T) {
	model := newConfirmModel("DELETE Session #1", "time  08:00 - 09:00", "Delete this session?")

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated, _ = updated.(confirmModel).Update(tea.KeyMsg{Type: tea.KeyEnter})
	final := updated.(confirmModel)

	if !final.done {
		t.Fatal("done = false, want true")
	}
	if final.answer {
		t.Fatal("answer = true, want false")
	}
}

func TestConfirmModelViewIncludesContentAndControls(t *testing.T) {
	view := newConfirmModel("DELETE Session #1", "time  08:00 - 09:00", "Delete this session?").View()

	for _, want := range []string{"DELETE", "Session #1", "time", "Delete this session?", "Yes", "No", "enter submit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q:\n%s", want, view)
		}
	}
}
