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
	WordWrap      string `json:"word_wrap"`
	DebugToggle   string `json:"debug_toggle"`
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
		NextPane:   "tab,]",
		PrevPane:   "shift+tab,[",

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
		Quit:          "q,ctrl+c",
		SavedProjects: "p",
		Config:        "c",
		CopyLogs:      "y",
		WordWrap:      "w",
		DebugToggle:   "ctrl+g",
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

// Config represents the application configuration
type Config struct {
	Notifications *NotificationSettings `json:"notifications,omitempty"`
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
	if kb.Up == "" {
		kb.Up = defaults.Up
	}
	if kb.Down == "" {
		kb.Down = defaults.Down
	}
	if kb.Left == "" {
		kb.Left = defaults.Left
	}
	if kb.Right == "" {
		kb.Right = defaults.Right
	}
	if kb.ScrollUp == "" {
		kb.ScrollUp = defaults.ScrollUp
	}
	if kb.ScrollDown == "" {
		kb.ScrollDown = defaults.ScrollDown
	}
	if kb.Top == "" {
		kb.Top = defaults.Top
	}
	if kb.Bottom == "" {
		kb.Bottom = defaults.Bottom
	}
	if kb.NextPane == "" {
		kb.NextPane = defaults.NextPane
	}
	if kb.PrevPane == "" {
		kb.PrevPane = defaults.PrevPane
	}
	if kb.Select == "" {
		kb.Select = defaults.Select
	}
	if kb.SelectAll == "" {
		kb.SelectAll = defaults.SelectAll
	}
	if kb.ClearAll == "" {
		kb.ClearAll = defaults.ClearAll
	}
	if kb.Confirm == "" {
		kb.Confirm = defaults.Confirm
	}
	if kb.Back == "" {
		kb.Back = defaults.Back
	}
	if kb.Start == "" {
		kb.Start = defaults.Start
	}
	if kb.Stop == "" {
		kb.Stop = defaults.Stop
	}
	if kb.Restart == "" {
		kb.Restart = defaults.Restart
	}
	if kb.Kill == "" {
		kb.Kill = defaults.Kill
	}
	if kb.Remove == "" {
		kb.Remove = defaults.Remove
	}
	if kb.Exec == "" {
		kb.Exec = defaults.Exec
	}
	if kb.Inspect == "" {
		kb.Inspect = defaults.Inspect
	}
	if kb.ComposeUp == "" {
		kb.ComposeUp = defaults.ComposeUp
	}
	if kb.ComposeDown == "" {
		kb.ComposeDown = defaults.ComposeDown
	}
	if kb.ComposeRestart == "" {
		kb.ComposeRestart = defaults.ComposeRestart
	}
	if kb.ComposeBuild == "" {
		kb.ComposeBuild = defaults.ComposeBuild
	}
	if kb.Refresh == "" {
		kb.Refresh = defaults.Refresh
	}
	if kb.Search == "" {
		kb.Search = defaults.Search
	}
	if kb.Help == "" {
		kb.Help = defaults.Help
	}
	if kb.Quit == "" {
		kb.Quit = defaults.Quit
	}
	if kb.SavedProjects == "" {
		kb.SavedProjects = defaults.SavedProjects
	}
	if kb.Config == "" {
		kb.Config = defaults.Config
	}
	if kb.CopyLogs == "" {
		kb.CopyLogs = defaults.CopyLogs
	}
	if kb.WordWrap == "" {
		kb.WordWrap = defaults.WordWrap
	}
	if kb.DebugToggle == "" {
		kb.DebugToggle = defaults.DebugToggle
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

	// Keybindings file
	kbPath, _ := keybindingsPath()
	if _, err := os.Stat(kbPath); os.IsNotExist(err) {
		if err := SaveKeyBindings(DefaultKeyBindings()); err != nil {
			return err
		}
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

