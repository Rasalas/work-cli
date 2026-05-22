package tui

import (
	"fmt"

	"github.com/Rasalas/work-cli/internal/db"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type projectItem struct {
	project db.Project
}

func (i projectItem) FilterValue() string { return i.project.Name }
func (i projectItem) Title() string       { return i.project.Name }
func (i projectItem) Description() string { return "" }

type pickerModel struct {
	list     list.Model
	selected *db.Project
	quit     bool
}

func (m pickerModel) Init() tea.Cmd {
	return nil
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.quit = true
			return m, tea.Quit
		case "enter":
			if item, ok := m.list.SelectedItem().(projectItem); ok {
				project := item.project
				m.selected = &project
			}
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m pickerModel) View() string {
	if m.quit || m.selected != nil {
		return ""
	}
	return m.list.View()
}

func PickProject(projects []db.Project) (*db.Project, error) {
	items := make([]list.Item, 0, len(projects))
	for _, project := range projects {
		items = append(items, projectItem{project: project})
	}

	l := list.New(items, list.NewDefaultDelegate(), 44, 12)
	l.Title = "Project"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)

	model := pickerModel{list: l}
	result, err := tea.NewProgram(model).Run()
	if err != nil {
		return nil, err
	}
	final, ok := result.(pickerModel)
	if !ok {
		return nil, fmt.Errorf("unexpected picker result")
	}
	return final.selected, nil
}
