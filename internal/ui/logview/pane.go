package logview

import (
	"fmt"
	"strings"

	"cm/internal/docker"
	"cm/internal/ui/common"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

const maxLogLines = 1000

// Pane represents a single log pane
type Pane struct {
	ID        string
	Container docker.Container
	Viewport  viewport.Model
	LogLines  []docker.LogLine
	Active    bool
	Connected bool
}

// NewPane creates a new log pane for a container
func NewPane(container docker.Container, width, height int) Pane {
	vp := viewport.New(width-2, height-3) // Account for border and title
	vp.Style = lipgloss.NewStyle()

	return Pane{
		ID:        container.ID,
		Container: container,
		Viewport:  vp,
		LogLines:  make([]docker.LogLine, 0, maxLogLines),
		Active:    false,
		Connected: true,
	}
}

// AddLogLine adds a log line to the pane
func (p *Pane) AddLogLine(line docker.LogLine) {
	p.LogLines = append(p.LogLines, line)

	// Trim if too many lines
	if len(p.LogLines) > maxLogLines {
		p.LogLines = p.LogLines[len(p.LogLines)-maxLogLines:]
	}

	// Update viewport content
	p.Viewport.SetContent(p.renderLogs())
	p.Viewport.GotoBottom()
}

// SetSize updates the pane dimensions
func (p *Pane) SetSize(width, height int) {
	vpWidth := width - 2 // Account for border
	if vpWidth < 1 {
		vpWidth = 1
	}
	vpHeight := height - 3 // Account for border and title
	if vpHeight < 1 {
		vpHeight = 1
	}
	p.Viewport.Width = vpWidth
	p.Viewport.Height = vpHeight
	p.Viewport.SetContent(p.renderLogs())
}

// renderLogs renders all log lines as a string
func (p *Pane) renderLogs() string {
	if len(p.LogLines) == 0 {
		return common.SubtitleStyle.Render("Waiting for logs...")
	}

	var b strings.Builder
	for _, line := range p.LogLines {
		// Format timestamp
		ts := common.TimestampStyle.Render(line.Timestamp.Format("15:04:05"))

		// Format content based on stream type
		var content string
		switch line.Stream {
		case "stderr":
			content = common.StderrStyle.Render(line.Content)
		case "system":
			content = common.SubtitleStyle.Render(line.Content)
		default:
			content = common.StdoutStyle.Render(line.Content)
		}

		b.WriteString(fmt.Sprintf("%s %s\n", ts, content))
	}

	return b.String()
}

// View renders the pane
func (p *Pane) View(width, height int, focused bool) string {
	// Update size if needed
	if p.Viewport.Width != width-2 || p.Viewport.Height != height-3 {
		p.SetSize(width, height)
	}

	// Title bar
	title := p.Container.DisplayName()
	if !p.Connected {
		title += " (disconnected)"
	}

	// Status indicator based on container state
	var status string
	if p.Container.State == "running" {
		status = common.RunningStyle.Render("●")
	} else {
		status = common.StoppedStyle.Render("○")
	}

	// Inner content width (excluding borders)
	innerWidth := width - 2
	if innerWidth < 0 {
		innerWidth = 0
	}

	// Build title with status
	fullTitle := fmt.Sprintf(" %s %s", status, title)
	fullTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("252")).
		Width(innerWidth).
		MaxWidth(innerWidth).
		Render(fullTitle)

	// Choose border style
	borderStyle := common.PaneBorderStyle
	if focused {
		borderStyle = common.PaneActiveBorderStyle
	}

	// Calculate inner height (excluding borders)
	innerHeight := height - 2
	if innerHeight < 1 {
		innerHeight = 1
	}

	// Viewport height (excluding title line)
	vpHeight := innerHeight - 1
	if vpHeight < 1 {
		vpHeight = 1
	}

	// Get viewport content and ensure it fits
	viewportContent := lipgloss.NewStyle().
		Width(innerWidth).
		MaxWidth(innerWidth).
		Height(vpHeight).
		MaxHeight(vpHeight).
		Render(p.Viewport.View())

	// Combine title and viewport
	content := lipgloss.JoinVertical(lipgloss.Left,
		fullTitle,
		viewportContent,
	)

	// Render with exact dimensions
	return borderStyle.
		Width(innerWidth).
		Height(innerHeight).
		MaxWidth(width).
		MaxHeight(height).
		Render(content)
}
