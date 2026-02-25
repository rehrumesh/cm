package ui

import (
	"cm/internal/docker"
	"cm/internal/ui/common"
	"cm/internal/ui/discovery"
	"cm/internal/ui/logview"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Screen represents the current screen
type Screen int

const (
	ScreenDiscovery Screen = iota
	ScreenLogView
)

// App is the root model that manages screen transitions
type App struct {
	screen           Screen
	discovery        discovery.Model
	logview          logview.Model
	width, height    int
	dockerClient     *docker.Client
	selectedConts    []docker.Container
	startWithLogView bool
}

// NewApp creates a new application model
func NewApp(dockerClient *docker.Client, initialContainers []docker.Container) App {
	if len(initialContainers) > 0 {
		// Start directly in log view mode
		return App{
			screen:           ScreenLogView,
			dockerClient:     dockerClient,
			selectedConts:    initialContainers,
			startWithLogView: true,
		}
	}
	return App{
		screen:       ScreenDiscovery,
		discovery:    discovery.New(dockerClient, nil),
		dockerClient: dockerClient,
	}
}

// Init initializes the application
// Note: AltScreen and Mouse are already enabled via tea.NewProgram options in main.go
func (a App) Init() tea.Cmd {
	if a.startWithLogView {
		// Initialize log view directly (no tutorial when starting directly in logview)
		a.logview = logview.New(a.selectedConts, a.dockerClient, a.width, a.height, common.Tutorial{})
		return a.logview.Init()
	}
	return a.discovery.Init()
}

// Update handles messages
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Always honor terminal interrupt semantics.
		if msg.String() == "ctrl+c" {
			if a.screen == ScreenLogView {
				a.logview.Cleanup()
			}
			return a, tea.Quit
		}

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		// Initialize log view with proper dimensions if starting with it
		if a.startWithLogView && a.screen == ScreenLogView {
			a.logview = logview.New(a.selectedConts, a.dockerClient, a.width, a.height, common.Tutorial{})
			a.startWithLogView = false
			return a, a.logview.Init()
		}

	case discovery.ContainerSelectedMsg:
		// Transition to log view, passing tutorial state from discovery
		a.selectedConts = msg.Containers
		a.logview = logview.New(msg.Containers, a.dockerClient, a.width, a.height, a.discovery.GetTutorial())
		a.screen = ScreenLogView
		return a, a.logview.Init()

	case logview.BackToDiscoveryMsg:
		// Go back to discovery, preserving selection
		a.logview.Cleanup()
		a.screen = ScreenDiscovery
		a.discovery = discovery.New(a.dockerClient, a.selectedConts)
		// Forward current window size to the new discovery model
		a.discovery, _ = a.discovery.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
		return a, a.discovery.Init()
	}

	// Route to current screen
	switch a.screen {
	case ScreenDiscovery:
		var cmd tea.Cmd
		a.discovery, cmd = a.discovery.Update(msg)
		return a, cmd

	case ScreenLogView:
		var cmd tea.Cmd
		a.logview, cmd = a.logview.Update(msg)
		return a, cmd
	}

	return a, nil
}

// View renders the application
func (a App) View() string {
	var content string
	switch a.screen {
	case ScreenDiscovery:
		content = a.discovery.View()
	case ScreenLogView:
		content = a.logview.View()
	default:
		content = "Unknown screen"
	}

	// Always paint a full frame to avoid stale cells/background artifacts.
	if a.width > 0 && a.height > 0 {
		return lipgloss.NewStyle().
			Width(a.width).
			Height(a.height).
			Render(content)
	}

	return content
}
