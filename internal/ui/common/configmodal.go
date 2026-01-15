package common

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"cm/internal/config"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConfigModalClosedMsg is sent when the config modal is closed
type ConfigModalClosedMsg struct {
	ConfigChanged bool
}

// OpenEditorMsg is sent when we need to open the config in an editor
type OpenEditorMsg struct {
	ConfigPath string
}

// OpenSavedProjectsMsg is sent when we need to open the cache management modal
type OpenSavedProjectsMsg struct{}

// ConfigModalItem represents a configurable item
type ConfigModalItem int

const (
	ItemNotificationMode ConfigModalItem = iota
	ItemToastDuration
	ItemToastPosition
	ItemEditKeyBindings
	ItemResetKeyBindings
	ItemResetAll
	ItemSave
	ItemCancel
)

// ConfigModal represents the configuration modal
type ConfigModal struct {
	visible      bool
	width        int
	height       int
	selectedItem ConfigModalItem
	cfg          *config.Config
	originalCfg  config.Config // To detect changes

	// Current values being edited
	notifyMode       config.NotificationMode
	toastDuration    int
	toastPosition    config.ToastPosition
	keyBindings      config.KeyBindings
	keyBindingsReset bool // Track if key bindings were reset this session
}

// NewConfigModal creates a new config modal
func NewConfigModal() ConfigModal {
	return ConfigModal{
		visible:      false,
		selectedItem: ItemNotificationMode,
	}
}

// Open opens the modal and loads current config
func (m *ConfigModal) Open() tea.Cmd {
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}

	m.cfg = cfg
	m.originalCfg = *cfg
	settings := cfg.GetNotificationSettings()
	m.notifyMode = settings.Mode
	m.toastDuration = settings.GetToastDuration()
	m.toastPosition = settings.GetToastPosition()
	m.keyBindings = config.LoadKeyBindings()
	m.visible = true
	m.selectedItem = ItemNotificationMode
	m.keyBindingsReset = false

	return nil
}

// Close closes the modal
func (m *ConfigModal) Close() {
	m.visible = false
}

// IsVisible returns whether the modal is visible
func (m ConfigModal) IsVisible() bool {
	return m.visible
}

// SetSize sets the modal dimensions
func (m *ConfigModal) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Update handles messages for the modal
func (m ConfigModal) Update(msg tea.Msg) (ConfigModal, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.visible = false
			return m, func() tea.Msg { return ConfigModalClosedMsg{ConfigChanged: false} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if m.selectedItem > ItemNotificationMode {
				m.selectedItem--
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if m.selectedItem < ItemCancel {
				m.selectedItem++
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			m.handleLeft()

		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			m.handleRight()

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter", " "))):
			return m.handleSelect()
		}
	}

	return m, nil
}

func (m *ConfigModal) handleLeft() {
	switch m.selectedItem {
	case ItemNotificationMode:
		m.notifyMode = m.prevNotifyMode()
	case ItemToastDuration:
		if m.toastDuration > 1 {
			m.toastDuration--
		}
	case ItemToastPosition:
		m.toastPosition = m.prevToastPosition()
	}
}

func (m *ConfigModal) handleRight() {
	switch m.selectedItem {
	case ItemNotificationMode:
		m.notifyMode = m.nextNotifyMode()
	case ItemToastDuration:
		if m.toastDuration < 10 {
			m.toastDuration++
		}
	case ItemToastPosition:
		m.toastPosition = m.nextToastPosition()
	}
}

func (m *ConfigModal) handleSelect() (ConfigModal, tea.Cmd) {
	switch m.selectedItem {
	case ItemNotificationMode:
		m.notifyMode = m.nextNotifyMode()

	case ItemToastDuration:
		// Cycle 1-10 on enter
		m.toastDuration++
		if m.toastDuration > 10 {
			m.toastDuration = 1
		}

	case ItemToastPosition:
		m.toastPosition = m.nextToastPosition()

	case ItemEditKeyBindings:
		// Open keybindings file in editor
		kbPath := config.GetKeybindingsPath()
		m.visible = false
		return *m, openEditor(kbPath)

	case ItemResetKeyBindings:
		m.keyBindings = config.DefaultKeyBindings()
		m.keyBindingsReset = true

	case ItemResetAll:
		m.notifyMode = config.NotifyTerminal
		m.toastDuration = 3
		m.toastPosition = config.ToastBottomRight
		m.keyBindings = config.DefaultKeyBindings()
		m.keyBindingsReset = true

	case ItemSave:
		m.cfg.Notifications = &config.NotificationSettings{
			Mode:          m.notifyMode,
			ToastDuration: m.toastDuration,
			ToastPosition: m.toastPosition,
		}
		// Save config
		if err := m.cfg.Save(); err != nil {
			return *m, nil
		}
		// Save keybindings if reset
		if m.keyBindingsReset {
			_ = config.SaveKeyBindings(m.keyBindings)
		}
		m.visible = false
		return *m, func() tea.Msg { return ConfigModalClosedMsg{ConfigChanged: true} }

	case ItemCancel:
		m.visible = false
		return *m, func() tea.Msg { return ConfigModalClosedMsg{ConfigChanged: false} }
	}

	return *m, nil
}

// openEditor opens the config file in the default editor
func openEditor(configPath string) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		// Try common editors
		for _, e := range []string{"nano", "vim", "vi"} {
			if _, err := exec.LookPath(e); err == nil {
				editor = e
				break
			}
		}
	}
	if editor == "" {
		editor = "vi" // Fallback
	}

	// Split editor command into parts (handles "cursor --wait" etc.)
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		parts = []string{"vi"}
	}

	// Build args: editor flags + config path
	args := append(parts[1:], configPath)
	c := exec.Command(parts[0], args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return ConfigModalClosedMsg{ConfigChanged: err == nil}
	})
}

