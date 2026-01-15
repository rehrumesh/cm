package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"cm/internal/config"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// Cache for compose services to avoid spawning processes on every refresh
var (
	composeServicesCache     = make(map[string][]string)
	composeServicesCacheLock sync.RWMutex
	composeServicesCacheTime = make(map[string]time.Time)
	cacheExpiry              = 30 * time.Second // Cache for 30 seconds

	// Config cache to avoid loading from disk on every refresh
	configCache     *config.Config
	configCacheLock sync.RWMutex
	configCacheTime time.Time
	configDirty     bool

	// Projects cache to avoid loading from disk on every refresh
	projectsCache     *config.Projects
	projectsCacheLock sync.RWMutex
	projectsCacheTime time.Time
	projectsDirty     bool
)

// Client wraps the Docker SDK client
type Client struct {
	cli *client.Client
}

// NewClient creates a new Docker client
func NewClient() (*Client, error) {
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = cli.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("Docker daemon not reachable: %w", err)
	}

	return &Client{cli: cli}, nil
}

// Close closes the Docker client and saves any pending config/project changes
func (c *Client) Close() error {
	SaveConfigIfDirty()
	SaveProjectsIfDirty()
	return c.cli.Close()
}

// ListContainers returns all containers (running and recently exited)
func (c *Client) ListContainers(ctx context.Context) ([]Container, error) {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All: true, // Include exited containers
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	// Load saved projects (cached)
	projects := getCachedProjects()

	result := make([]Container, 0, len(containers))
	projectInfo := make(map[string]composeProjectInfo)

	// Include local compose project from current directory
	localProject, localComposeFile := DetectLocalComposeProjectWithFile()
	if localProject != "" && localComposeFile != "" {
		cwd, _ := os.Getwd()
		projectInfo[localProject] = composeProjectInfo{
			configFile: localComposeFile,
			workingDir: cwd,
		}
		// Auto-save local compose project
		updateProject(localProject, localComposeFile, cwd)
	}

	for _, cont := range containers {
		// Skip containers that have been exited for more than 1 hour
		if cont.State == "exited" {
			// Check if exited recently (within last hour)
			created := time.Unix(cont.Created, 0)
			if time.Since(created) > 24*time.Hour {
				continue
			}
		}

		name := ""
		if len(cont.Names) > 0 {
			name = strings.TrimPrefix(cont.Names[0], "/")
		}

		// Collect compose project info from labels
		project := cont.Labels[LabelComposeProject]
		if project != "" {
			configFile := cont.Labels[LabelComposeConfigFile]
			workingDir := cont.Labels[LabelComposeWorkingDir]

			if _, exists := projectInfo[project]; !exists {
				projectInfo[project] = composeProjectInfo{
					configFile: configFile,
					workingDir: workingDir,
				}
				// Auto-save detected compose projects
				updateProject(project, configFile, workingDir)
			}
		}

		result = append(result, Container{
			ID:             cont.ID[:12],
			Name:           name,
			Status:         cont.Status,
			State:          cont.State,
			ComposeProject: project,
			ComposeService: cont.Labels[LabelComposeService],
			Image:          cont.Image,
			Created:        time.Unix(cont.Created, 0),
		})
	}

	// Merge saved projects into projectInfo (for projects with no running containers)
	for name, proj := range projects.SavedProjects {
		if _, exists := projectInfo[name]; !exists {
			projectInfo[name] = composeProjectInfo{
				configFile: proj.ConfigFile,
				workingDir: proj.WorkingDir,
			}
		}
	}

	// Find stopped services from compose projects
	stoppedServices := c.getStoppedComposeServices(result, projectInfo)
	result = append(result, stoppedServices...)

	return result, nil
}

// composeProjectInfo stores compose file info for a project
type composeProjectInfo struct {
	configFile string
	workingDir string
}

// getStoppedComposeServices finds services defined in compose files that aren't running
func (c *Client) getStoppedComposeServices(containers []Container, projectInfo map[string]composeProjectInfo) []Container {
	// Get unique compose projects and running services
	runningServices := make(map[string]map[string]bool) // project -> service -> exists

	for _, cont := range containers {
		if cont.ComposeProject != "" {
			if runningServices[cont.ComposeProject] == nil {
				runningServices[cont.ComposeProject] = make(map[string]bool)
			}
			if cont.ComposeService != "" {
				runningServices[cont.ComposeProject][cont.ComposeService] = true
			}
		}
	}

	var stopped []Container
	for project, info := range projectInfo {
		// Get all services from compose config using the actual file path
		services := getComposeServices(project, info.configFile, info.workingDir)
		for _, svc := range services {
			if !runningServices[project][svc] {
				stopped = append(stopped, Container{
					ID:             fmt.Sprintf("stopped:%s:%s", project, svc), // Unique ID for stopped services
					Name:           fmt.Sprintf("%s-%s", project, svc),
					Status:         "Not started",
					State:          "stopped",
					ComposeProject: project,
					ComposeService: svc,
				})
			}
		}
	}

	return stopped
}

