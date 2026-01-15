package discovery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cm/internal/docker"
	"cm/internal/ui/common"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Messages
type ContainersLoadedMsg struct {
	Groups []docker.ContainerGroup
}

type ContainerSelectedMsg struct {
	Containers []docker.Container
}

type LoadErrorMsg struct {
	Err error
}

type autoRefreshTickMsg struct{}

type actionCompleteMsg struct {
	action string
	err    error
}

type actionStartedMsg struct {
	action string
}

// Model represents the container discovery screen
type Model struct {
	groups        []docker.ContainerGroup
	flatList      []listItem
	cursor        int
	selected      map[string]bool
	width, height int
	ready         bool
	err           error
	keys          common.KeyMap
	dockerClient  *docker.Client
	actionStatus  string // Current action status message
}

type listItem struct {
	isGroup     bool
	isSeparator bool
	groupName   string
	container   docker.Container
}

// New creates a new discovery model
func New(dockerClient *docker.Client, initialSelection []docker.Container) Model {
	selected := make(map[string]bool)
	for _, c := range initialSelection {
		selected[c.ID] = true
	}
	return Model{
		selected:     selected,
		keys:         common.DefaultKeyMap(),
		dockerClient: dockerClient,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return m.loadContainers()
}

func (m Model) loadContainers() tea.Cmd {
	return func() tea.Msg {
		containers, err := m.dockerClient.ListContainers(context.Background())
		if err != nil {
			return LoadErrorMsg{Err: err}
		}
		// Detect if we're in a directory with a compose file
		localProject := docker.DetectLocalComposeProject()
		groups := docker.GroupByComposeProject(containers, localProject)
		return ContainersLoadedMsg{Groups: groups}
	}
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case ContainersLoadedMsg:
		m.groups = msg.Groups
		m.flatList = m.buildFlatList()
		m.ready = true
		// Move cursor to first container (skip group headers)
		if m.cursor == 0 || m.cursor >= len(m.flatList) {
			for i, item := range m.flatList {
				if !item.isGroup {
					m.cursor = i
					break
				}
			}
		}
		// Schedule next refresh (5 seconds to reduce resource usage)
		return m, tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
			return autoRefreshTickMsg{}
		})

	case autoRefreshTickMsg:
		return m, m.loadContainers()

	case actionStartedMsg:
		m.actionStatus = msg.action
		return m, nil

	case actionCompleteMsg:
		m.actionStatus = ""
		if msg.err != nil {
			m.actionStatus = fmt.Sprintf("Error: %v", msg.err)
		}
		// Refresh containers after action
		return m, m.loadContainers()

	case LoadErrorMsg:
		m.err = msg.Err
		m.ready = true

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Up):
			m.moveCursor(-1)

		case key.Matches(msg, m.keys.Down):
			m.moveCursor(1)

		case key.Matches(msg, m.keys.Select):
			if m.cursor >= 0 && m.cursor < len(m.flatList) {
				item := m.flatList[m.cursor]
				if !item.isGroup {
					if m.selected[item.container.ID] {
						delete(m.selected, item.container.ID)
					} else {
						m.selected[item.container.ID] = true
					}
				}
			}

		case key.Matches(msg, m.keys.Confirm):
			if len(m.selected) > 0 {
				return m, m.confirmSelection()
			}

		case key.Matches(msg, m.keys.Refresh):
			m.ready = false
			return m, m.loadContainers()

		case key.Matches(msg, m.keys.Start):
			// Start the currently focused container
			if m.cursor >= 0 && m.cursor < len(m.flatList) {
				item := m.flatList[m.cursor]
				if !item.isGroup && !item.isSeparator && item.container.ComposeService != "" {
					return m, m.startContainer(item.container)
				}
			}

		case key.Matches(msg, m.keys.ComposeRestart):
			// Restart (down/up) the currently focused container
			if m.cursor >= 0 && m.cursor < len(m.flatList) {
				item := m.flatList[m.cursor]
				if !item.isGroup && !item.isSeparator && item.container.ComposeService != "" {
					return m, m.restartContainer(item.container)
				}
			}

		case key.Matches(msg, m.keys.ComposeBuild):
			// Build and start the currently focused container
			if m.cursor >= 0 && m.cursor < len(m.flatList) {
				item := m.flatList[m.cursor]
				if !item.isGroup && !item.isSeparator && item.container.ComposeService != "" {
					return m, m.buildContainer(item.container)
				}
			}

		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m *Model) moveCursor(delta int) {
	if len(m.flatList) == 0 {
		return
	}

	newCursor := m.cursor + delta
	// Skip group headers and separators
	for newCursor >= 0 && newCursor < len(m.flatList) {
		item := m.flatList[newCursor]
		if !item.isGroup && !item.isSeparator {
			break
		}
		newCursor += delta
	}

	if newCursor >= 0 && newCursor < len(m.flatList) {
		item := m.flatList[newCursor]
		if !item.isGroup && !item.isSeparator {
			m.cursor = newCursor
		}
	}
}

func (m Model) buildFlatList() []listItem {
	var items []listItem
	for _, group := range m.groups {
		items = append(items, listItem{
			isGroup:   true,
			groupName: group.ProjectName,
		})

		// Separate main services from infrastructure services
		var mainServices, infraServices []docker.Container
		for _, c := range group.Containers {
			if group.InfrastructureServices[c.ComposeService] {
				infraServices = append(infraServices, c)
			} else {
				mainServices = append(mainServices, c)
			}
		}

		// Add main services first
		for _, c := range mainServices {
			items = append(items, listItem{
				isGroup:   false,
				container: c,
			})
		}

		// Add separator and infrastructure services if any
		if len(infraServices) > 0 && len(mainServices) > 0 {
			items = append(items, listItem{
				isSeparator: true,
			})
		}
		for _, c := range infraServices {
			items = append(items, listItem{
				isGroup:   false,
				container: c,
			})
		}
	}
	return items
}

