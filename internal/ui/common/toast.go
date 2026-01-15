package common

import (
	"time"

	"cm/internal/config"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ToastType represents the type of toast notification
type ToastType int

const (
	ToastSuccess ToastType = iota
	ToastError
	ToastInfo
)

// ShowToastMsg is sent to display a toast
type ShowToastMsg struct {
	Title   string
	Message string
	Type    ToastType
}

// ToastExpiredMsg is sent when a toast should be hidden
type ToastExpiredMsg struct {
	ID int
}

// Toast represents a toast notification
type Toast struct {
	visible  bool
	title    string
	message  string
	typ      ToastType
	id       int
	duration time.Duration
	position config.ToastPosition
}

// NewToast creates a new toast manager
func NewToast() Toast {
	// Load settings from config
	cfg, _ := config.Load()
	duration := 3 * time.Second
	position := config.ToastBottomRight

	if cfg != nil {
		settings := cfg.GetNotificationSettings()
		duration = time.Duration(settings.GetToastDuration()) * time.Second
		position = settings.GetToastPosition()
	}

	return Toast{
		visible:  false,
		id:       0,
		duration: duration,
		position: position,
	}
}

// SetDuration updates the toast duration
func (t *Toast) SetDuration(seconds int) {
	if seconds < 1 {
		seconds = 1
	}
	if seconds > 10 {
		seconds = 10
	}
	t.duration = time.Duration(seconds) * time.Second
}

// SetPosition updates the toast position
func (t *Toast) SetPosition(pos config.ToastPosition) {
	t.position = pos
}

// ReloadConfig reloads toast settings from config
func (t *Toast) ReloadConfig() {
	cfg, _ := config.Load()
	if cfg != nil {
		settings := cfg.GetNotificationSettings()
		t.duration = time.Duration(settings.GetToastDuration()) * time.Second
		t.position = settings.GetToastPosition()
	}
}

// Show displays a toast and returns a command to hide it after duration
func (t *Toast) Show(title, message string, typ ToastType) tea.Cmd {
	t.visible = true
	t.title = title
	t.message = message
	t.typ = typ
	t.id++

	currentID := t.id
	return tea.Tick(t.duration, func(time.Time) tea.Msg {
		return ToastExpiredMsg{ID: currentID}
	})
}

// Hide hides the toast if the ID matches (prevents hiding newer toasts)
func (t *Toast) Hide(id int) {
	if t.id == id {
		t.visible = false
	}
}

// IsVisible returns whether the toast is visible
func (t Toast) IsVisible() bool {
	return t.visible
}

// Update handles toast messages
func (t Toast) Update(msg tea.Msg) (Toast, tea.Cmd) {
	switch msg := msg.(type) {
	case ShowToastMsg:
		return t, t.Show(msg.Title, msg.Message, msg.Type)

	case ToastExpiredMsg:
		t.Hide(msg.ID)
	}

	return t, nil
}

// RenderInline renders just the toast box without positioning (for inline display)
func (t Toast) RenderInline() string {
	if !t.visible {
		return ""
	}

	// Choose style based on type
	var borderColor, iconColor lipgloss.Color
	var icon string

	switch t.typ {
	case ToastSuccess:
		borderColor = lipgloss.Color("42") // Green
		iconColor = lipgloss.Color("42")
		icon = "✓"
	case ToastError:
		borderColor = lipgloss.Color("196") // Red
		iconColor = lipgloss.Color("196")
		icon = "✗"
	case ToastInfo:
		borderColor = lipgloss.Color("39") // Blue
		iconColor = lipgloss.Color("39")
		icon = "●"
	}

	// Build toast content
	iconStyle := lipgloss.NewStyle().
		Foreground(iconColor).
		Bold(true).
		MarginRight(1)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("252"))

	messageStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	content := lipgloss.JoinHorizontal(lipgloss.Center,
		iconStyle.Render(icon),
		titleStyle.Render(t.title),
	)

	if t.message != "" {
		content = lipgloss.JoinVertical(lipgloss.Left,
			content,
			messageStyle.Render(t.message),
		)
	}

	// Toast container style
	toastStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Background(lipgloss.Color("236")).
		Padding(0, 2)

	return toastStyle.Render(content)
}

// View renders the toast with positioning (for overlay display)
func (t Toast) View(screenWidth, screenHeight int) string {
	if !t.visible {
		return ""
	}

	toastContent := t.RenderInline()

	// Calculate position based on config
	toastWidth := lipgloss.Width(toastContent)
	toastHeight := lipgloss.Height(toastContent)

	var x, y int
	switch t.position {
	case config.ToastTopLeft:
		x = 2
		y = 1
	case config.ToastTopRight:
		x = screenWidth - toastWidth - 2
		y = 1
	case config.ToastBottomLeft:
		x = 2
		y = screenHeight - toastHeight - 2
	case config.ToastBottomRight:
		x = screenWidth - toastWidth - 2
		y = screenHeight - toastHeight - 2
	default:
		// Default to bottom-right
		x = screenWidth - toastWidth - 2
		y = screenHeight - toastHeight - 2
	}

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	// Create positioned toast
	positioned := lipgloss.NewStyle().
		MarginLeft(x).
		MarginTop(y).
		Render(toastContent)

	return positioned
}