// getCachedConfig returns the cached config or loads from disk
func getCachedConfig() *config.Config {
	configCacheLock.RLock()
	if configCache != nil && time.Since(configCacheTime) < cacheExpiry {
		configCacheLock.RUnlock()
		return configCache
	}
	configCacheLock.RUnlock()

	configCacheLock.Lock()
	defer configCacheLock.Unlock()

	// Double-check after acquiring write lock
	if configCache != nil && time.Since(configCacheTime) < cacheExpiry {
		return configCache
	}

	// Save dirty config before reloading
	if configDirty && configCache != nil {
		_ = configCache.Save()
		configDirty = false
	}

	cfg, _ := config.Load()
	if cfg == nil {
		cfg = &config.Config{}
	}
	configCache = cfg
	configCacheTime = time.Now()
	return cfg
}

// markConfigDirty marks the config as needing to be saved
func markConfigDirty() {
	configCacheLock.Lock()
	configDirty = true
	configCacheLock.Unlock()
}

// SaveConfigIfDirty saves the config if it has been modified
func SaveConfigIfDirty() {
	configCacheLock.Lock()
	defer configCacheLock.Unlock()
	if configDirty && configCache != nil {
		_ = configCache.Save()
		configDirty = false
	}
}

// getCachedProjects returns the cached projects or loads from disk
func getCachedProjects() *config.Projects {
	projectsCacheLock.RLock()
	if projectsCache != nil && time.Since(projectsCacheTime) < cacheExpiry {
		projectsCacheLock.RUnlock()
		return projectsCache
	}
	projectsCacheLock.RUnlock()

	projectsCacheLock.Lock()
	defer projectsCacheLock.Unlock()

	// Double-check after acquiring write lock
	if projectsCache != nil && time.Since(projectsCacheTime) < cacheExpiry {
		return projectsCache
	}

	// Save dirty projects before reloading
	if projectsDirty && projectsCache != nil {
		_ = projectsCache.Save()
		projectsDirty = false
	}

	projectsCache = config.LoadProjects()
	projectsCacheTime = time.Now()
	return projectsCache
}

// updateProject adds or updates a project in the cache and saves immediately
func updateProject(name string, configFile, workingDir string) {
	if name == "" || (configFile == "" && workingDir == "") {
		return
	}

	projectsCacheLock.Lock()
	defer projectsCacheLock.Unlock()

	if projectsCache == nil {
		projectsCache = config.LoadProjects()
		projectsCacheTime = time.Now()
	}

	// Check if project already exists with same info
	if existing, ok := projectsCache.SavedProjects[name]; ok {
		if existing.ConfigFile == configFile && existing.WorkingDir == workingDir {
			return // No change needed
		}
	}

	// Add or update the project
	projectsCache.SavedProjects[name] = config.SavedProject{
		ConfigFile: configFile,
		WorkingDir: workingDir,
	}

	// Save immediately so it's available when modal opens
	_ = projectsCache.Save()
}

// SaveProjectsIfDirty saves the projects if they have been modified
func SaveProjectsIfDirty() {
	projectsCacheLock.Lock()
	defer projectsCacheLock.Unlock()
	if projectsDirty && projectsCache != nil {
		_ = projectsCache.Save()
		projectsDirty = false
	}
}

// getComposeServices runs docker compose to get service names (cached)
func getComposeServices(project, configFile, workingDir string) []string {
	cacheKey := project + ":" + configFile + ":" + workingDir

	// Check cache first
	composeServicesCacheLock.RLock()
	if services, ok := composeServicesCache[cacheKey]; ok {
		if time.Since(composeServicesCacheTime[cacheKey]) < cacheExpiry {
			composeServicesCacheLock.RUnlock()
			return services
		}
	}
	composeServicesCacheLock.RUnlock()

	var args []string

	// Use specific compose file if available
	if configFile != "" {
		// configFile may contain multiple files separated by comma
		for _, f := range strings.Split(configFile, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				args = append(args, "-f", f)
			}
		}
	}

	args = append(args, "-p", project, "config", "--services")

	cmd := exec.Command("docker", append([]string{"compose"}, args...)...)

	// Set working directory if available
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var services []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line != "" {
			services = append(services, line)
		}
	}

	// Store in cache
	composeServicesCacheLock.Lock()
	composeServicesCache[cacheKey] = services
	composeServicesCacheTime[cacheKey] = time.Now()
	composeServicesCacheLock.Unlock()

	return services
}