func (m ConfigModal) nextNotifyMode() config.NotificationMode {
	modes := []config.NotificationMode{config.NotifyTerminal, config.NotifyOS, config.NotifyNone}
	for i, mode := range modes {
		if mode == m.notifyMode {
			return modes[(i+1)%len(modes)]
		}
	}
	return config.NotifyTerminal
}

func (m ConfigModal) prevNotifyMode() config.NotificationMode {
	modes := []config.NotificationMode{config.NotifyTerminal, config.NotifyOS, config.NotifyNone}
	for i, mode := range modes {
		if mode == m.notifyMode {
			prev := i - 1
			if prev < 0 {
				prev = len(modes) - 1
			}
			return modes[prev]
		}
	}
	return config.NotifyTerminal
}

func (m ConfigModal) nextToastPosition() config.ToastPosition {
	positions := []config.ToastPosition{
		config.ToastBottomRight,
		config.ToastBottomLeft,
		config.ToastTopRight,
		config.ToastTopLeft,
	}
	for i, pos := range positions {
		if pos == m.toastPosition {
			return positions[(i+1)%len(positions)]
		}
	}
	return config.ToastBottomRight
}

func (m ConfigModal) prevToastPosition() config.ToastPosition {
	positions := []config.ToastPosition{
		config.ToastBottomRight,
		config.ToastBottomLeft,
		config.ToastTopRight,
		config.ToastTopLeft,
	}
	for i, pos := range positions {
		if pos == m.toastPosition {
			prev := i - 1
			if prev < 0 {
				prev = len(positions) - 1
			}
			return positions[prev]
		}
	}
	return config.ToastBottomRight
}

func (m ConfigModal) notifyModeDisplay() string {
	switch m.notifyMode {
	case config.NotifyTerminal:
		return "Terminal"
	case config.NotifyOS:
		return "OS Native"
	case config.NotifyNone:
		return "Disabled"
	default:
		return string(m.notifyMode)
	}
}

func (m ConfigModal) toastPositionDisplay() string {
	switch m.toastPosition {
	case config.ToastTopLeft:
		return "Top Left"
	case config.ToastTopRight:
		return "Top Right"
	case config.ToastBottomLeft:
		return "Bottom Left"
	case config.ToastBottomRight:
		return "Bottom Right"
	default:
		return "Bottom Right"
	}
}

