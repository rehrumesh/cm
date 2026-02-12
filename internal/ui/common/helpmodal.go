package common

import (
	"strings"

	"cm/internal/config"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// HelpModalClosedMsg is sent when the help modal is closed
type HelpModalClosedMsg struct{}

// HelpModal represents the help modal
type HelpModal struct {
	visible bool
	width   int
	height  int
	scroll  int
	kb      config.KeyBindings
}

// NewHelpModal creates a new help modal
func NewHelpModal() HelpModal {
	return HelpModal{
		visible: false,
	}
}

// Open opens the modal
func (m *HelpModal) Open() tea.Cmd {
	m.visible = true
	m.scroll = 0
	m.kb = config.LoadKeyBindings()
	return nil
}

// Close closes the modal
func (m *HelpModal) Close() {
	m.visible = false
}

// IsVisible returns whether the modal is visible
func (m HelpModal) IsVisible() bool {
	return m.visible
}

// SetSize sets the modal dimensions
func (m *HelpModal) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Update handles messages for the modal
func (m HelpModal) Update(msg tea.Msg) (HelpModal, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "?", "q"))):
			m.visible = false
			return m, func() tea.Msg { return HelpModalClosedMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if m.scroll > 0 {
				m.scroll--
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			m.scroll++
		}
	}

	return m, nil
}

// View renders the modal
func (m HelpModal) View(screenWidth, screenHeight int) string {
	if !m.visible {
		return ""
	}

	var content strings.Builder

	// Title
	content.WriteString(ModalTitleStyle.Render("Keyboard Shortcuts"))
	content.WriteString("\n\n")

	keyStyle := HelpKeyStyle
	descStyle := MutedInlineStyle
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117"))

	// Helper to format key names nicely
	formatKey := func(k string) string {
		replacements := map[string]string{
			"up":        "↑",
			"down":      "↓",
			"left":      "←",
			"right":     "→",
			"space":     "space",
			"enter":     "enter",
			"esc":       "esc",
			"tab":       "tab",
			"shift+tab": "shift+tab",
			"ctrl+u":    "ctrl+u",
			"ctrl+d":    "ctrl+d",
			"ctrl+r":    "ctrl+r",
			"ctrl+l":    "ctrl+l",
			"ctrl+c":    "ctrl+c",
			"ctrl+g":    "ctrl+g",
			"{":         "{",
			"}":         "}",
		}
		// Handle comma-separated keys (e.g., "up,k")
		parts := strings.Split(k, ",")
		for i, p := range parts {
			if rep, ok := replacements[p]; ok {
				parts[i] = rep
			}
		}
		return strings.Join(parts, "/")
	}

	// Build sections
	sections := []struct {
		title string
		items []struct{ key, desc string }
	}{
		{
			title: "Navigation",
			items: []struct{ key, desc string }{
				{formatKey(m.kb.Up) + "/" + formatKey(m.kb.Down), "Move up/down"},
				{formatKey(m.kb.Left) + "/" + formatKey(m.kb.Right), "Move left/right (tiled) / scroll (maximized)"},
				{formatKey(m.kb.ScrollUp) + "/" + formatKey(m.kb.ScrollDown), "Scroll viewport up/down"},
				{formatKey(m.kb.Top) + "/" + formatKey(m.kb.Bottom), "Go to top/bottom"},
				{formatKey(m.kb.NextPane) + "/" + formatKey(m.kb.PrevPane), "Next/previous pane"},
				{"1-9", "Jump to pane 1-9"},
			},
		},
		{
			title: "Selection (Discovery Screen)",
			items: []struct{ key, desc string }{
				{formatKey(m.kb.Select), "Toggle container selection"},
				{formatKey(m.kb.SelectAll), "Select all containers"},
				{formatKey(m.kb.ClearAll), "Clear all selections"},
				{formatKey(m.kb.Confirm), "Confirm and start monitoring"},
			},
		},
		{
			title: "Container Actions",
			items: []struct{ key, desc string }{
				{formatKey(m.kb.Restart), "Restart container"},
				{formatKey(m.kb.Kill), "Kill container (force stop)"},
				{formatKey(m.kb.Remove), "Remove container"},
				{formatKey(m.kb.Start), "Start stopped container"},
				{formatKey(m.kb.Stop), "Stop running container"},
				{formatKey(m.kb.Exec), "Open shell in container"},
				{formatKey(m.kb.Inspect), "Inspect container details"},
			},
		},
		{
			title: "Compose Actions",
			items: []struct{ key, desc string }{
				{formatKey(m.kb.ComposeRestart), "Compose down/up"},
				{formatKey(m.kb.ComposeBuild), "Build (no-cache) and start"},
				{formatKey(m.kb.ComposeUp), "Compose up"},
				{formatKey(m.kb.ComposeDown), "Compose down"},
			},
		},
		{
			title: "Log Actions",
			items: []struct{ key, desc string }{
				{formatKey(m.kb.ClearLogs), "Clear logs in focused pane"},
				{formatKey(m.kb.PauseLogs), "Pause/resume log streaming"},
				{"Right-click / ctrl+c", "Copy selected text"},
				{formatKey(m.kb.CopyLogs), "Copy all logs to clipboard"},
				{formatKey(m.kb.WordWrap), "Toggle word wrap"},
				{formatKey(m.kb.Search), "Search/filter logs"},
			},
		},
		{
			title: "General",
			items: []struct{ key, desc string }{
				{formatKey(m.kb.Confirm), "Toggle maximize pane"},
				{formatKey(m.kb.Back), "Un-maximize / go back"},
				{formatKey(m.kb.Config), "Open configuration"},
				{formatKey(m.kb.SavedProjects), "Saved projects"},
				{formatKey(m.kb.Help), "Show this help"},
				{formatKey(m.kb.Refresh), "Refresh container list"},
				{formatKey(m.kb.DebugToggle), "Toggle debug logging"},
				{formatKey(m.kb.Quit), "Quit"},
			},
		},
	}

	// Render sections
	var lines []string
	for _, section := range sections {
		lines = append(lines, sectionStyle.Render("  "+section.title))
		lines = append(lines, "")
		for _, item := range section.items {
			line := "    " + keyStyle.Render(item.key) + " " + descStyle.Render(item.desc)
			lines = append(lines, line)
		}
		lines = append(lines, "")
	}

	// Apply scroll offset
	maxScroll := len(lines) - 15 // Show about 15 lines
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}

	visibleLines := lines
	if m.scroll < len(lines) {
		visibleLines = lines[m.scroll:]
	}
	if len(visibleLines) > 20 {
		visibleLines = visibleLines[:20]
	}

	content.WriteString(strings.Join(visibleLines, "\n"))
	content.WriteString("\n\n")

	// Scroll indicator
	if len(lines) > 20 {
		scrollInfo := MutedInlineStyle.Render("  j/k: scroll  ")
		content.WriteString(scrollInfo)
	}

	// Close hint
	content.WriteString(MutedInlineStyle.Render("esc/?/q: close"))

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
