package docker

import (
	"context"
	"encoding/json"
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

// RemoveContainer removes a container (force removes if running)
func (c *Client) RemoveContainer(ctx context.Context, containerID string) error {
	return c.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
}

// InspectContainer returns detailed information about a container
func (c *Client) InspectContainer(ctx context.Context, containerID string) (*ContainerDetails, error) {
	info, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	details := &ContainerDetails{
		ID:     info.ID[:12],
		Name:   strings.TrimPrefix(info.Name, "/"),
		Image:  info.Config.Image,
		Status: info.State.Status,
		State:  info.State.Status,
		Labels: info.Config.Labels,
	}

	// Parse created time
	if created, err := time.Parse(time.RFC3339Nano, info.Created); err == nil {
		details.Created = created
	}

	// Parse started time
	if info.State.StartedAt != "" {
		if started, err := time.Parse(time.RFC3339Nano, info.State.StartedAt); err == nil {
			details.Started = started
		}
	}

	// Get port mappings
	for port, bindings := range info.NetworkSettings.Ports {
		for _, binding := range bindings {
			details.Ports = append(details.Ports, fmt.Sprintf("%s:%s->%s", binding.HostIP, binding.HostPort, port))
		}
	}

	// Get environment variables (filter sensitive ones)
	for _, env := range info.Config.Env {
		// Store raw env for optional viewing
		details.RawEnv = append(details.RawEnv, env)

		// Redact sensitive environment variables
		lower := strings.ToLower(env)
		if strings.Contains(lower, "password") || strings.Contains(lower, "secret") ||
			strings.Contains(lower, "token") || strings.Contains(lower, "key") ||
			strings.Contains(lower, "credential") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				details.Env = append(details.Env, parts[0]+"=<redacted>")
			}
		} else {
			details.Env = append(details.Env, env)
		}
	}

	// Get volume mounts
	for _, mount := range info.Mounts {
		details.Volumes = append(details.Volumes, fmt.Sprintf("%s:%s", mount.Source, mount.Destination))
	}

	// Get networks
	for name := range info.NetworkSettings.Networks {
		details.Networks = append(details.Networks, name)
	}

	// Command and entrypoint
	if len(info.Config.Cmd) > 0 {
		details.Command = strings.Join(info.Config.Cmd, " ")
	}
	if len(info.Config.Entrypoint) > 0 {
		details.Entrypoint = strings.Join(info.Config.Entrypoint, " ")
	}

	details.WorkingDir = info.Config.WorkingDir
	details.RestartPolicy = string(info.HostConfig.RestartPolicy.Name)

	return details, nil
}

