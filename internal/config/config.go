package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	configDir       = ".cm"
	configFile      = "config.json"
	keybindingsFile = "keybindings.json"
	projectsFile    = "projects.json"
)

// SavedProject stores compose file info for a project
type SavedProject struct {
	ConfigFile string `json:"config_file"`
	WorkingDir string `json:"working_dir"`
}

// KeyBindings stores all configurable key bindings
type KeyBindings struct {
	// Navigation
	Up         string `json:"up"`
	Down       string `json:"down"`
	Left       string `json:"left"`
	Right      string `json:"right"`
	ScrollUp   string `json:"scroll_up"`
	ScrollDown string `json:"scroll_down"`
	Top        string `json:"top"`
	Bottom     string `json:"bottom"`
	NextPane   string `json:"next_pane"`
	PrevPane   string `json:"prev_pane"`

	// Selection
	Select    string `json:"select"`
	SelectAll string `json:"select_all"`
	ClearAll  string `json:"clear_all"`
	Confirm   string `json:"confirm"`
	Back      string `json:"back"`

	// Container actions
	Start   string `json:"start"`
	Stop    string `json:"stop"`
	Restart string `json:"restart"`
	Kill    string `json:"kill"`
	Remove  string `json:"remove"`
	Exec    string `json:"exec"`
	Inspect string `json:"inspect"`

	// Compose actions
	ComposeUp      string `json:"compose_up"`
	ComposeDown    string `json:"compose_down"`
	ComposeRestart string `json:"compose_restart"`
	ComposeBuild   string `json:"compose_build"`

	// General
	Refresh       string `json:"refresh"`
	Search        string `json:"search"`
	Help          string `json:"help"`
	Quit          string `json:"quit"`
	SavedProjects string `json:"saved_projects_key"`
	Config        string `json:"config"`
	CopyLogs      string `json:"copy_logs"`
	CopySelection string `json:"copy_selection"`
	WordWrap      string `json:"word_wrap"`
	DebugToggle   string `json:"debug_toggle"`
	ClearLogs     string `json:"clear_logs"`
	PauseLogs     string `json:"pause_logs"`

	// Pane shortcuts
	Pane1 string `json:"pane_1"`
	Pane2 string `json:"pane_2"`
	Pane3 string `json:"pane_3"`
	Pane4 string `json:"pane_4"`
	Pane5 string `json:"pane_5"`
	Pane6 string `json:"pane_6"`
	Pane7 string `json:"pane_7"`
	Pane8 string `json:"pane_8"`
	Pane9 string `json:"pane_9"`

	// Pane resize
	ResizeLeft  string `json:"resize_left"`
	ResizeRight string `json:"resize_right"`
	ResizeUp    string `json:"resize_up"`
	ResizeDown  string `json:"resize_down"`
	ResizeReset string `json:"resize_reset"`
}

// DefaultKeyBindings returns the default key bindings
func DefaultKeyBindings() KeyBindings {
	return KeyBindings{
		// Navigation
		Up:         "up,k",
		Down:       "down,j",
		Left:       "left",
		Right:      "right",
		ScrollUp:   "ctrl+u",
		ScrollDown: "ctrl+d",
		Top:        "g",
		Bottom:     "G",
		NextPane:   "}",
		PrevPane:   "{",

		// Selection
		Select:    "space",
		SelectAll: "a",
		ClearAll:  "A",
		Confirm:   "enter",
		Back:      "esc",

		// Container actions
		Start:   "u",
		Stop:    "s",
		Restart: "r",
		Kill:    "K",
		Remove:  "D",
		Exec:    "e",
		Inspect: "i",

		// Compose actions
		ComposeUp:      "U",
		ComposeDown:    "S",
		ComposeRestart: "R",
		ComposeBuild:   "b",

		// General
		Refresh:       "ctrl+r",
		Search:        "/",
		Help:          "?",
		Quit:          "q",
		SavedProjects: "p",
		Config:        "c",
		CopyLogs:      "y",
		CopySelection: "ctrl+shift+c",
		WordWrap:      "w",
		DebugToggle:   "ctrl+g",
		ClearLogs:     "ctrl+l",
		PauseLogs:     "P",

		// Pane shortcuts
		Pane1: "1",
		Pane2: "2",
		Pane3: "3",
		Pane4: "4",
		Pane5: "5",
		Pane6: "6",
		Pane7: "7",
		Pane8: "8",
		Pane9: "9",

		// Pane resize
		ResizeLeft:  "<",
		ResizeRight: ">",
		ResizeUp:    "-",
		ResizeDown:  "+",
		ResizeReset: "=",
	}
}

// NotificationMode specifies how notifications are delivered
type NotificationMode string

const (
	NotifyTerminal NotificationMode = "terminal" // Terminal escape sequences (default)
	NotifyOS       NotificationMode = "os"       // OS-native notifications
	NotifyNone     NotificationMode = "none"     // Disabled
)

// ToastPosition specifies where toasts appear
type ToastPosition string