func (m Model) confirmSelection() tea.Cmd {
	return func() tea.Msg {
		var containers []docker.Container
		for _, item := range m.flatList {
			if !item.isGroup && m.selected[item.container.ID] {
				containers = append(containers, item.container)
			}
		}
		return ContainerSelectedMsg{Containers: containers}
	}
}

func (m Model) startContainer(cont docker.Container) tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			return actionStartedMsg{action: fmt.Sprintf("Starting %s...", cont.ComposeService)}
		},
		func() tea.Msg {
			err := m.dockerClient.ComposeUp(context.Background(), cont)
			return actionCompleteMsg{action: "start", err: err}
		},
	)
}

func (m Model) restartContainer(cont docker.Container) tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			return actionStartedMsg{action: fmt.Sprintf("Restarting %s...", cont.ComposeService)}
		},
		func() tea.Msg {
			err := m.dockerClient.ComposeDownUp(context.Background(), cont)
			return actionCompleteMsg{action: "restart", err: err}
		},
	)
}

func (m Model) buildContainer(cont docker.Container) tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			return actionStartedMsg{action: fmt.Sprintf("Building %s...", cont.ComposeService)}
		},
		func() tea.Msg {
			err := m.dockerClient.ComposeBuildUp(context.Background(), cont)
			return actionCompleteMsg{action: "build", err: err}
		},
	)
}

// View renders the model
func (m Model) View() string {
	if !m.ready {
		return "\n  Loading containers..."
	}

	if m.err != nil {
		return fmt.Sprintf("\n  Error: %v\n\n  Press 'r' to retry or 'q' to quit.", m.err)
	}

	if len(m.flatList) == 0 {
		return common.EmptyStateStyle.Render("\n\n  No running containers found.\n\n  Press 'r' to refresh or 'q' to quit.")
	}

	var b strings.Builder

	// Logo
	logo := `
    ██████╗███╗   ███╗
   ██╔════╝████╗ ████║
   ██║     ██╔████╔██║
   ██║     ██║╚██╔╝██║
   ╚██████╗██║ ╚═╝ ██║
    ╚═════╝╚═╝     ╚═╝ `
	b.WriteString(common.TitleStyle.Render(logo))
	b.WriteString("\n")
	b.WriteString(common.SubtitleStyle.Render("   container monitor ~ stream them logs"))
	b.WriteString("\n\n")

	// List
	for i, item := range m.flatList {
		if item.isGroup {
			b.WriteString(common.GroupHeaderStyle.Render(fmt.Sprintf("  %s", item.groupName)))
			b.WriteString("\n")
			continue
		}

		if item.isSeparator {
			b.WriteString(common.MutedInlineStyle.Render("    ─── dependencies (auto-started) ───"))
			b.WriteString("\n")
			continue
		}

		// Cursor
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		// Checkbox
		checkbox := "[ ]"
		if m.selected[item.container.ID] {
			checkbox = common.CheckedStyle.Render("[x]")
		}

		// Container info
		name := item.container.DisplayName()
		isRunning := item.container.State == "running"
		isStopped := item.container.State == "stopped" // Not started from compose

		status := common.StoppedStyle.Render("○") // exited
		if isRunning {
			status = common.RunningStyle.Render("●")
		} else if isStopped {
			status = common.MutedInlineStyle.Render("◌") // not started
		}

		line := fmt.Sprintf("%s%s %s %s", cursor, checkbox, status, name)
		if i == m.cursor {
			line = common.SelectedItemStyle.Render(line)
		}

		b.WriteString("  ")
		b.WriteString(line)

		// Show container ID and status for context
		if i == m.cursor {
			if isStopped {
				b.WriteString(common.MutedInlineStyle.Render(" (not started)"))
			} else {
				info := item.container.ID
				if !isRunning {
					info = fmt.Sprintf("%s - %s", item.container.ID, item.container.Status)
				}
				b.WriteString(common.MutedInlineStyle.Render(fmt.Sprintf(" (%s)", info)))
			}
		}
		b.WriteString("\n")
	}

	// Action status
	if m.actionStatus != "" {
		b.WriteString("\n")
		b.WriteString(common.MutedInlineStyle.Render(fmt.Sprintf("  %s", m.actionStatus)))
	}

	// Help bar (unified style)
	b.WriteString("\n")
	helpBar := m.renderHelpBar()
	b.WriteString(helpBar)

	width := m.width
	if width <= 0 {
		width = 80
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(b.String())
}

func (m Model) renderHelpBar() string {
	key := common.HelpKeyStyle.Render
	desc := common.HelpDescStyle.Render

	selectedCount := len(m.selected)
	var selectedText string
	if selectedCount > 0 {
		selectedText = desc(fmt.Sprintf(" %d selected  ", selectedCount))
	} else {
		selectedText = " "
	}

	help := selectedText +
		key("space") + desc(":select") +
		desc("  ") + key("enter") + desc(":logs") +
		desc("  ") + key("s") + desc(":start") +
		desc("  ") + key("R") + desc(":restart") +
		desc("  ") + key("B") + desc(":build") +
		desc("  ") + key("r") + desc(":refresh") +
		desc("  ") + key("q") + desc(":quit")

	width := m.width
	if width <= 0 {
		width = 80
	}
	return common.HelpBarStyle.Width(width).Render(help)
}

// SelectedContainers returns the currently selected containers
func (m Model) SelectedContainers() []docker.Container {
	var containers []docker.Container
	for _, item := range m.flatList {
		if !item.isGroup && m.selected[item.container.ID] {
			containers = append(containers, item.container)
		}
	}
	return containers
}
