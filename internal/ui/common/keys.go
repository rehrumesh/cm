package common

import (
	"strings"

	"cm/internal/config"

	"github.com/charmbracelet/bubbles/key"
)

// KeyMap defines the key bindings for the application
type KeyMap struct {
	// Navigation
	Up         key.Binding
	Down       key.Binding
	Left       key.Binding
	Right      key.Binding
	ScrollUp   key.Binding
	ScrollDown key.Binding
	Top        key.Binding
	Bottom     key.Binding
	NextPane   key.Binding
	PrevPane   key.Binding

	// Selection
	Select    key.Binding
	SelectAll key.Binding
	ClearAll  key.Binding
	Confirm   key.Binding
	Back      key.Binding

	// Container actions
	Start   key.Binding
	Stop    key.Binding
	Restart key.Binding
	Kill    key.Binding
	Remove  key.Binding
	Exec    key.Binding
	Inspect key.Binding

	// Compose actions
	ComposeUp      key.Binding
	ComposeDown    key.Binding
	ComposeRestart key.Binding
	ComposeBuild   key.Binding

	// General
	Refresh       key.Binding
	Search        key.Binding
	Help          key.Binding
	Config        key.Binding
	SavedProjects key.Binding
	Quit          key.Binding
	CopyLogs      key.Binding
	WordWrap      key.Binding
	DebugToggle   key.Binding
	ClearLogs     key.Binding
	PauseLogs     key.Binding

	// Pane shortcuts
	Pane1 key.Binding
	Pane2 key.Binding
	Pane3 key.Binding
	Pane4 key.Binding
	Pane5 key.Binding
	Pane6 key.Binding
	Pane7 key.Binding
	Pane8 key.Binding
	Pane9 key.Binding

	// Pane resize
	ResizeLeft  key.Binding
	ResizeRight key.Binding
	ResizeUp    key.Binding
	ResizeDown  key.Binding
	ResizeReset key.Binding
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

// DefaultKeyMap returns the default key bindings, loaded from keybindings file
func DefaultKeyMap() KeyMap {
	bindings := config.LoadKeyBindings()

	return KeyMap{
		// Navigation
		Up: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Up)...),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Down)...),
			key.WithHelp("↓/j", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Left)...),
			key.WithHelp("←", "left"),
		),
		Right: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Right)...),
			key.WithHelp("→", "right"),
		),
		ScrollUp: key.NewBinding(
			key.WithKeys(parseKeys(bindings.ScrollUp)...),
			key.WithHelp("ctrl+u", "scroll up"),
		),
		ScrollDown: key.NewBinding(
			key.WithKeys(parseKeys(bindings.ScrollDown)...),
			key.WithHelp("ctrl+d", "scroll down"),
		),
		Top: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Top)...),
			key.WithHelp("g", "top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Bottom)...),
			key.WithHelp("G", "bottom"),
		),
		NextPane: key.NewBinding(
			key.WithKeys(parseKeys(bindings.NextPane)...),
			key.WithHelp("}", "next pane"),
		),
		PrevPane: key.NewBinding(
			key.WithKeys(parseKeys(bindings.PrevPane)...),
			key.WithHelp("{", "prev pane"),
		),

		// Selection
		Select: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Select)...),
			key.WithHelp("space", "select"),
		),
		SelectAll: key.NewBinding(
			key.WithKeys(parseKeys(bindings.SelectAll)...),
			key.WithHelp("a", "select all"),
		),
		ClearAll: key.NewBinding(
			key.WithKeys(parseKeys(bindings.ClearAll)...),
			key.WithHelp("A", "clear all"),
		),
		Confirm: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Confirm)...),
			key.WithHelp("enter", "confirm"),
		),
		Back: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Back)...),
			key.WithHelp("esc", "back"),
		),

		// Container actions
		Start: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Start)...),
			key.WithHelp("u", "start"),
		),
		Stop: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Stop)...),
			key.WithHelp("s", "stop"),
		),
		Restart: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Restart)...),
			key.WithHelp("r", "restart"),
		),
		Kill: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Kill)...),
			key.WithHelp("K", "kill"),
		),
		Remove: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Remove)...),
			key.WithHelp("D", "remove"),
		),
		Exec: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Exec)...),
			key.WithHelp("e", "exec"),
		),
		Inspect: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Inspect)...),
			key.WithHelp("i", "inspect"),
		),

		// Compose actions
		ComposeUp: key.NewBinding(
			key.WithKeys(parseKeys(bindings.ComposeUp)...),
			key.WithHelp("U", "compose up"),
		),
		ComposeDown: key.NewBinding(
			key.WithKeys(parseKeys(bindings.ComposeDown)...),
			key.WithHelp("S", "compose down"),
		),
		ComposeRestart: key.NewBinding(
			key.WithKeys(parseKeys(bindings.ComposeRestart)...),
			key.WithHelp("R", "compose restart"),
		),
		ComposeBuild: key.NewBinding(
			key.WithKeys(parseKeys(bindings.ComposeBuild)...),
			key.WithHelp("b", "build"),
		),

		// General
		Refresh: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Refresh)...),
			key.WithHelp("ctrl+r", "refresh"),
		),
		Search: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Search)...),
			key.WithHelp("/", "search"),
		),
		Help: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Help)...),
			key.WithHelp("?", "help"),
		),
		Config: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Config)...),
			key.WithHelp("c", "config"),
		),
		SavedProjects: key.NewBinding(
			key.WithKeys(parseKeys(bindings.SavedProjects)...),
			key.WithHelp("p", "projects"),
		),
		Quit: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Quit)...),
			key.WithHelp("q", "quit"),
		),
		CopyLogs: key.NewBinding(
			key.WithKeys(parseKeys(bindings.CopyLogs)...),
			key.WithHelp("y", "copy logs"),
		),
		WordWrap: key.NewBinding(
			key.WithKeys(parseKeys(bindings.WordWrap)...),
			key.WithHelp("w", "word wrap"),
		),
		DebugToggle: key.NewBinding(
			key.WithKeys(parseKeys(bindings.DebugToggle)...),
			key.WithHelp("ctrl+g", "debug logs"),
		),
		ClearLogs: key.NewBinding(
			key.WithKeys(parseKeys(bindings.ClearLogs)...),
			key.WithHelp("ctrl+l", "clear logs"),
		),
		PauseLogs: key.NewBinding(
			key.WithKeys(parseKeys(bindings.PauseLogs)...),
			key.WithHelp("P", "pause logs"),
		),

		// Pane shortcuts
		Pane1: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Pane1)...),
			key.WithHelp("1", "pane 1"),
		),
		Pane2: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Pane2)...),
			key.WithHelp("2", "pane 2"),
		),
		Pane3: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Pane3)...),
			key.WithHelp("3", "pane 3"),
		),
		Pane4: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Pane4)...),
			key.WithHelp("4", "pane 4"),
		),
		Pane5: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Pane5)...),
			key.WithHelp("5", "pane 5"),
		),
		Pane6: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Pane6)...),
			key.WithHelp("6", "pane 6"),
		),
		Pane7: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Pane7)...),
			key.WithHelp("7", "pane 7"),
		),
		Pane8: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Pane8)...),
			key.WithHelp("8", "pane 8"),
		),
		Pane9: key.NewBinding(
			key.WithKeys(parseKeys(bindings.Pane9)...),
			key.WithHelp("9", "pane 9"),
		),

		// Pane resize
		ResizeLeft: key.NewBinding(
			key.WithKeys(parseKeys(bindings.ResizeLeft)...),
			key.WithHelp("<", "shrink width"),
		),
		ResizeRight: key.NewBinding(
			key.WithKeys(parseKeys(bindings.ResizeRight)...),
			key.WithHelp(">", "grow width"),
		),
		ResizeUp: key.NewBinding(
			key.WithKeys(parseKeys(bindings.ResizeUp)...),
			key.WithHelp("-", "shrink height"),
		),
		ResizeDown: key.NewBinding(
			key.WithKeys(parseKeys(bindings.ResizeDown)...),
			key.WithHelp("+", "grow height"),
		),
		ResizeReset: key.NewBinding(
			key.WithKeys(parseKeys(bindings.ResizeReset)...),
			key.WithHelp("=", "reset size"),
		),
	}
}
