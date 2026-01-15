package docker

import (
	"os"
	"os/exec"
	"strings"

	"cm/internal/config"

	"gopkg.in/yaml.v3"
)

// ComposeService represents a service in compose file
type ComposeService struct {
	DependsOn interface{} `yaml:"depends_on"`
}

// ComposeFile represents a docker-compose file structure
type ComposeFile struct {
	Services map[string]ComposeService `yaml:"services"`
}

// infraCache caches infrastructure services per project to avoid repeated parsing
// Infrastructure services = services that ARE depended upon by other services
var infraCache = make(map[string]map[string]bool)

// ClearInfraCache clears the infrastructure services cache
func ClearInfraCache() {
	infraCache = make(map[string]map[string]bool)
}

// GetInfrastructureServices returns services that are depended upon by other services
func GetInfrastructureServices(project string) map[string]bool {
	// Return cached result if available
	if cached, ok := infraCache[project]; ok {
		return cached
	}

	infra := make(map[string]bool)

	// Get compose file info from config
	cfg, _ := config.Load()
	var configFile, workingDir string
	if cfg != nil {
		if proj, ok := cfg.ComposeProjects[project]; ok {
			configFile = proj.ConfigFile
			workingDir = proj.WorkingDir
		}
	}

	// Build compose command args
	var args []string
	if configFile != "" {
		for _, f := range strings.Split(configFile, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				args = append(args, "-f", f)
			}
		}
	}
	args = append(args, "-p", project, "config", "--format", "yaml")

	cmd := exec.Command("docker", append([]string{"compose"}, args...)...)
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	output, err := cmd.Output()
	if err != nil {
		infraCache[project] = infra
		return infra
	}

	var compose ComposeFile
	if err := yaml.Unmarshal(output, &compose); err != nil {
		infraCache[project] = infra
		return infra
	}

	// Collect all services that are depended upon by other services
	// These are infrastructure/dependency services
	for _, svc := range compose.Services {
		if svc.DependsOn == nil {
			continue
		}
		switch v := svc.DependsOn.(type) {
		case []interface{}:
			for _, dep := range v {
				if depName, ok := dep.(string); ok {
					infra[depName] = true
				}
			}
		case map[string]interface{}:
			for depName := range v {
				infra[depName] = true
			}
		}
	}

	infraCache[project] = infra
	return infra
}

// GetInfrastructureServicesFromFile parses a compose file directly
func GetInfrastructureServicesFromFile(filePath string) map[string]bool {
	infra := make(map[string]bool)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return infra
	}

	var compose ComposeFile
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return infra
	}

	for name, svc := range compose.Services {
		if svc.DependsOn == nil {
			infra[name] = true
		} else {
			switch v := svc.DependsOn.(type) {
			case []interface{}:
				if len(v) == 0 {
					infra[name] = true
				}
			case map[string]interface{}:
				if len(v) == 0 {
					infra[name] = true
				}
			}
		}
	}

	return infra
}