// RestartContainer restarts a container
func (c *Client) RestartContainer(ctx context.Context, containerID string) error {
	timeout := 10 // seconds
	return c.cli.ContainerRestart(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

// StreamStats streams container resource stats
func (c *Client) StreamStats(ctx context.Context, containerID string) (<-chan ContainerStats, <-chan error) {
	statsChan := make(chan ContainerStats, 10)
	errChan := make(chan error, 1)

	go func() {
		defer close(statsChan)
		defer close(errChan)

		resp, err := c.cli.ContainerStats(ctx, containerID, true)
		if err != nil {
			errChan <- fmt.Errorf("failed to get container stats: %w", err)
			return
		}
		defer resp.Body.Close()

		decoder := json.NewDecoder(resp.Body)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				var statsJSON container.StatsResponse
				if err := decoder.Decode(&statsJSON); err != nil {
					if err != context.Canceled && ctx.Err() == nil {
						errChan <- err
					}
					return
				}

				stats := ContainerStats{
					Timestamp: time.Now(),
				}

				// Calculate CPU percentage
				cpuDelta := float64(statsJSON.CPUStats.CPUUsage.TotalUsage - statsJSON.PreCPUStats.CPUUsage.TotalUsage)
				systemDelta := float64(statsJSON.CPUStats.SystemUsage - statsJSON.PreCPUStats.SystemUsage)
				if systemDelta > 0 && cpuDelta > 0 {
					cpuCount := float64(statsJSON.CPUStats.OnlineCPUs)
					if cpuCount == 0 {
						cpuCount = float64(len(statsJSON.CPUStats.CPUUsage.PercpuUsage))
					}
					if cpuCount == 0 {
						cpuCount = 1
					}
					stats.CPUPercent = (cpuDelta / systemDelta) * cpuCount * 100.0
				}

				// Memory stats
				stats.MemoryUsage = statsJSON.MemoryStats.Usage
				stats.MemoryLimit = statsJSON.MemoryStats.Limit
				if stats.MemoryLimit > 0 {
					stats.MemoryPercent = float64(stats.MemoryUsage) / float64(stats.MemoryLimit) * 100.0
				}

				// Network stats
				for _, netStats := range statsJSON.Networks {
					stats.NetworkRx += netStats.RxBytes
					stats.NetworkTx += netStats.TxBytes
				}

				// PIDs
				stats.PIDs = statsJSON.PidsStats.Current

				select {
				case statsChan <- stats:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return statsChan, errChan
}

// GetTopProcesses returns running processes in a container
func (c *Client) GetTopProcesses(ctx context.Context, containerID string) ([]ContainerProcess, error) {
	top, err := c.cli.ContainerTop(ctx, containerID, []string{})
	if err != nil {
		return nil, fmt.Errorf("failed to get container processes: %w", err)
	}

	// Find column indices
	var pidIdx, userIdx, timeIdx, cmdIdx int = -1, -1, -1, -1
	for i, title := range top.Titles {
		switch strings.ToUpper(title) {
		case "PID":
			pidIdx = i
		case "USER", "UID":
			userIdx = i
		case "TIME":
			timeIdx = i
		case "CMD", "COMMAND":
			cmdIdx = i
		}
	}

	var processes []ContainerProcess
	for _, proc := range top.Processes {
		p := ContainerProcess{}
		if pidIdx >= 0 && pidIdx < len(proc) {
			p.PID = proc[pidIdx]
		}
		if userIdx >= 0 && userIdx < len(proc) {
			p.User = proc[userIdx]
		}
		if timeIdx >= 0 && timeIdx < len(proc) {
			p.Time = proc[timeIdx]
		}
		if cmdIdx >= 0 && cmdIdx < len(proc) {
			p.Command = proc[cmdIdx]
		} else if len(proc) > 0 {
			// If no command column found, use the last column
			p.Command = proc[len(proc)-1]
		}
		processes = append(processes, p)
	}

	return processes, nil
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

// OperationLog represents a log line from a compose operation
type OperationLog struct {
	Timestamp time.Time
	Stream    string // "stdout", "stderr", "system"
	Content   string
}

// StreamingResult holds channels for operation output
type StreamingResult struct {
	LogChan  <-chan OperationLog
	ErrChan  <-chan error
	DoneChan <-chan struct{}
}

// getComposeBaseArgs returns the base args for compose commands
func getComposeBaseArgs(cont Container) (baseArgs []string, workingDir string) {
	projects := getCachedProjects()
	var configFile string
	if proj, ok := projects.SavedProjects[cont.ComposeProject]; ok {
		configFile = proj.ConfigFile
		workingDir = proj.WorkingDir
	}

	if configFile != "" {
		for _, f := range strings.Split(configFile, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				baseArgs = append(baseArgs, "-f", f)
			}
		}
	}
	baseArgs = append(baseArgs, "-p", cont.ComposeProject)
	return baseArgs, workingDir
}

// runStreamingCommand executes a command and streams output to channels
func runStreamingCommand(ctx context.Context, cmd *exec.Cmd) StreamingResult {
	logChan := make(chan OperationLog, 100)
	errChan := make(chan error, 1)
	doneChan := make(chan struct{})

	go func() {
		defer close(logChan)
		defer close(errChan)
		defer close(doneChan)

		// Get stdout and stderr pipes
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			errChan <- fmt.Errorf("failed to get stdout pipe: %w", err)
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			errChan <- fmt.Errorf("failed to get stderr pipe: %w", err)
			return
		}

		// Start the command
		if err := cmd.Start(); err != nil {
			errChan <- fmt.Errorf("failed to start command: %w", err)
			return
		}

		// Read stdout and stderr concurrently
		var wg sync.WaitGroup
		wg.Add(2)

		// Read stdout
		go func() {
			defer wg.Done()
			scanner := newLineScanner(stdout)
			for scanner.Scan() {
				select {
				case <-ctx.Done():
					return
				case logChan <- OperationLog{
					Timestamp: time.Now(),
					Stream:    "stdout",
					Content:   scanner.Text(),
				}:
				}
			}
		}()

		// Read stderr
		go func() {
			defer wg.Done()
			scanner := newLineScanner(stderr)
			for scanner.Scan() {
				select {
				case <-ctx.Done():
					return
				case logChan <- OperationLog{
					Timestamp: time.Now(),
					Stream:    "stderr",
					Content:   scanner.Text(),
				}:
				}
			}
		}()

		// Wait for readers to finish
		wg.Wait()

		// Wait for command to complete
		if err := cmd.Wait(); err != nil {
			errChan <- err
		}
	}()

	return StreamingResult{
		LogChan:  logChan,
		ErrChan:  errChan,
		DoneChan: doneChan,
	}
}

// newLineScanner creates a scanner that handles long lines
func newLineScanner(r interface{ Read([]byte) (int, error) }) *lineScanner {
	return &lineScanner{reader: r, buf: make([]byte, 0, 4096)}
}

type lineScanner struct {
	reader interface{ Read([]byte) (int, error) }
	buf    []byte
	line   string
	err    error
}

func (s *lineScanner) Scan() bool {
	for {
		// Check if we have a complete line in the buffer
		if idx := strings.Index(string(s.buf), "\n"); idx >= 0 {
			s.line = string(s.buf[:idx])
			s.buf = s.buf[idx+1:]
			return true
		}

		// Read more data
		tmp := make([]byte, 4096)
		n, err := s.reader.Read(tmp)
		if n > 0 {
			s.buf = append(s.buf, tmp[:n]...)
		}
		if err != nil {
			// If we have remaining data, return it as the last line
			if len(s.buf) > 0 {
				s.line = string(s.buf)
				s.buf = s.buf[:0]
				return true
			}
			s.err = err
			return false
		}
	}
}

func (s *lineScanner) Text() string {
	return s.line
}

// ComposeBuildStream runs docker compose build --no-cache with streaming output
func (c *Client) ComposeBuildStream(ctx context.Context, cont Container) StreamingResult {
	if cont.ComposeProject == "" || cont.ComposeService == "" {
		errChan := make(chan error, 1)
		logChan := make(chan OperationLog)
		doneChan := make(chan struct{})
		errChan <- fmt.Errorf("container is not part of a compose project")
		close(errChan)
		close(logChan)
		close(doneChan)
		return StreamingResult{LogChan: logChan, ErrChan: errChan, DoneChan: doneChan}
	}

	baseArgs, workingDir := getComposeBaseArgs(cont)
	buildArgs := append(baseArgs, "build", "--no-cache", "--progress=plain", cont.ComposeService)
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, buildArgs...)...)
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	return runStreamingCommand(ctx, cmd)
}

// ComposeUpStream runs docker compose up -d with streaming output
func (c *Client) ComposeUpStream(ctx context.Context, cont Container) StreamingResult {
	if cont.ComposeProject == "" || cont.ComposeService == "" {
		errChan := make(chan error, 1)
		logChan := make(chan OperationLog)
		doneChan := make(chan struct{})
		errChan <- fmt.Errorf("container is not part of a compose project")
		close(errChan)
		close(logChan)
		close(doneChan)
		return StreamingResult{LogChan: logChan, ErrChan: errChan, DoneChan: doneChan}
	}

	baseArgs, workingDir := getComposeBaseArgs(cont)
	upArgs := append(baseArgs, "up", "-d", cont.ComposeService)
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, upArgs...)...)
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	return runStreamingCommand(ctx, cmd)
}

