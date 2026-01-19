package common

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SearchModalClosedMsg is sent when the search modal is closed
type SearchModalClosedMsg struct {
	Query string
}

// SearchNextMsg is sent when user wants to go to next match
type SearchNextMsg struct{}

// SearchPrevMsg is sent when user wants to go to previous match
type SearchPrevMsg struct{}

// SearchClearMsg is sent when search is cleared
type SearchClearMsg struct{}

// SearchModal represents the search input modal
type SearchModal struct {
	visible      bool
	width        int
	height       int
	input        textinput.Model
	matchCount   int
	currentMatch int
}

// NewSearchModal creates a new search modal
func NewSearchModal() SearchModal {
	ti := textinput.New()
	ti.Placeholder = "Search logs..."
	ti.CharLimit = 100
	ti.Width = 40

	return SearchModal{
		visible: false,
		input:   ti,
	}
}

// Open opens the modal
func (m *SearchModal) Open() tea.Cmd {
	m.visible = true
	m.input.Focus()
	m.input.SetValue("")
	m.matchCount = 0
	m.currentMatch = 0
	return textinput.Blink
}

// Close closes the modal
func (m *SearchModal) Close() {
	m.visible = false
	m.input.Blur()
}

// IsVisible returns whether the modal is visible
func (m SearchModal) IsVisible() bool {
	return m.visible
}

// GetQuery returns the current search query
func (m SearchModal) GetQuery() string {
	return m.input.Value()
}

// SetMatchInfo sets the match count information
func (m *SearchModal) SetMatchInfo(current, total int) {
	m.currentMatch = current
	m.matchCount = total
}

// SetSize sets the modal dimensions
func (m *SearchModal) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Update handles messages for the modal
func (m SearchModal) Update(msg tea.Msg) (SearchModal, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.visible = false
			m.input.Blur()
			// Clear search on escape
			return m, func() tea.Msg { return SearchClearMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			m.visible = false
			m.input.Blur()
			query := m.input.Value()
			return m, func() tea.Msg { return SearchModalClosedMsg{Query: query} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+n"))):
			// Next match
			return m, func() tea.Msg { return SearchNextMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+p"))):
			// Previous match
			return m, func() tea.Msg { return SearchPrevMsg{} }

		default:
			// Update the text input
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)
			// Send search update
			query := m.input.Value()
			cmds = append(cmds, func() tea.Msg { return SearchModalClosedMsg{Query: query} })
		}
	}

	return m, tea.Batch(cmds...)
}

// View renders the search bar (single line, positioned at top like Tempo's filter)
func (m SearchModal) View(screenWidth, screenHeight int) string {
	if !m.visible {
		return ""
	}

	// Build the search bar content
	var parts []string

	// Search prefix indicator (like Tempo's "/")
	prefix := lipgloss.NewStyle().
		Foreground(lipgloss.Color("117")).
		Bold(true).
		Render("/")
	parts = append(parts, prefix)

	// Search input
	parts = append(parts, m.input.View())

	// Match count (if searching)
	if m.input.Value() != "" {
		if m.matchCount > 0 {
			matchInfo := lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Render(fmt.Sprintf(" %d/%d", m.currentMatch, m.matchCount))
			parts = append(parts, matchInfo)
		} else {
			noMatch := lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")).
				Render(" No matches")
			parts = append(parts, noMatch)
		}
	}

	// Help text (right-aligned)
	helpText := MutedInlineStyle.Render("  enter:confirm esc:clear ctrl+n/p:next/prev")
	parts = append(parts, helpText)

	content := strings.Join(parts, "")

	// Style the entire bar
	barStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1).
		Width(screenWidth)

	return barStyle.Render(content)
}