// StopContainer stops a container gracefully
func (c *Client) StopContainer(ctx context.Context, containerID string) error {
	timeout := 10 // seconds
	return c.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

// KillContainer forcefully kills a container
func (c *Client) KillContainer(ctx context.Context, containerID string) error {
	return c.cli.ContainerKill(ctx, containerID, "SIGKILL")
}

// RestartContainer restarts a container
func (c *Client) RestartContainer(ctx context.Context, containerID string) error {
	timeout := 10 // seconds
	return c.cli.ContainerRestart(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

// StartContainer starts a stopped container
func (c *Client) StartContainer(ctx context.Context, containerID string) error {
	return c.cli.ContainerStart(ctx, containerID, container.StartOptions{})
}

// ComposeUp runs docker compose up -d for a specific service
func (c *Client) ComposeUp(ctx context.Context, cont Container) error {
	if cont.ComposeProject == "" || cont.ComposeService == "" {
		return fmt.Errorf("container is not part of a compose project")
	}

	// Get compose file info from projects (cached)
	projects := getCachedProjects()
	var configFile, workingDir string
	if proj, ok := projects.SavedProjects[cont.ComposeProject]; ok {
		configFile = proj.ConfigFile
		workingDir = proj.WorkingDir
	}

	// Build compose args
	var baseArgs []string
	if configFile != "" {
		for _, f := range strings.Split(configFile, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				baseArgs = append(baseArgs, "-f", f)
			}
		}
	}
	baseArgs = append(baseArgs, "-p", cont.ComposeProject)

	// Run compose up for the service
	upArgs := append(baseArgs, "up", "-d", cont.ComposeService)
	upCmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, upArgs...)...)
	if workingDir != "" {
		upCmd.Dir = workingDir
	}
	return upCmd.Run()
}

// ComposeDown runs docker compose down for a specific service (stop only)
func (c *Client) ComposeDown(ctx context.Context, cont Container) error {
	if cont.ComposeProject == "" || cont.ComposeService == "" {
		return fmt.Errorf("container is not part of a compose project")
	}

	projects := getCachedProjects()
	var configFile, workingDir string
	if proj, ok := projects.SavedProjects[cont.ComposeProject]; ok {
		configFile = proj.ConfigFile
		workingDir = proj.WorkingDir
	}

	var baseArgs []string
	if configFile != "" {
		for _, f := range strings.Split(configFile, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				baseArgs = append(baseArgs, "-f", f)
			}
		}
	}
	baseArgs = append(baseArgs, "-p", cont.ComposeProject)

	downArgs := append(baseArgs, "down", cont.ComposeService)
	downCmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, downArgs...)...)
	if workingDir != "" {
		downCmd.Dir = workingDir
	}
	return downCmd.Run()
}

// ComposeDownUp runs docker compose down then up for a specific service
func (c *Client) ComposeDownUp(ctx context.Context, cont Container) error {
	if cont.ComposeProject == "" || cont.ComposeService == "" {
		return fmt.Errorf("container is not part of a compose project")
	}

	projects := getCachedProjects()
	var configFile, workingDir string
	if proj, ok := projects.SavedProjects[cont.ComposeProject]; ok {
		configFile = proj.ConfigFile
		workingDir = proj.WorkingDir
	}

	var baseArgs []string
	if configFile != "" {
		for _, f := range strings.Split(configFile, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				baseArgs = append(baseArgs, "-f", f)
			}
		}
	}
	baseArgs = append(baseArgs, "-p", cont.ComposeProject)

	// Run compose down for the service
	downArgs := append(baseArgs, "down", cont.ComposeService)
	downCmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, downArgs...)...)
	if workingDir != "" {
		downCmd.Dir = workingDir
	}
	_ = downCmd.Run()

	// Run compose up for the service
	upArgs := append(baseArgs, "up", "-d", cont.ComposeService)
	upCmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, upArgs...)...)
	if workingDir != "" {
		upCmd.Dir = workingDir
	}
	return upCmd.Run()
}

// ComposeBuildUp runs docker compose build --no-cache then up for a specific service
func (c *Client) ComposeBuildUp(ctx context.Context, cont Container) error {
	if cont.ComposeProject == "" || cont.ComposeService == "" {
		return fmt.Errorf("container is not part of a compose project")
	}

	// Get compose file info from projects (cached)
	projects := getCachedProjects()
	var configFile, workingDir string
	if proj, ok := projects.SavedProjects[cont.ComposeProject]; ok {
		configFile = proj.ConfigFile
		workingDir = proj.WorkingDir
	}

	// Build compose args
	var baseArgs []string
	if configFile != "" {
		for _, f := range strings.Split(configFile, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				baseArgs = append(baseArgs, "-f", f)
			}
		}
	}
	baseArgs = append(baseArgs, "-p", cont.ComposeProject)

	// Run compose build --no-cache for the service
	buildArgs := append(baseArgs, "build", "--no-cache", cont.ComposeService)
	buildCmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, buildArgs...)...)
	if workingDir != "" {
		buildCmd.Dir = workingDir
	}
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	// Run compose up for the service
	upArgs := append(baseArgs, "up", "-d", cont.ComposeService)
	upCmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, upArgs...)...)
	if workingDir != "" {
		upCmd.Dir = workingDir
	}
	return upCmd.Run()
}