// View renders the modal
func (m ConfigModal) View(screenWidth, screenHeight int) string {
	if !m.visible {
		return ""
	}

	var content strings.Builder

	// Title
	content.WriteString(ModalTitleStyle.Render("Configuration"))
	content.WriteString("\n\n")

	// Notification Mode
	m.renderSelectItem(&content, ItemNotificationMode, "Notifications", m.notifyModeDisplay())

	// Toast Duration
	durationValue := fmt.Sprintf("< %ds >", m.toastDuration)
	m.renderSelectItemRaw(&content, ItemToastDuration, "Toast Duration", durationValue, fmt.Sprintf("%ds", m.toastDuration))

	// Toast Position
	m.renderSelectItem(&content, ItemToastPosition, "Toast Position", m.toastPositionDisplay())

	// Key Bindings section
	content.WriteString(MutedInlineStyle.Render("  ─── Key Bindings ───────────"))
	content.WriteString("\n\n")

	// Get current key bindings
	kb := m.keyBindings

	// Display key bindings in a compact format
	keyStyle := HelpKeyStyle
	descStyle := MutedInlineStyle

	// Navigation row
	content.WriteString(descStyle.Render("  Navigation: "))
	content.WriteString(keyStyle.Render(kb.Up) + descStyle.Render("/") + keyStyle.Render(kb.Down) + descStyle.Render(":move "))
	content.WriteString(keyStyle.Render(kb.ScrollUp) + descStyle.Render("/") + keyStyle.Render(kb.ScrollDown) + descStyle.Render(":scroll "))
	content.WriteString(keyStyle.Render(kb.Top) + descStyle.Render("/") + keyStyle.Render(kb.Bottom) + descStyle.Render(":top/btm"))
	content.WriteString("\n")

	// Selection row
	content.WriteString(descStyle.Render("  Selection:  "))
	content.WriteString(keyStyle.Render(kb.Select) + descStyle.Render(":sel "))
	content.WriteString(keyStyle.Render(kb.SelectAll) + descStyle.Render(":all "))
	content.WriteString(keyStyle.Render(kb.ClearAll) + descStyle.Render(":clr "))
	content.WriteString(keyStyle.Render(kb.Confirm) + descStyle.Render(":confirm "))
	content.WriteString(keyStyle.Render(kb.Back) + descStyle.Render(":back"))
	content.WriteString("\n")

	// Actions row
	content.WriteString(descStyle.Render("  Actions:    "))
	content.WriteString(keyStyle.Render(kb.Start) + descStyle.Render(":start "))
	content.WriteString(keyStyle.Render(kb.Stop) + descStyle.Render(":stop "))
	content.WriteString(keyStyle.Render(kb.Restart) + descStyle.Render(":restart "))
	content.WriteString(keyStyle.Render(kb.ComposeBuild) + descStyle.Render(":build"))
	content.WriteString("\n")

	// Compose row
	content.WriteString(descStyle.Render("  Compose:    "))
	content.WriteString(keyStyle.Render(kb.ComposeRestart) + descStyle.Render(":down/up "))
	content.WriteString(keyStyle.Render(kb.Refresh) + descStyle.Render(":refresh "))
	content.WriteString(keyStyle.Render(kb.Quit) + descStyle.Render(":quit"))
	content.WriteString("\n\n")

	// Edit Key Bindings
	editKeyLabel := "[Edit Key Bindings]"
	if m.selectedItem == ItemEditKeyBindings {
		content.WriteString(ModalSelectedStyle.Render("  " + editKeyLabel))
	} else {
		content.WriteString(MutedInlineStyle.Render("  " + editKeyLabel))
	}
	content.WriteString("\n")

	// Reset Key Bindings
	resetKeyLabel := "[Reset Key Bindings]"
	if m.keyBindingsReset {
		resetKeyLabel = "[Reset Key Bindings] ✓"
	}
	if m.selectedItem == ItemResetKeyBindings {
		content.WriteString(ModalSelectedStyle.Render("  " + resetKeyLabel))
	} else {
		content.WriteString(MutedInlineStyle.Render("  " + resetKeyLabel))
	}
	content.WriteString("\n\n")

	// Separator
	content.WriteString(MutedInlineStyle.Render("  ─────────────────────────────"))
	content.WriteString("\n\n")

	// Reset All
	if m.selectedItem == ItemResetAll {
		content.WriteString(ModalSelectedStyle.Render("  [Reset All to Defaults]"))
	} else {
		content.WriteString(MutedInlineStyle.Render("  [Reset All to Defaults]"))
	}
	content.WriteString("\n\n")

	// Buttons row
	saveBtn := "  Save  "
	cancelBtn := "  Cancel  "

	if m.selectedItem == ItemSave {
		content.WriteString(ModalButtonActiveStyle.Render(saveBtn))
		content.WriteString(ModalButtonStyle.Render(cancelBtn))
	} else if m.selectedItem == ItemCancel {
		content.WriteString(ModalButtonStyle.Render(saveBtn))
		content.WriteString(ModalButtonActiveStyle.Render(cancelBtn))
	} else {
		content.WriteString(ModalButtonStyle.Render(saveBtn))
		content.WriteString(ModalButtonStyle.Render(cancelBtn))
	}
	content.WriteString("\n\n")

	// Help
	content.WriteString(MutedInlineStyle.Render("  j/k: navigate  h/l: change  enter: select  esc: close"))

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

func (m ConfigModal) renderSelectItem(b *strings.Builder, item ConfigModalItem, label, value string) {
	m.renderSelectItemRaw(b, item, label, fmt.Sprintf("< %s >", value), value)
}

func (m ConfigModal) renderSelectItemRaw(b *strings.Builder, item ConfigModalItem, label, selectedValue, normalValue string) {
	if m.selectedItem == item {
		b.WriteString(ModalSelectedStyle.Render(fmt.Sprintf("  %-16s %s", label, selectedValue)))
	} else {
		b.WriteString(fmt.Sprintf("  %s  %s",
			ModalLabelStyle.Render(fmt.Sprintf("%-14s", label)),
			ModalValueStyle.Render(normalValue)))
	}
	b.WriteString("\n\n")
}
