package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"cm/internal/config"
	"cm/internal/docker"
	"cm/internal/notify"
	"cm/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

// Version information set by ldflags
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func printHelp() {
	help := `
    ██████╗███╗   ███╗
   ██╔════╝████╗ ████║
   ██║     ██╔████╔██║
   ██║     ██║╚██╔╝██║
   ╚██████╗██║ ╚═╝ ██║
    ╚═════╝╚═╝     ╚═╝

  Container Monitor - docker logs, beautifully

USAGE
  cm [OPTIONS] [CONTAINER...]

ARGUMENTS
  CONTAINER    Container name(s) to stream logs from directly
               (partial matches supported)

OPTIONS
  -h, --help      Show this help message
  -v, --version   Show version information

EXAMPLES
  cm              Start interactive container selector
  cm api          Stream logs from container matching "api"
  cm api db       Stream logs from multiple containers

KEYBINDINGS
  j/k, ↑/↓        Navigate up/down
  space           Select/deselect container
  a/A             Select all / Clear selection
  enter           Confirm and view logs
  u/s/r           Start/stop/restart container
  b               Build and restart (compose)
  p               Manage saved projects
  c               Open configuration
  q, ctrl+c       Quit

CONFIG FILES
  ~/.cm/config.json        General settings
  ~/.cm/keybindings.json   Key bindings
  ~/.cm/projects.json      Saved compose projects
`
	fmt.Println(help)
}

func main() {
	// Handle help flag
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		printHelp()
		os.Exit(0)
	}

	// Handle version flag
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("cm %s (commit: %s, built: %s)\n", Version, Commit, BuildTime)
		os.Exit(0)
	}

	// Ensure config file exists with defaults
	_ = config.EnsureDefaults()

	// Initialize notification system
	notify.Initialize()
	defer notify.Close()

	// Create Docker client
	dockerClient, err := docker.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Make sure Docker is running and accessible.\n")
		os.Exit(1)
	}
	defer func() { _ = dockerClient.Close() }()

	// Check for container name arguments
	var initialContainers []docker.Container
	if len(os.Args) > 1 {
		containerNames := os.Args[1:]
		initialContainers = findContainersByName(dockerClient, containerNames)
		if len(initialContainers) == 0 {
			fmt.Fprintf(os.Stderr, "No matching containers found for: %s\n", strings.Join(containerNames, ", "))
			os.Exit(1)
		}
	}

	// Create and run the application
	app := ui.NewApp(dockerClient, initialContainers)
	p := tea.NewProgram(
		app,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(), // Use cell motion instead of all motion for better terminal compatibility
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running application: %v\n", err)
		os.Exit(1)
	}
}

// findContainersByName finds containers matching the given names
func findContainersByName(client *docker.Client, names []string) []docker.Container {
	containers, err := client.ListContainers(context.Background())
	if err != nil {
		return nil
	}

	var matched []docker.Container
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		for _, c := range containers {
			// Match against service name or container name
			if strings.ToLower(c.ComposeService) == name ||
				strings.ToLower(c.Name) == name ||
				strings.Contains(strings.ToLower(c.Name), name) ||
				strings.Contains(strings.ToLower(c.ComposeService), name) {
				// Avoid duplicates
				found := false
				for _, m := range matched {
					if m.ID == c.ID {
						found = true
						break
					}
				}
				if !found {
					matched = append(matched, c)
				}
			}
		}
	}

	return matched
}
