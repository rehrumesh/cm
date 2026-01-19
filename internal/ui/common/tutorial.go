package common

import (
	"cm/internal/config"

	"github.com/charmbracelet/lipgloss"
)

// TutorialStep represents the current step in the tutorial
type TutorialStep int

const (
	TutorialStepNone TutorialStep = iota
	TutorialStepIntro     // Discovery: intro modal
	TutorialStepNavigate  // Discovery: navigate with j/k
	TutorialStepSelect    // Discovery: select with space
	TutorialStepConfirm   // Discovery: enter to view logs
	TutorialStepPaneNav   // LogView: arrow keys between panes
	TutorialStepMaximize  // LogView: enter to maximize
	TutorialStepShell     // LogView: e for shell (final)
	TutorialStepComplete
)

// Tutorial tracks the interactive tutorial state
type Tutorial struct {
	Active bool
	Step   TutorialStep
}

// NewTutorial creates a new tutorial, checking config to see if it should be active
// The tutorial starts in a pending state (Step=None) until StartIfReady is called
func NewTutorial() Tutorial {
	cfg, err := config.Load()
	if err != nil {
		return Tutorial{Active: false, Step: TutorialStepNone}
	}

	if cfg.ShouldShowTutorial() {
		// Active but pending - will show intro modal after containers load
		return Tutorial{Active: true, Step: TutorialStepNone}
	}
	return Tutorial{Active: false, Step: TutorialStepNone}
}

// StartIfReady activates the tutorial intro if there are containers, otherwise skips
func (t *Tutorial) StartIfReady(hasContainers bool) {
	if !t.Active || t.Step != TutorialStepNone {
		return
	}

	if hasContainers {
		t.Step = TutorialStepIntro
	} else {
		// No containers - can't do tutorial, mark as complete
		t.Skip()
	}
}

// NewTutorialFromState creates a tutorial with existing state (for screen transitions)
func NewTutorialFromState(active bool, step TutorialStep) Tutorial {
	return Tutorial{Active: active, Step: step}
}

// Advance moves to the next tutorial step
func (t *Tutorial) Advance() {
	if !t.Active {
		return
	}
	t.Step++
	if t.Step >= TutorialStepComplete {
		t.Skip()
	}
}

// Skip ends the tutorial and saves completion to config
func (t *Tutorial) Skip() {
	t.Active = false
	t.Step = TutorialStepComplete

	// Save completion to config
	cfg, err := config.Load()
	if err != nil {
		return
	}
	_ = cfg.MarkTutorialCompleted()
}

// HintText returns the hint text for the current step
func (t Tutorial) HintText() string {
	switch t.Step {
	case TutorialStepNavigate:
		return "Tutorial: Press \u2193/j to navigate down"
	case TutorialStepSelect:
		return "Great! Press space to select containers (pick 2+ for split view)"
	case TutorialStepConfirm:
		return "Perfect! Press enter to start monitoring"
	case TutorialStepPaneNav:
		return "Use \u2190/\u2192 arrows to switch between panes"
	case TutorialStepMaximize:
		return "Press enter to maximize this pane"
	case TutorialStepShell:
		return "Tip: Press e to open a shell. Press s to finish tutorial"
	default:
		return ""
	}
}

// Tutorial hint bar styles
var (
	tutorialBarStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("214")). // Orange/amber
				Foreground(lipgloss.Color("0")).   // Black text
				Bold(false)

	tutorialHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")). // Black
				Bold(false)

	tutorialSkipStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("238")). // Dark gray
				Bold(false)
)

// View renders the tutorial hint bar
func (t Tutorial) View(width int) string {
	if !t.Active || t.Step == TutorialStepNone || t.Step >= TutorialStepComplete {
		return ""
	}

	hint := t.HintText()
	skip := "[s] skip"

	// Calculate spacing
	hintWidth := lipgloss.Width(hint)
	skipWidth := lipgloss.Width(skip)
	availableSpace := width - hintWidth - skipWidth - 4 // 4 for padding/margins

	spacing := ""
	if availableSpace > 0 {
		for range availableSpace {
			spacing += " "
		}
	}

	content := " " + tutorialHintStyle.Render(hint) + spacing + tutorialSkipStyle.Render(skip) + " "

	return tutorialBarStyle.Width(width).Render(content)
}

// IsDiscoveryStep returns true if the current step is a discovery screen step
func (t Tutorial) IsDiscoveryStep() bool {
	return t.Step >= TutorialStepNavigate && t.Step <= TutorialStepConfirm
}

// IsIntroStep returns true if we're at the intro modal step
func (t Tutorial) IsIntroStep() bool {
	return t.Active && t.Step == TutorialStepIntro
}

// IsLogViewStep returns true if the current step is a logview screen step
func (t Tutorial) IsLogViewStep() bool {
	return t.Step >= TutorialStepPaneNav && t.Step <= TutorialStepShell
}

// ShouldSkipPaneNav returns true if pane navigation should be skipped (single pane)
func (t *Tutorial) ShouldSkipPaneNav(paneCount int) bool {
	if t.Step == TutorialStepPaneNav && paneCount <= 1 {
		return true
	}
	return false
}

// ShouldAdvanceFromSelect returns true if we should advance past the select step
// Requires 2+ selected unless only 1 container is available
func (t Tutorial) ShouldAdvanceFromSelect(selectedCount, availableCount int) bool {
	if t.Step != TutorialStepSelect {
		return false
	}
	// If only 1 container available, advance after selecting it
	if availableCount <= 1 {
		return selectedCount >= 1
	}
	// Otherwise require 2+ selections
	return selectedCount >= 2
}

// Intro modal styles
var (
	introModalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("214")). // Orange
			Background(lipgloss.Color("235")).
			Padding(1, 3)

	introTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("214")). // Orange
			MarginBottom(1)

	introTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	introKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")) // Blue

	introMutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

// ViewIntroModal renders the tutorial intro modal
func (t Tutorial) ViewIntroModal(width, height int) string {
	if !t.IsIntroStep() {
		return ""
	}

	title := introTitleStyle.Render("Welcome to cm!")

	content := introTextStyle.Render("This quick tutorial will teach you the basics.") + "\n" +
		introTextStyle.Render("Follow the prompts to learn how to:") + "\n\n" +
		introTextStyle.Render("  \u2022 Navigate and select containers") + "\n" +
		introTextStyle.Render("  \u2022 View logs in split panes") + "\n" +
		introTextStyle.Render("  \u2022 Maximize panes and open shells") + "\n\n" +
		introMutedStyle.Render("Tip: For the best experience, have 2+ containers running.") + "\n\n" +
		introKeyStyle.Render("enter") + introMutedStyle.Render(" start tutorial    ") +
		introKeyStyle.Render("s") + introMutedStyle.Render(" skip")

	modal := introModalStyle.Render(title + "\n" + content)

	// Center the modal
	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Center,
		modal,
		lipgloss.WithWhitespaceChars(" "),
	)
}