const (
	ToastTopLeft     ToastPosition = "top-left"
	ToastTopRight    ToastPosition = "top-right"
	ToastBottomLeft  ToastPosition = "bottom-left"
	ToastBottomRight ToastPosition = "bottom-right"
)

// NotificationSettings stores notification preferences
type NotificationSettings struct {
	Mode          NotificationMode `json:"mode"`           // "terminal", "os", or "none"
	ToastDuration int              `json:"toast_duration"` // Toast duration in seconds (1-10)
	ToastPosition ToastPosition    `json:"toast_position"` // Toast position on screen
}

// DefaultNotificationSettings returns default notification settings
func DefaultNotificationSettings() NotificationSettings {
	return NotificationSettings{
		Mode:          NotifyTerminal,
		ToastDuration: 3,
		ToastPosition: ToastBottomRight,
	}
}

// GetToastDuration returns the toast duration, ensuring it's within valid range
func (n NotificationSettings) GetToastDuration() int {
	if n.ToastDuration < 1 {
		return 3
	}
	if n.ToastDuration > 10 {
		return 10
	}
	return n.ToastDuration
}

// GetToastPosition returns the toast position, defaulting to bottom-right
func (n NotificationSettings) GetToastPosition() ToastPosition {
	switch n.ToastPosition {
	case ToastTopLeft, ToastTopRight, ToastBottomLeft, ToastBottomRight:
		return n.ToastPosition
	default:
		return ToastBottomRight
	}
}

// TutorialSettings stores tutorial progress
type TutorialSettings struct {
	Completed bool `json:"completed"`
}

// Config represents the application configuration
type Config struct {
	Notifications *NotificationSettings `json:"notifications,omitempty"`
	Tutorial      *TutorialSettings     `json:"tutorial,omitempty"`
}

// ShouldShowTutorial returns true if the tutorial should be shown
func (c *Config) ShouldShowTutorial() bool {
	return c.Tutorial == nil || !c.Tutorial.Completed
}

// MarkTutorialCompleted marks the tutorial as completed and saves to disk
func (c *Config) MarkTutorialCompleted() error {
	c.Tutorial = &TutorialSettings{Completed: true}
	return c.Save()
}

// Projects represents saved compose projects (stored separately)
type Projects struct {
	SavedProjects map[string]SavedProject `json:"saved_projects"`
}

// GetNotificationSettings returns the configured notification settings or defaults
func (c *Config) GetNotificationSettings() NotificationSettings {
	if c.Notifications != nil {
		return *c.Notifications
	}
	return DefaultNotificationSettings()
}

// configPath returns the full path to the config file
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDir, configFile), nil
}

// GetConfigPath returns the full path to the config file (public version)
func GetConfigPath() string {
	path, err := configPath()
	if err != nil {
		return ""
	}
	return path
}

// keybindingsPath returns the full path to the keybindings file
func keybindingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDir, keybindingsFile), nil
}

// GetKeybindingsPath returns the full path to the keybindings file (public version)
func GetKeybindingsPath() string {
	path, err := keybindingsPath()
	if err != nil {
		return ""
	}
	return path
}

// LoadKeyBindings loads key bindings from the keybindings file
func LoadKeyBindings() KeyBindings {
	path, err := keybindingsPath()
	if err != nil {
		return DefaultKeyBindings()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultKeyBindings()
	}

	var kb KeyBindings
	if err := json.Unmarshal(data, &kb); err != nil {
		return DefaultKeyBindings()
	}

	// Fill in any missing keys with defaults (for forward compatibility)
	defaults := DefaultKeyBindings()
	modified := false

	// Helper to set default if empty
	setDefault := func(current *string, def string) {
		if *current == "" {
			*current = def
			modified = true
		}
	}

	setDefault(&kb.Up, defaults.Up)
	setDefault(&kb.Down, defaults.Down)
	setDefault(&kb.Left, defaults.Left)
	setDefault(&kb.Right, defaults.Right)
	setDefault(&kb.ScrollUp, defaults.ScrollUp)
	setDefault(&kb.ScrollDown, defaults.ScrollDown)
	setDefault(&kb.Top, defaults.Top)
	setDefault(&kb.Bottom, defaults.Bottom)
	setDefault(&kb.NextPane, defaults.NextPane)
	setDefault(&kb.PrevPane, defaults.PrevPane)
	setDefault(&kb.Select, defaults.Select)
	setDefault(&kb.SelectAll, defaults.SelectAll)
	setDefault(&kb.ClearAll, defaults.ClearAll)
	setDefault(&kb.Confirm, defaults.Confirm)
	setDefault(&kb.Back, defaults.Back)
	setDefault(&kb.Start, defaults.Start)
	setDefault(&kb.Stop, defaults.Stop)
	setDefault(&kb.Restart, defaults.Restart)
	setDefault(&kb.Kill, defaults.Kill)
	setDefault(&kb.Remove, defaults.Remove)
	setDefault(&kb.Exec, defaults.Exec)
	setDefault(&kb.Inspect, defaults.Inspect)
	setDefault(&kb.ComposeUp, defaults.ComposeUp)
	setDefault(&kb.ComposeDown, defaults.ComposeDown)
	setDefault(&kb.ComposeRestart, defaults.ComposeRestart)
	setDefault(&kb.ComposeBuild, defaults.ComposeBuild)
	setDefault(&kb.Refresh, defaults.Refresh)
	setDefault(&kb.Search, defaults.Search)
	setDefault(&kb.Help, defaults.Help)
	setDefault(&kb.Quit, defaults.Quit)
	setDefault(&kb.SavedProjects, defaults.SavedProjects)
	setDefault(&kb.Config, defaults.Config)
	setDefault(&kb.CopyLogs, defaults.CopyLogs)
	setDefault(&kb.CopySelection, defaults.CopySelection)
	setDefault(&kb.WordWrap, defaults.WordWrap)
	setDefault(&kb.DebugToggle, defaults.DebugToggle)
	setDefault(&kb.ClearLogs, defaults.ClearLogs)
	setDefault(&kb.PauseLogs, defaults.PauseLogs)
	setDefault(&kb.Pane1, defaults.Pane1)
	setDefault(&kb.Pane2, defaults.Pane2)
	setDefault(&kb.Pane3, defaults.Pane3)
	setDefault(&kb.Pane4, defaults.Pane4)
	setDefault(&kb.Pane5, defaults.Pane5)
	setDefault(&kb.Pane6, defaults.Pane6)
	setDefault(&kb.Pane7, defaults.Pane7)
	setDefault(&kb.Pane8, defaults.Pane8)
	setDefault(&kb.Pane9, defaults.Pane9)
	setDefault(&kb.ResizeLeft, defaults.ResizeLeft)
	setDefault(&kb.ResizeRight, defaults.ResizeRight)
	setDefault(&kb.ResizeUp, defaults.ResizeUp)
	setDefault(&kb.ResizeDown, defaults.ResizeDown)
	setDefault(&kb.ResizeReset, defaults.ResizeReset)

	// Save back to file if any new keys were added
	if modified {
		_ = SaveKeyBindings(kb)
	}

	return kb
}

