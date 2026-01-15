package common

import (
	"strings"

	"cm/internal/config"

	"github.com/charmbracelet/bubbles/key"
)

// KeyMap defines the key bindings for the application
type KeyMap struct {
	Up             key.Binding
	Down           key.Binding
	Left           key.Binding
	Right          key.Binding
	ScrollUp       key.Binding
	ScrollDown     key.Binding
	Select         key.Binding
	Confirm        key.Binding
	Back           key.Binding
	Quit           key.Binding
	Help           key.Binding
	Refresh        key.Binding
	NextPane       key.Binding
	PrevPane       key.Binding
	Start          key.Binding
	Restart        key.Binding
	ComposeRestart key.Binding
	ComposeBuild   key.Binding
}

// parseKeys splits a comma-separated key string into a slice
func parseKeys(keys string) []string {
	parts := strings.Split(keys, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "space" {
			p = " "
		}
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// DefaultKeyMap returns the default key bindings, loaded from config if available
func DefaultKeyMap() KeyMap {
	cfg, _ := config.Load()
	var bindings config.KeyBindings
	if cfg != nil {
		bindings = cfg.GetKeyBindings()
	} else {
		bindings = config.DefaultKeyBindings()
	}

	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Up)...),
			key.WithHelp("↑", "pane up"),
		),
		Down: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Down)...),
			key.WithHelp("↓", "pane down"),
		),
		Left: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Left)...),
			key.WithHelp("←", "pane left"),
		),
		Right: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Right)...),
			key.WithHelp("→", "pane right"),
		),
		ScrollUp: key.NewBinding(
			key.WithKeys(parseKeys(bindings.ScrollUp)...),
			key.WithHelp("k", "scroll up"),
		),
		ScrollDown: key.NewBinding(
			key.WithKeys(parseKeys(bindings.ScrollDown)...),
			key.WithHelp("j", "scroll down"),
		),
		Select: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Select)...),
			key.WithHelp("space/x", "toggle select"),
		),
		Confirm: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Confirm)...),
			key.WithHelp("enter", "confirm"),
		),
		Back: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Back)...),
			key.WithHelp("esc", "back"),
		),
		Quit: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Quit)...),
			key.WithHelp("q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Help)...),
			key.WithHelp("?", "help"),
		),
		Refresh: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Refresh)...),
			key.WithHelp("r", "refresh"),
		),
		NextPane: key.NewBinding(
			key.WithKeys(parseKeys(bindings.NextPane)...),
			key.WithHelp("tab", "next pane"),
		),
		PrevPane: key.NewBinding(
			key.WithKeys(parseKeys(bindings.PrevPane)...),
			key.WithHelp("shift+tab", "prev pane"),
		),
		Start: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Start)...),
			key.WithHelp("s", "start"),
		),
		Restart: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Restart)...),
			key.WithHelp("r", "restart"),
		),
		ComposeRestart: key.NewBinding(
			key.WithKeys(parseKeys(bindings.ComposeRestart)...),
			key.WithHelp("R", "compose down/up"),
		),
		ComposeBuild: key.NewBinding(
			key.WithKeys(parseKeys(bindings.ComposeBuild)...),
			key.WithHelp("B", "build & up"),
		),
	}
}
