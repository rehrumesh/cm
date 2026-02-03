package common

import (
	"fmt"
	"strings"
	"time"

	"cm/internal/docker"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// BuildPanelLogMsg is sent when a new log line is received
type BuildPanelLogMsg struct {
	Log docker.OperationLog
}

// BuildPanelCompleteMsg is sent when the build operation completes
type BuildPanelCompleteMsg struct {
	Success bool
	Error   error
}

// BuildPanelCloseMsg is sent when the build panel should close
type BuildPanelCloseMsg struct{}

// BuildPanel represents a build log panel component
type BuildPanel struct {
	visible     bool
	logs        []docker.OperationLog
	viewport    viewport.Model
	width       int
	height      int
	operation   string // "build", "up", "down"
	serviceName string
	status      string // "running", "success", "error"
	autoClose   bool   // whether to auto-close after completion
	closeTimer  *time.Timer
}

// NewBuildPanel creates a new build panel
func NewBuildPanel() BuildPanel {
	vp := viewport.New(40, 20)
	vp.Style = lipgloss.NewStyle()
	return BuildPanel{
		viewport: vp,
		status:   "idle",
	}
}

// Start starts showing build logs for an operation
func (b *BuildPanel) Start(operation, serviceName string) {
	b.visible = true
	b.logs = nil
	b.operation = operation
	b.serviceName = serviceName
	b.status = "running"
	b.autoClose = true
	b.viewport.SetContent("")
	b.viewport.GotoTop()
}

// AddLog adds a log line to the panel
func (b *BuildPanel) AddLog(log docker.OperationLog) {
	b.logs = append(b.logs, log)
	b.viewport.SetContent(b.renderLogs())
	b.viewport.GotoBottom()
}

// Complete marks the operation as complete
func (b *BuildPanel) Complete(success bool, err error) tea.Cmd {
	if success {
		b.status = "success"
		b.AddLog(docker.OperationLog{
			Timestamp: time.Now(),
			Stream:    "system",
			Content:   "--- Operation completed successfully ---",
		})
	} else {
		b.status = "error"
		errMsg := "unknown error"
		if err != nil {
			errMsg = err.Error()
		}
		b.AddLog(docker.OperationLog{
			Timestamp: time.Now(),
			Stream:    "stderr",
			Content:   fmt.Sprintf("--- Operation failed: %s ---", errMsg),
		})
	}

	// Auto-close after 2 seconds if successful
	if b.autoClose && success {
		return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return BuildPanelCloseMsg{}
		})
	}
	return nil
}

// Close closes the build panel
func (b *BuildPanel) Close() {
	b.visible = false
	b.logs = nil
	b.status = "idle"
}

// IsVisible returns whether the panel is visible
func (b *BuildPanel) IsVisible() bool {
	return b.visible
}

// SetSize sets the panel dimensions
func (b *BuildPanel) SetSize(width, height int) {
	b.width = width
	b.height = height
	// Viewport is slightly smaller to account for border and title
	vpWidth := width - 4
	vpHeight := height - 4
	if vpWidth < 10 {
		vpWidth = 10
	}
	if vpHeight < 5 {
		vpHeight = 5
	}
	b.viewport.Width = vpWidth
	b.viewport.Height = vpHeight
	b.viewport.SetContent(b.renderLogs())
}

// Update handles messages
func (b BuildPanel) Update(msg tea.Msg) (BuildPanel, tea.Cmd) {
	if !b.visible {
		return b, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			b.Close()
			return b, nil
		case "up", "k":
			b.viewport.SetYOffset(b.viewport.YOffset - 1)
		case "down", "j":
			b.viewport.SetYOffset(b.viewport.YOffset + 1)
		case "ctrl+u":
			b.viewport.SetYOffset(b.viewport.YOffset - b.viewport.Height/2)
		case "ctrl+d":
			b.viewport.SetYOffset(b.viewport.YOffset + b.viewport.Height/2)
		case "g":
			b.viewport.GotoTop()
		case "G":
			b.viewport.GotoBottom()
		}
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				b.viewport.SetYOffset(b.viewport.YOffset - 3)
			case tea.MouseButtonWheelDown:
				b.viewport.SetYOffset(b.viewport.YOffset + 3)
			}
		}
	case BuildPanelLogMsg:
		b.AddLog(msg.Log)
	case BuildPanelCompleteMsg:
		return b, b.Complete(msg.Success, msg.Error)
	case BuildPanelCloseMsg:
		b.Close()
	}

	return b, nil
}

// renderLogs renders the log content
func (b *BuildPanel) renderLogs() string {
	if len(b.logs) == 0 {
		return SubtitleStyle.Render("Waiting for output...")
	}

	var sb strings.Builder
	for _, log := range b.logs {
		ts := TimestampStyle.Render(log.Timestamp.Format("15:04:05"))
		var content string
		switch log.Stream {
		case "stderr":
			content = StderrStyle.Render(log.Content)
		case "system":
			content = SubtitleStyle.Render(log.Content)
		default:
			content = log.Content
		}
		sb.WriteString(fmt.Sprintf("%s %s\n", ts, content))
	}
	return sb.String()
}

// View renders the build panel
func (b BuildPanel) View() string {
	if !b.visible {
		return ""
	}

	// Title with operation and service name
	var statusIcon string
	var statusStyle lipgloss.Style
	switch b.status {
	case "running":
		statusIcon = "..."
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")) // Blue
	case "success":
		statusIcon = " OK"
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")) // Green
	case "error":
		statusIcon = " ERR"
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // Red
	}

	title := lipgloss.NewStyle().Bold(true).Render(
		fmt.Sprintf(" %s %s", b.capitalizeOp(), b.serviceName),
	)
	statusText := statusStyle.Render(statusIcon)
	titleLine := title + statusText

	// Help text
	helpText := MutedInlineStyle.Render(" esc: close  ↑↓: scroll")

	// Viewport content
	content := lipgloss.JoinVertical(lipgloss.Left,
		titleLine,
		b.viewport.View(),
		helpText,
	)

	// Apply border only (no background to match terminal default)
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")).
		Width(b.width - 2).
		Height(b.height - 2)

	return borderStyle.Render(content)
}

func (b *BuildPanel) capitalizeOp() string {
	if b.operation == "" {
		return "Build"
	}
	return strings.ToUpper(b.operation[:1]) + b.operation[1:]
}

// GetStatus returns the current status
func (b *BuildPanel) GetStatus() string {
	return b.status
}
