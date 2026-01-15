package common

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	primaryColor   = lipgloss.Color("39")  // Blue
	secondaryColor = lipgloss.Color("170") // Purple
	successColor   = lipgloss.Color("42")  // Green
	errorColor     = lipgloss.Color("196") // Red
	mutedColor     = lipgloss.Color("241") // Gray
	borderColor    = lipgloss.Color("240") // Light gray

	// Title styles
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			MarginBottom(1)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginBottom(1)

	// List styles
	SelectedItemStyle = lipgloss.NewStyle().
				Foreground(primaryColor).
				Bold(true)

	NormalItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	CheckedStyle = lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true)

	GroupHeaderStyle = lipgloss.NewStyle().
				Foreground(secondaryColor).
				Bold(true).
				MarginTop(1)

	// Status styles
	RunningStyle = lipgloss.NewStyle().
			Foreground(successColor)

	StoppedStyle = lipgloss.NewStyle().
			Foreground(errorColor)

	// Pane styles
	PaneBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor)

	PaneActiveBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primaryColor)

	PaneTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	// Log styles
	StdoutStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	StderrStyle = lipgloss.NewStyle().
			Foreground(errorColor)

	TimestampStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	// Help styles
	HelpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginTop(1)

	// Help bar styles for log view
	HelpBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252"))

	HelpKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")) // Blue/cyan

	HelpDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	// Empty state
	EmptyStateStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Align(lipgloss.Center)

	// Inline muted text (no margin)
	MutedInlineStyle = lipgloss.NewStyle().
				Foreground(mutedColor)
)
