package ui

import (
	"cm/internal/docker"
	"cm/internal/ui/discovery"
	"cm/internal/ui/logview"

	tea "github.com/charmbracelet/bubbletea"
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
func (a App) Init() tea.Cmd {
	cmds := []tea.Cmd{
		tea.EnterAltScreen,
		tea.EnableMouseAllMotion,
	}

	if a.startWithLogView {
		// Initialize log view directly
		a.logview = logview.New(a.selectedConts, a.dockerClient, a.width, a.height)
		cmds = append(cmds, a.logview.Init())
	} else {
		cmds = append(cmds, a.discovery.Init())
	}

	return tea.Batch(cmds...)
}

// Update handles messages
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		// Initialize log view with proper dimensions if starting with it
		if a.startWithLogView && a.screen == ScreenLogView {
			a.logview = logview.New(a.selectedConts, a.dockerClient, a.width, a.height)
			a.startWithLogView = false
			return a, a.logview.Init()
		}

	case discovery.ContainerSelectedMsg:
		// Transition to log view
		a.selectedConts = msg.Containers
		a.logview = logview.New(msg.Containers, a.dockerClient, a.width, a.height)
		a.screen = ScreenLogView
		return a, a.logview.Init()

	case logview.BackToDiscoveryMsg:
		// Go back to discovery, preserving selection
		a.logview.Cleanup()
		a.screen = ScreenDiscovery
		a.discovery = discovery.New(a.dockerClient, a.selectedConts)
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
	switch a.screen {
	case ScreenDiscovery:
		return a.discovery.View()
	case ScreenLogView:
		return a.logview.View()
	default:
		return "Unknown screen"
	}
}