// ComposeDownStream runs docker compose down with streaming output
func (c *Client) ComposeDownStream(ctx context.Context, cont Container) StreamingResult {
	if cont.ComposeProject == "" || cont.ComposeService == "" {
		errChan := make(chan error, 1)
		logChan := make(chan OperationLog)
		doneChan := make(chan struct{})
		errChan <- fmt.Errorf("container is not part of a compose project")
		close(errChan)
		close(logChan)
		close(doneChan)
		return StreamingResult{LogChan: logChan, ErrChan: errChan, DoneChan: doneChan}
	}

	baseArgs, workingDir := getComposeBaseArgs(cont)
	downArgs := append(baseArgs, "down", cont.ComposeService)
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, downArgs...)...)
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	return runStreamingCommand(ctx, cmd)
}

// ComposeBuildUpStreamMulti runs docker compose build --no-cache then up -d for multiple services with streaming output
func (c *Client) ComposeBuildUpStreamMulti(ctx context.Context, containers []Container) StreamingResult {
	if len(containers) == 0 {
		errChan := make(chan error, 1)
		logChan := make(chan OperationLog)
		doneChan := make(chan struct{})
		errChan <- fmt.Errorf("no containers provided")
		close(errChan)
		close(logChan)
		close(doneChan)
		return StreamingResult{LogChan: logChan, ErrChan: errChan, DoneChan: doneChan}
	}

	// All containers must be from the same project
	project := containers[0].ComposeProject
	for _, cont := range containers {
		if cont.ComposeProject != project {
			errChan := make(chan error, 1)
			logChan := make(chan OperationLog)
			doneChan := make(chan struct{})
			errChan <- fmt.Errorf("all containers must be from the same compose project")
			close(errChan)
			close(logChan)
			close(doneChan)
			return StreamingResult{LogChan: logChan, ErrChan: errChan, DoneChan: doneChan}
		}
	}

	// Collect service names
	var services []string
	for _, cont := range containers {
		if cont.ComposeService != "" {
			services = append(services, cont.ComposeService)
		}
	}

	if len(services) == 0 {
		errChan := make(chan error, 1)
		logChan := make(chan OperationLog)
		doneChan := make(chan struct{})
		errChan <- fmt.Errorf("no compose services found")
		close(errChan)
		close(logChan)
		close(doneChan)
		return StreamingResult{LogChan: logChan, ErrChan: errChan, DoneChan: doneChan}
	}

	logChan := make(chan OperationLog, 100)
	errChan := make(chan error, 1)
	doneChan := make(chan struct{})

	go func() {
		defer close(logChan)
		defer close(errChan)
		defer close(doneChan)

		baseArgs, workingDir := getComposeBaseArgs(containers[0])

		// Phase 1: Build all services
		logChan <- OperationLog{
			Timestamp: time.Now(),
			Stream:    "system",
			Content:   fmt.Sprintf("--- Building %d services (no-cache) ---", len(services)),
		}

		buildArgs := append(baseArgs, "build", "--no-cache", "--progress=plain")
		buildArgs = append(buildArgs, services...)
		buildCmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, buildArgs...)...)
		if workingDir != "" {
			buildCmd.Dir = workingDir
		}

		buildResult := runStreamingCommand(ctx, buildCmd)

		// Forward build logs
		var buildErr error
	buildLoop:
		for {
			select {
			case <-ctx.Done():
				return
			case log, ok := <-buildResult.LogChan:
				if !ok {
					break buildLoop
				}
				logChan <- log
			case err := <-buildResult.ErrChan:
				if err != nil {
					buildErr = err
				}
			}
		}

		// Wait for build done
		<-buildResult.DoneChan

		if buildErr != nil {
			logChan <- OperationLog{
				Timestamp: time.Now(),
				Stream:    "stderr",
				Content:   fmt.Sprintf("--- Build failed: %v ---", buildErr),
			}
			errChan <- buildErr
			return
		}

		logChan <- OperationLog{
			Timestamp: time.Now(),
			Stream:    "system",
			Content:   "--- Build complete, starting containers ---",
		}

		// Phase 2: Up all services
		upArgs := append(baseArgs, "up", "-d")
		upArgs = append(upArgs, services...)
		upCmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, upArgs...)...)
		if workingDir != "" {
			upCmd.Dir = workingDir
		}

		upResult := runStreamingCommand(ctx, upCmd)

		// Forward up logs
		var upErr error
	upLoop:
		for {
			select {
			case <-ctx.Done():
				return
			case log, ok := <-upResult.LogChan:
				if !ok {
					break upLoop
				}
				logChan <- log
			case err := <-upResult.ErrChan:
				if err != nil {
					upErr = err
				}
			}
		}

		// Wait for up done
		<-upResult.DoneChan

		if upErr != nil {
			logChan <- OperationLog{
				Timestamp: time.Now(),
				Stream:    "stderr",
				Content:   fmt.Sprintf("--- Up failed: %v ---", upErr),
			}
			errChan <- upErr
			return
		}

		logChan <- OperationLog{
			Timestamp: time.Now(),
			Stream:    "system",
			Content:   fmt.Sprintf("--- %d containers started successfully ---", len(services)),
		}
	}()

	return StreamingResult{
		LogChan:  logChan,
		ErrChan:  errChan,
		DoneChan: doneChan,
	}
}

