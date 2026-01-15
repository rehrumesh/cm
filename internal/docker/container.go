package docker

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	LabelComposeProject    = "com.docker.compose.project"
	LabelComposeService    = "com.docker.compose.service"
	LabelComposeConfigFile = "com.docker.compose.project.config_files"
	LabelComposeWorkingDir = "com.docker.compose.project.working_dir"
)

// Container represents a Docker container with compose metadata
type Container struct {
	ID             string
	Name           string
	Status         string
	State          string
	ComposeProject string
	ComposeService string
	Image          string
	Created        time.Time
}

// DisplayName returns the best name to display for the container
func (c Container) DisplayName() string {
	if c.ComposeService != "" {
		return c.ComposeService
	}
	return c.Name
}

// ContainerGroup groups containers by compose project
type ContainerGroup struct {
	ProjectName string
	Containers  []Container
}

// composeFileHeader is used to parse just the name field from compose files
type composeFileHeader struct {
	Name string `yaml:"name"`
}

// DetectLocalComposeProject checks if current directory has a compose file
// and returns the project name (from compose file's name field or directory name)
func DetectLocalComposeProject() string {
	name, _ := DetectLocalComposeProjectWithFile()
	return name
}

// DetectLocalComposeProjectWithFile checks if current directory has a compose file
// and returns both the project name and the compose file path
func DetectLocalComposeProjectWithFile() (projectName string, composeFile string) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", ""
	}

	// Check for compose files
	composeFiles := []string{
		"compose.yml",
		"compose.yaml",
		"docker-compose.yml",
		"docker-compose.yaml",
	}

	for _, f := range composeFiles {
		filePath := filepath.Join(cwd, f)
		if _, err := os.Stat(filePath); err == nil {
			// Found a compose file - check for name field
			if name := getComposeProjectName(filePath, cwd); name != "" {
				return name, filePath
			}
			// Fall back to directory name
			return filepath.Base(cwd), filePath
		}
	}

	return "", ""
}

// getComposeProjectName extracts the project name from a compose file
// It checks the name field in the file, or uses docker compose config as fallback
func getComposeProjectName(filePath, workingDir string) string {
	// First try to parse the file directly for the name field
	data, err := os.ReadFile(filePath)
	if err == nil {
		var header composeFileHeader
		if yaml.Unmarshal(data, &header) == nil && header.Name != "" {
			return header.Name
		}
	}

	// Fallback: use docker compose config to get the resolved project name
	// This handles environment variables and other compose features
	cmd := exec.Command("docker", "compose", "-f", filePath, "config", "--format", "json")
	cmd.Dir = workingDir
	output, err := cmd.Output()
	if err == nil {
		// Simple JSON parsing for just the name field
		// Format: {"name": "projectname", ...}
		if idx := strings.Index(string(output), `"name":`); idx != -1 {
			rest := string(output)[idx+7:]
			rest = strings.TrimSpace(rest)
			if len(rest) > 0 && rest[0] == '"' {
				rest = rest[1:]
				if endIdx := strings.Index(rest, `"`); endIdx != -1 {
					return rest[:endIdx]
				}
			}
		}
	}

	return ""
}

// GroupByComposeProject groups containers by their compose project
// If priorityProject is set, that project will be listed first
func GroupByComposeProject(containers []Container, priorityProject string) []ContainerGroup {
	groups := make(map[string][]Container)
	var standalone []Container

	for _, c := range containers {
		if c.ComposeProject != "" {
			groups[c.ComposeProject] = append(groups[c.ComposeProject], c)
		} else {
			standalone = append(standalone, c)
		}
	}

	result := make([]ContainerGroup, 0, len(groups)+1)

	// Sort project names for consistent ordering
	projectNames := make([]string, 0, len(groups))
	for name := range groups {
		projectNames = append(projectNames, name)
	}
	sort.Strings(projectNames)

	// If we have a priority project, move it to the front
	if priorityProject != "" {
		for i, name := range projectNames {
			if name == priorityProject {
				// Remove from current position and prepend
				projectNames = append(projectNames[:i], projectNames[i+1:]...)
				projectNames = append([]string{priorityProject}, projectNames...)
				break
			}
		}
	}

	for _, name := range projectNames {
		projectContainers := groups[name]

		// Sort containers alphabetically
		sort.Slice(projectContainers, func(i, j int) bool {
			return projectContainers[i].DisplayName() < projectContainers[j].DisplayName()
		})

		result = append(result, ContainerGroup{
			ProjectName: name,
			Containers:  projectContainers,
		})
	}

	// Add standalone containers at the end
	if len(standalone) > 0 {
		// Sort standalone containers too
		sort.Slice(standalone, func(i, j int) bool {
			return standalone[i].DisplayName() < standalone[j].DisplayName()
		})
		result = append(result, ContainerGroup{
			ProjectName: "(standalone)",
			Containers:  standalone,
		})
	}

	return result
}
