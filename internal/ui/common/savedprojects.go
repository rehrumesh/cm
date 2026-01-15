package common

import (
	"fmt"
	"sort"
	"strings"

	"cm/internal/config"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SavedProjectsClosedMsg is sent when the saved projects modal is closed
type SavedProjectsClosedMsg struct {
	Changed bool
}

// SavedProjectsModal represents the saved projects management modal
type SavedProjectsModal struct {
	visible  bool
	width    int
	height   int
	proj     *config.Projects
	projects []savedProject
	cursor   int
	selected map[string]bool
}

type savedProject struct {
	name       string
	workingDir string
}

// NewSavedProjectsModal creates a new saved projects modal
func NewSavedProjectsModal() SavedProjectsModal {
	return SavedProjectsModal{
		visible:  false,
		selected: make(map[string]bool),
	}
}

// Open opens the modal and loads saved projects
func (m *SavedProjectsModal) Open() tea.Cmd {
	m.proj = config.LoadProjects()
	m.projects = make([]savedProject, 0, len(m.proj.SavedProjects))

	// Sort projects by name
	names := make([]string, 0, len(m.proj.SavedProjects))
	for name := range m.proj.SavedProjects {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		proj := m.proj.SavedProjects[name]
		m.projects = append(m.projects, savedProject{
			name:       name,
			workingDir: proj.WorkingDir,
		})
	}

	m.visible = true
	m.cursor = 0
	m.selected = make(map[string]bool)

	return nil
}

// Close closes the modal
func (m *SavedProjectsModal) Close() {
	m.visible = false
}

// IsVisible returns whether the modal is visible
func (m SavedProjectsModal) IsVisible() bool {
	return m.visible
}

// SetSize sets the modal dimensions
func (m *SavedProjectsModal) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Update handles messages for the modal
func (m SavedProjectsModal) Update(msg tea.Msg) (SavedProjectsModal, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.visible = false
			return m, func() tea.Msg { return SavedProjectsClosedMsg{Changed: false} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if m.cursor < len(m.projects)-1 {
				m.cursor++
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys(" "))):
			// Toggle selection
			if m.cursor < len(m.projects) {
				name := m.projects[m.cursor].name
				if m.selected[name] {
					delete(m.selected, name)
				} else {
					m.selected[name] = true
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
			// Select all
			for _, p := range m.projects {
				m.selected[p.name] = true
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("A"))):
			// Clear selection
			m.selected = make(map[string]bool)

		case key.Matches(msg, key.NewBinding(key.WithKeys("d", "backspace", "delete"))):
			// Remove selected projects
			if len(m.selected) > 0 {
				for name := range m.selected {
					m.proj.RemoveProject(name)
				}
				if err := m.proj.Save(); err == nil {
					m.visible = false
					return m, func() tea.Msg { return SavedProjectsClosedMsg{Changed: true} }
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			// Remove item under cursor if nothing selected
			if len(m.selected) == 0 && m.cursor < len(m.projects) {
				name := m.projects[m.cursor].name
				m.proj.RemoveProject(name)
				if err := m.proj.Save(); err == nil {
					m.visible = false
					return m, func() tea.Msg { return SavedProjectsClosedMsg{Changed: true} }
				}
			} else if len(m.selected) > 0 {
				// Remove selected
				for name := range m.selected {
					m.proj.RemoveProject(name)
				}
				if err := m.proj.Save(); err == nil {
					m.visible = false
					return m, func() tea.Msg { return SavedProjectsClosedMsg{Changed: true} }
				}
			}
		}
	}

	return m, nil
}

// View renders the modal
func (m SavedProjectsModal) View(screenWidth, screenHeight int) string {
	if !m.visible {
		return ""
	}

	var content strings.Builder

	// Title
	content.WriteString(ModalTitleStyle.Render("Saved Projects"))
	content.WriteString("\n\n")

	if len(m.projects) == 0 {
		content.WriteString(MutedInlineStyle.Render("  No saved projects"))
		content.WriteString("\n\n")
	} else {
		// Show selected count
		if len(m.selected) > 0 {
			content.WriteString(HelpKeyStyle.Render(fmt.Sprintf("  %d selected", len(m.selected))))
			content.WriteString("\n\n")
		}

		// List projects
		maxVisible := 10
		start := 0
		if m.cursor >= maxVisible {
			start = m.cursor - maxVisible + 1
		}

		for i := start; i < len(m.projects) && i < start+maxVisible; i++ {
			proj := m.projects[i]

			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}

			checkbox := "[ ]"
			if m.selected[proj.name] {
				checkbox = CheckedStyle.Render("[x]")
			}

			// Truncate working dir if too long
			dir := proj.workingDir
			maxDirLen := 40
			if len(dir) > maxDirLen {
				dir = "..." + dir[len(dir)-maxDirLen+3:]
			}

			line := fmt.Sprintf("%s%s %s", cursor, checkbox, proj.name)
			if i == m.cursor {
				line = ModalSelectedStyle.Render(line)
				content.WriteString(line)
				content.WriteString(MutedInlineStyle.Render(fmt.Sprintf("\n       %s", dir)))
			} else {
				content.WriteString(line)
			}
			content.WriteString("\n")
		}

		if len(m.projects) > maxVisible {
			content.WriteString(MutedInlineStyle.Render(fmt.Sprintf("\n  ... and %d more", len(m.projects)-maxVisible)))
		}
	}

	content.WriteString("\n")

	// Help
	content.WriteString(MutedInlineStyle.Render("  j/k:nav  space:sel  a/A:all/clr  d/⏎:remove  esc:close"))

	// Style the modal
	modalContent := ModalStyle.Render(content.String())

	// Get modal dimensions
	modalWidth := lipgloss.Width(modalContent)
	modalHeight := lipgloss.Height(modalContent)

	// Center the modal
	x := (screenWidth - modalWidth) / 2
	y := (screenHeight - modalHeight) / 2

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	// Create positioned modal
	positioned := lipgloss.NewStyle().
		MarginLeft(x).
		MarginTop(y).
		Render(modalContent)

	return positioned
}
