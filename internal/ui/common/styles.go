package common

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	primaryColor   = lipgloss.Color("39")  // Blue
	secondaryColor = lipgloss.Color("170") // Purple
	successColor   = lipgloss.Color("#00ff66") // Green (truecolor for terminal consistency)
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

	// Modal styles
	ModalOverlayStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("0"))

	ModalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 2)

	ModalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			MarginBottom(1)

	ModalLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Width(20)

	ModalValueStyle = lipgloss.NewStyle().
			Foreground(successColor)

	ModalSelectedStyle = lipgloss.NewStyle().
				Foreground(primaryColor).
				Bold(true)

	ModalButtonStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Background(lipgloss.Color("240")).
				Padding(0, 2).
				MarginRight(1)

	ModalButtonActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("235")).
				Background(primaryColor).
				Bold(true).
				Padding(0, 2).
				MarginRight(1)

	ModalDangerButtonStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Background(errorColor).
				Padding(0, 2).
				MarginRight(1)

	// Tab bar styles for maximized container view
	TabBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	TabActiveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("235")).
			Background(primaryColor).
			Bold(true).
			Padding(0, 2)

	TabInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Background(lipgloss.Color("238")).
				Padding(0, 2)

	TabSeparatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))

	// Stats tab specific styles
	StatsLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Bold(true)

	StatsValueStyle = lipgloss.NewStyle().
			Foreground(successColor)

	// Env/Config tab styles
	EnvKeyStyle = lipgloss.NewStyle().
			Foreground(primaryColor)

	EnvValueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	EnvRedactedStyle = lipgloss.NewStyle().
				Foreground(mutedColor).
				Italic(true)

	// Top tab styles
	TopHeaderStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	TopRowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
)
