package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	configDir  = ".cm"
	configFile = "config.json"
)

// ComposeProject stores compose file info for a project
type ComposeProject struct {
	ConfigFile string `json:"config_file"`
	WorkingDir string `json:"working_dir"`
}

// KeyBindings stores all configurable key bindings
type KeyBindings struct {
	Up             string `json:"up"`
	Down           string `json:"down"`
	Left           string `json:"left"`
	Right          string `json:"right"`
	ScrollUp       string `json:"scroll_up"`
	ScrollDown     string `json:"scroll_down"`
	Select         string `json:"select"`
	Confirm        string `json:"confirm"`
	Back           string `json:"back"`
	Quit           string `json:"quit"`
	Help           string `json:"help"`
	Refresh        string `json:"refresh"`
	NextPane       string `json:"next_pane"`
	PrevPane       string `json:"prev_pane"`
	Start          string `json:"start"`
	Restart        string `json:"restart"`
	ComposeRestart string `json:"compose_restart"`
	ComposeBuild   string `json:"compose_build"`
}

// DefaultKeyBindings returns the default key bindings
func DefaultKeyBindings() KeyBindings {
	return KeyBindings{
		Up:             "up",
		Down:           "down",
		Left:           "left",
		Right:          "right",
		ScrollUp:       "k",
		ScrollDown:     "j",
		Select:         "space,x",
		Confirm:        "enter",
		Back:           "esc,backspace",
		Quit:           "q,ctrl+c",
		Help:           "?",
		Refresh:        "r",
		NextPane:       "tab,l",
		PrevPane:       "shift+tab,h",
		Start:          "s",
		Restart:        "r",
		ComposeRestart: "R",
		ComposeBuild:   "B",
	}
}

// Config represents the application configuration
type Config struct {
	ComposeProjects        map[string]ComposeProject `json:"compose_projects"`
	InfrastructureServices []string                  `json:"infrastructure_services,omitempty"`
	KeyBindings            *KeyBindings              `json:"key_bindings,omitempty"`
}

// DefaultInfrastructureServices returns common infrastructure service patterns
func DefaultInfrastructureServices() []string {
	return []string{
		"db", "database", "postgres", "postgresql", "mysql", "mariadb", "mongo", "mongodb",
		"redis", "memcached", "cache",
		"db-setup", "db-migrate", "migrate", "migration", "seed", "seeder",
		"temporal", "temporal-admin", "temporal-ui", "temporal-worker",
		"kafka", "zookeeper", "rabbitmq", "nats",
		"elasticsearch", "kibana", "logstash",
		"prometheus", "grafana", "jaeger",
		"mailhog", "mailpit", "smtp",
		"minio", "s3",
		"nginx", "traefik", "caddy", "proxy",
	}
}

// GetInfrastructureServices returns the configured infrastructure services or defaults
func (c *Config) GetInfrastructureServices() []string {
	if len(c.InfrastructureServices) > 0 {
		return c.InfrastructureServices
	}
	return DefaultInfrastructureServices()
}

// GetKeyBindings returns the configured key bindings or defaults
func (c *Config) GetKeyBindings() KeyBindings {
	if c.KeyBindings != nil {
		return *c.KeyBindings
	}
	return DefaultKeyBindings()
}

// IsInfrastructureService checks if a service name matches infrastructure patterns
func (c *Config) IsInfrastructureService(serviceName string) bool {
	name := strings.ToLower(serviceName)
	for _, pattern := range c.GetInfrastructureServices() {
		pattern = strings.ToLower(pattern)
		if name == pattern || strings.HasPrefix(name, pattern+"-") || strings.HasSuffix(name, "-"+pattern) {
			return true
		}
	}
	return false
}

// configPath returns the full path to the config file
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDir, configFile), nil
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
			return &Config{
				ComposeProjects: make(map[string]ComposeProject),
			}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.ComposeProjects == nil {
		cfg.ComposeProjects = make(map[string]ComposeProject)
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

// EnsureDefaults ensures the config file exists with default values
func EnsureDefaults() error {
	cfg, err := Load()
	if err != nil {
		cfg = &Config{
			ComposeProjects: make(map[string]ComposeProject),
		}
	}

	// Add default key bindings if not present
	if cfg.KeyBindings == nil {
		defaults := DefaultKeyBindings()
		cfg.KeyBindings = &defaults
	}

	return cfg.Save()
}

// UpdateProject updates or adds a compose project
func (c *Config) UpdateProject(name string, project ComposeProject) {
	if c.ComposeProjects == nil {
		c.ComposeProjects = make(map[string]ComposeProject)
	}
	// Only update if we have valid info
	if project.ConfigFile != "" && project.WorkingDir != "" {
		c.ComposeProjects[name] = project
	}
}

// RemoveStaleProjects removes projects whose compose files no longer exist
func (c *Config) RemoveStaleProjects() {
	for name, project := range c.ComposeProjects {
		// Check if working dir exists
		if project.WorkingDir == "" {
			delete(c.ComposeProjects, name)
			continue
		}
		if _, err := os.Stat(project.WorkingDir); os.IsNotExist(err) {
			delete(c.ComposeProjects, name)
			continue
		}
		// Check if config file exists
		if project.ConfigFile == "" {
			delete(c.ComposeProjects, name)
			continue
		}
		// Just check that working dir exists - compose file path might be relative
		// and we don't want to over-prune valid projects
	}
}