// ComposeBuildUpStream runs docker compose build --no-cache then up -d with streaming output
func (c *Client) ComposeBuildUpStream(ctx context.Context, cont Container) StreamingResult {
	if cont.ComposeProject == "" || cont.ComposeService == "" {
		errChan := make(chan error, 1)
		logChan := make(chan OperationLog)
		doneChan := make(chan struct{})
		errChan <- fmt.Errorf("container is not part of a compose project")
		close(errChan)
		close(logChan)
		close(doneChan)
		return StreamingResult{LogChan: logChan, ErrChan: errChan, DoneChan: doneChan}
	}

	logChan := make(chan OperationLog, 100)
	errChan := make(chan error, 1)
	doneChan := make(chan struct{})

	go func() {
		defer close(logChan)
		defer close(errChan)
		defer close(doneChan)

		baseArgs, workingDir := getComposeBaseArgs(cont)

		// Phase 1: Build
		logChan <- OperationLog{
			Timestamp: time.Now(),
			Stream:    "system",
			Content:   "--- Starting build (no-cache) ---",
		}

		buildArgs := append(baseArgs, "build", "--no-cache", "--progress=plain", cont.ComposeService)
		buildCmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, buildArgs...)...)
		if workingDir != "" {
			buildCmd.Dir = workingDir
		}

		buildResult := runStreamingCommand(ctx, buildCmd)

		// Forward build logs
		var buildErr error
	buildLoop:
		for {
			select {
			case <-ctx.Done():
				return
			case log, ok := <-buildResult.LogChan:
				if !ok {
					break buildLoop
				}
				logChan <- log
			case err := <-buildResult.ErrChan:
				if err != nil {
					buildErr = err
				}
			}
		}

		// Wait for build done
		<-buildResult.DoneChan

		if buildErr != nil {
			logChan <- OperationLog{
				Timestamp: time.Now(),
				Stream:    "stderr",
				Content:   fmt.Sprintf("--- Build failed: %v ---", buildErr),
			}
			errChan <- buildErr
			return
		}

		logChan <- OperationLog{
			Timestamp: time.Now(),
			Stream:    "system",
			Content:   "--- Build complete, starting container ---",
		}

		// Phase 2: Up
		upArgs := append(baseArgs, "up", "-d", cont.ComposeService)
		upCmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, upArgs...)...)
		if workingDir != "" {
			upCmd.Dir = workingDir
		}

		upResult := runStreamingCommand(ctx, upCmd)

		// Forward up logs
		var upErr error
	upLoop:
		for {
			select {
			case <-ctx.Done():
				return
			case log, ok := <-upResult.LogChan:
				if !ok {
					break upLoop
				}
				logChan <- log
			case err := <-upResult.ErrChan:
				if err != nil {
					upErr = err
				}
			}
		}

		// Wait for up done
		<-upResult.DoneChan

		if upErr != nil {
			logChan <- OperationLog{
				Timestamp: time.Now(),
				Stream:    "stderr",
				Content:   fmt.Sprintf("--- Up failed: %v ---", upErr),
			}
			errChan <- upErr
			return
		}

		logChan <- OperationLog{
			Timestamp: time.Now(),
			Stream:    "system",
			Content:   "--- Container started successfully ---",
		}
	}()

	return StreamingResult{
		LogChan:  logChan,
		ErrChan:  errChan,
		DoneChan: doneChan,
	}
}