// SaveKeyBindings saves key bindings to the keybindings file
func SaveKeyBindings(kb KeyBindings) error {
	path, err := keybindingsPath()
	if err != nil {
		return err
	}

	// Create config directory if it doesn't exist
	dir := filepath.Dir(path)
	if mkdirErr := os.MkdirAll(dir, 0755); mkdirErr != nil {
		return mkdirErr
	}

	data, err := json.MarshalIndent(kb, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// projectsPath returns the full path to the projects file
func projectsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDir, projectsFile), nil
}

// GetProjectsPath returns the full path to the projects file (public version)
func GetProjectsPath() string {
	path, err := projectsPath()
	if err != nil {
		return ""
	}
	return path
}

// LoadProjects loads saved projects from the projects file
func LoadProjects() *Projects {
	path, err := projectsPath()
	if err != nil {
		return &Projects{SavedProjects: make(map[string]SavedProject)}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return &Projects{SavedProjects: make(map[string]SavedProject)}
	}

	var p Projects
	if err := json.Unmarshal(data, &p); err != nil {
		return &Projects{SavedProjects: make(map[string]SavedProject)}
	}

	if p.SavedProjects == nil {
		p.SavedProjects = make(map[string]SavedProject)
	}

	return &p
}

// Save saves projects to the projects file
func (p *Projects) Save() error {
	path, err := projectsPath()
	if err != nil {
		return err
	}

	// Create config directory if it doesn't exist
	dir := filepath.Dir(path)
	if mkdirErr := os.MkdirAll(dir, 0755); mkdirErr != nil {
		return mkdirErr
	}

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// RemoveProject removes a project from saved projects
func (p *Projects) RemoveProject(name string) {
	delete(p.SavedProjects, name)
}

// Load loads the config from disk
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty config if file doesn't exist
			return &Config{}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Save saves the config to disk
func (c *Config) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}

	// Create config directory if it doesn't exist
	dir := filepath.Dir(path)
	if mkdirErr := os.MkdirAll(dir, 0755); mkdirErr != nil {
		return mkdirErr
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// EnsureDefaults ensures the config files exist with default values
func EnsureDefaults() error {
	// Config file
	cfg, err := Load()
	if err != nil {
		cfg = &Config{}
	}

	// Set notification defaults
	notifyDefaults := DefaultNotificationSettings()
	cfg.Notifications = &notifyDefaults

	if err := cfg.Save(); err != nil {
		return err
	}

	// Keybindings file - create if doesn't exist, or load to add any missing keys
	kbPath, _ := keybindingsPath()
	if _, err := os.Stat(kbPath); os.IsNotExist(err) {
		if err := SaveKeyBindings(DefaultKeyBindings()); err != nil {
			return err
		}
	} else {
		// Load keybindings to trigger adding any missing keys to the file
		_ = LoadKeyBindings()
	}

	// Projects file
	projPath, _ := projectsPath()
	if _, err := os.Stat(projPath); os.IsNotExist(err) {
		p := &Projects{SavedProjects: make(map[string]SavedProject)}
		if err := p.Save(); err != nil {
			return err
		}
	}

	return nil
}
