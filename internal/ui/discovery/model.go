package discovery

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"cm/internal/debug"
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

type bulkActionCompleteMsg struct {
	action    string
	succeeded int
	failed    int
	errors    []string
}

type actionStartedMsg struct {
	action string
}

// Model represents the container discovery screen
type Model struct {
	groups             []docker.ContainerGroup
	flatList           []listItem
	cursor             int
	selected           map[string]bool
	width, height      int
	ready              bool
	err                error
	keys               common.KeyMap
	dockerClient       *docker.Client
	actionStatus       string
	actionRunning      bool
	configModal        common.ConfigModal
	savedProjectsModal common.SavedProjectsModal
	toast              common.Toast
	tutorial           common.Tutorial
}

type listItem struct {
	isGroup     bool
	isSeparator bool
	groupName   string
	container   docker.Container
}

// selectionKey returns a stable key for selecting a container
// Uses compose project:service for compose containers, or ID for standalone
func selectionKey(c docker.Container) string {
	if c.ComposeProject != "" && c.ComposeService != "" {
		return c.ComposeProject + ":" + c.ComposeService
	}
	return c.ID
}

// New creates a new discovery model
func New(dockerClient *docker.Client, initialSelection []docker.Container) Model {
	selected := make(map[string]bool)
	for _, c := range initialSelection {
		selected[selectionKey(c)] = true
	}
	return Model{
		selected:           selected,
		keys:               common.DefaultKeyMap(),
		dockerClient:       dockerClient,
		configModal:        common.NewConfigModal(),
		savedProjectsModal: common.NewSavedProjectsModal(),
		toast:              common.NewToast(),
		tutorial:           common.NewTutorial(),
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
		localProject := docker.DetectLocalComposeProject()
		groups := docker.GroupByComposeProject(containers, localProject)
		return ContainersLoadedMsg{Groups: groups}
	}
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// Handle saved projects modal messages first
	if m.savedProjectsModal.IsVisible() {
		var cmd tea.Cmd
		m.savedProjectsModal, cmd = m.savedProjectsModal.Update(msg)
		return m, cmd
	}

	// Handle config modal messages
	if m.configModal.IsVisible() {
		var cmd tea.Cmd
		m.configModal, cmd = m.configModal.Update(msg)
		return m, cmd
	}

	// Handle modal closed messages
	if closed, ok := msg.(common.ConfigModalClosedMsg); ok {
		// Reload key bindings and toast settings in case they changed
		m.keys = common.DefaultKeyMap()
		if closed.ConfigChanged {
			m.toast.ReloadConfig()
		}
		return m, nil
	}

	// Handle saved projects modal closed message
	if _, ok := msg.(common.SavedProjectsClosedMsg); ok {
		return m, nil
	}

	// Handle open saved projects modal message
	if _, ok := msg.(common.OpenSavedProjectsMsg); ok {
		return m, m.savedProjectsModal.Open()
	}

	// Handle toast messages
	if _, ok := msg.(common.ShowToastMsg); ok {
		var cmd tea.Cmd
		m.toast, cmd = m.toast.Update(msg)
		return m, cmd
	}
	if _, ok := msg.(common.ToastExpiredMsg); ok {
		m.toast, _ = m.toast.Update(msg)
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.configModal.SetSize(msg.Width, msg.Height)
		m.savedProjectsModal.SetSize(msg.Width, msg.Height)

	case ContainersLoadedMsg:
		m.groups = msg.Groups
		m.flatList = m.buildFlatList()
		m.ready = true
		if m.cursor == 0 || m.cursor >= len(m.flatList) {
			for i, item := range m.flatList {
				if !item.isGroup && !item.isSeparator {
					m.cursor = i
					break
				}
			}
		}
		// Start tutorial if there are containers
		m.tutorial.StartIfReady(len(m.flatList) > 0)
		return m, tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
			return autoRefreshTickMsg{}
		})

	case autoRefreshTickMsg:
		if !m.actionRunning {
			return m, m.loadContainers()
		}

	case actionStartedMsg:
		m.actionStatus = msg.action
		m.actionRunning = true

	case bulkActionCompleteMsg:
		m.actionRunning = false
		var toastCmd tea.Cmd
		if msg.failed == 0 {
			m.actionStatus = fmt.Sprintf("%s completed (%d succeeded)", msg.action, msg.succeeded)
			toastCmd = m.toast.Show(capitalize(msg.action), fmt.Sprintf("%d containers", msg.succeeded), common.ToastSuccess)
		} else {
			m.actionStatus = fmt.Sprintf("%s: %d succeeded, %d failed", msg.action, msg.succeeded, msg.failed)
			toastCmd = m.toast.Show(capitalize(msg.action), fmt.Sprintf("%d failed", msg.failed), common.ToastError)
		}
		return m, tea.Batch(toastCmd, m.loadContainers())

	case LoadErrorMsg:
		m.err = msg.Err
		m.ready = true

	case tea.KeyMsg:
		if m.actionRunning {
			// Only allow quit during action
			if key.Matches(msg, m.keys.Quit) {
				return m, tea.Quit
			}
			return m, nil
		}

		// Handle tutorial intro modal - only allow enter to start or 's' to skip
		if m.tutorial.IsIntroStep() {
			if key.Matches(msg, m.keys.Confirm) {
				m.tutorial.Advance()
				return m, nil
			}
			if msg.String() == "s" {
				m.tutorial.Skip()
				return m, nil
			}
			if key.Matches(msg, m.keys.Quit) {
				return m, tea.Quit
			}
			// Block all other keys during intro modal
			return m, nil
		}

		// Handle tutorial skip with 's' when tutorial is active
		if m.tutorial.Active && msg.String() == "s" {
			m.tutorial.Skip()
			return m, nil
		}

		switch {
		case key.Matches(msg, m.keys.Up):
			m.moveCursor(-1)
			// Advance tutorial if on navigate step
			if m.tutorial.Active && m.tutorial.Step == common.TutorialStepNavigate {
				m.tutorial.Advance()
			}

		case key.Matches(msg, m.keys.Down):
			m.moveCursor(1)
			// Advance tutorial if on navigate step
			if m.tutorial.Active && m.tutorial.Step == common.TutorialStepNavigate {
				m.tutorial.Advance()
			}

		case key.Matches(msg, m.keys.Top):
			m.goToTop()

		case key.Matches(msg, m.keys.Bottom):
			m.goToBottom()

		case key.Matches(msg, m.keys.Select):
			m.toggleSelect()
			// Advance tutorial if on select step and enough containers selected
			if m.tutorial.Active && m.tutorial.Step == common.TutorialStepSelect {
				availableCount := m.countAvailableContainers()
				if m.tutorial.ShouldAdvanceFromSelect(len(m.selected), availableCount) {
					m.tutorial.Advance()
				}
			}

		case key.Matches(msg, m.keys.SelectAll):
			m.selectAll()

		case key.Matches(msg, m.keys.ClearAll):
			m.clearSelection()

		case key.Matches(msg, m.keys.Confirm):
			if len(m.selected) > 0 {
				// Advance tutorial to logview steps when confirming
				if m.tutorial.Active && m.tutorial.Step == common.TutorialStepConfirm {
					m.tutorial.Advance()
				}
				return m, m.confirmSelection()
			}

		case key.Matches(msg, m.keys.Refresh):
			m.ready = false
			m.actionStatus = ""
			return m, m.loadContainers()

		// Single container actions (on cursor)
		case key.Matches(msg, m.keys.Start):
			return m, m.doAction("start", m.getActionTargets(), m.dockerClient.ComposeUp)

		case key.Matches(msg, m.keys.Stop):
			return m, m.doAction("stop", m.getActionTargets(), m.dockerClient.ComposeDown)

		case key.Matches(msg, m.keys.Restart):
			return m, m.doAction("restart", m.getActionTargets(), m.dockerClient.ComposeDownUp)

		case key.Matches(msg, m.keys.ComposeBuild):
			return m, m.doAction("build", m.getActionTargets(), m.dockerClient.ComposeBuildUp)

		case key.Matches(msg, m.keys.Config):
			return m, m.configModal.Open()

		case key.Matches(msg, m.keys.SavedProjects):
			return m, m.savedProjectsModal.Open()

		case key.Matches(msg, m.keys.DebugToggle):
			enabled := debug.Toggle()
			status := "off"
			if enabled {
				status = "on"
			}
			return m, m.toast.Show("Debug Log", status, common.ToastSuccess)

		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		}
	}

	return m, nil
}

// getActionTargets returns selected containers, or the focused container if none selected
func (m *Model) getActionTargets() []docker.Container {
	var targets []docker.Container

	// If containers are selected, use those
	if len(m.selected) > 0 {
		for _, item := range m.flatList {
			if !item.isGroup && !item.isSeparator && m.selected[selectionKey(item.container)] {
				if item.container.ComposeService != "" {
					targets = append(targets, item.container)
				}
			}
		}
	} else {
		// Otherwise use the focused container
		if m.cursor >= 0 && m.cursor < len(m.flatList) {
			item := m.flatList[m.cursor]
			if !item.isGroup && !item.isSeparator && item.container.ComposeService != "" {
				targets = append(targets, item.container)
			}
		}
	}

	return targets
}

type composeAction func(context.Context, docker.Container) error

// capitalize returns a string with the first letter capitalized
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (m Model) doAction(name string, targets []docker.Container, action composeAction) tea.Cmd {
	if len(targets) == 0 {
		return nil
	}

	return tea.Batch(
		func() tea.Msg {
			if len(targets) == 1 {
				return actionStartedMsg{action: fmt.Sprintf("%s %s...", capitalize(name), targets[0].ComposeService)}
			}
			return actionStartedMsg{action: fmt.Sprintf("%s %d containers...", capitalize(name), len(targets))}
		},
		func() tea.Msg {
			var succeeded, failed int
			var errors []string
			var wg sync.WaitGroup
			var mu sync.Mutex

			// Run actions concurrently with a semaphore
			sem := make(chan struct{}, 3) // Max 3 concurrent actions

			for _, cont := range targets {
				wg.Add(1)
				go func(c docker.Container) {
					defer wg.Done()
					defer func() {
						if r := recover(); r != nil {
							mu.Lock()
							failed++
							errors = append(errors, fmt.Sprintf("%s: panic: %v", c.ComposeService, r))
							mu.Unlock()
						}
					}()
					sem <- struct{}{}
					defer func() { <-sem }()

					err := action(context.Background(), c)
					mu.Lock()
					if err != nil {
						failed++
						errors = append(errors, fmt.Sprintf("%s: %v", c.ComposeService, err))
					} else {
						succeeded++
					}
					mu.Unlock()
				}(cont)
			}

			wg.Wait()
			return bulkActionCompleteMsg{
				action:    name,
				succeeded: succeeded,
				failed:    failed,
				errors:    errors,
			}
		},
	)
}

func (m *Model) toggleSelect() {
	if m.cursor >= 0 && m.cursor < len(m.flatList) {
		item := m.flatList[m.cursor]
		if !item.isGroup && !item.isSeparator {
			key := selectionKey(item.container)
			if m.selected[key] {
				delete(m.selected, key)
			} else {
				m.selected[key] = true
			}
		}
	}
}

func (m *Model) selectAll() {
	for _, item := range m.flatList {
		if !item.isGroup && !item.isSeparator {
			m.selected[selectionKey(item.container)] = true
		}
	}
}

func (m *Model) clearSelection() {
	m.selected = make(map[string]bool)
}

func (m *Model) countAvailableContainers() int {
	count := 0
	for _, item := range m.flatList {
		if !item.isGroup && !item.isSeparator {
			count++
		}
	}
	return count
}

func (m *Model) goToTop() {
	for i, item := range m.flatList {
		if !item.isGroup && !item.isSeparator {
			m.cursor = i
			return
		}
	}
}

func (m *Model) goToBottom() {
	for i := len(m.flatList) - 1; i >= 0; i-- {
		item := m.flatList[i]
		if !item.isGroup && !item.isSeparator {
			m.cursor = i
			return
		}
	}
}

func (m *Model) moveCursor(delta int) {
	if len(m.flatList) == 0 {
		return
	}

	newCursor := m.cursor + delta
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

		for _, c := range group.Containers {
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
			if !item.isGroup && m.selected[selectionKey(item.container)] {
				// Skip stopped containers - they don't exist yet and have no logs
				if item.container.State == "stopped" {
					continue
				}
				containers = append(containers, item.container)
			}
		}
		return ContainerSelectedMsg{Containers: containers}
	}
}

// View renders the model
func (m Model) View() string {
	if !m.ready {
		return "\n  Loading containers..."
	}

	if m.err != nil {
		return fmt.Sprintf("\n  Error: %v\n\n  Press ctrl+r to retry or 'q' to quit.", m.err)
	}

	if len(m.flatList) == 0 {
		logo := `
    ██████╗███╗   ███╗
   ██╔════╝████╗ ████║
   ██║     ██╔████╔██║
   ██║     ██║╚██╔╝██║
   ╚██████╗██║ ╚═╝ ██║
    ╚═════╝╚═╝     ╚═╝ `
		return common.TitleStyle.Render(logo) + "\n" +
			common.SubtitleStyle.Render("   docker logs, beautifully") + "\n\n" +
			common.EmptyStateStyle.Render("  No running containers found.\n\n  Press ctrl+r to refresh, 'p' for projects, 'q' to quit")
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
	b.WriteString(common.SubtitleStyle.Render("   docker logs, beautifully"))
	b.WriteString("\n\n")

	// List
	for i, item := range m.flatList {
		if item.isGroup {
			b.WriteString(common.GroupHeaderStyle.Render(fmt.Sprintf("  %s", item.groupName)))
			b.WriteString("\n")
			continue
		}


		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		checkbox := "[ ]"
		if m.selected[selectionKey(item.container)] {
			checkbox = common.CheckedStyle.Render("[x]")
		}

		name := item.container.DisplayName()
		isRunning := item.container.State == "running"
		isStopped := item.container.State == "stopped"

		status := common.StoppedStyle.Render("○")
		if isRunning {
			status = common.RunningStyle.Render("●")
		} else if isStopped {
			status = common.MutedInlineStyle.Render("◌")
		}

		line := fmt.Sprintf("%s%s %s %s", cursor, checkbox, status, name)
		if i == m.cursor {
			line = common.SelectedItemStyle.Render(line)
		}

		b.WriteString("  ")
		b.WriteString(line)

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
		style := common.MutedInlineStyle
		if strings.Contains(m.actionStatus, "failed") {
			style = common.StderrStyle
		} else if strings.Contains(m.actionStatus, "completed") {
			style = common.RunningStyle
		}
		b.WriteString(style.Render(fmt.Sprintf("  %s", m.actionStatus)))
	}

	// Build main content area (everything except help bar)
	mainContent := b.String()

	// Create help bar
	helpBar := m.renderHelpBar()

	// Create toast line (empty if not visible)
	var toastLine string
	if m.toast.IsVisible() {
		toastLine = m.renderInlineToast()
	}

	// Combine: main content at top, help bar at bottom, toast above help bar if visible
	width := m.width
	height := m.height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	// Create tutorial hint bar (if tutorial is active)
	var tutorialBar string
	if m.tutorial.Active && m.tutorial.IsDiscoveryStep() {
		tutorialBar = m.tutorial.View(width)
	}

	// Calculate how many lines we need for the bottom section
	bottomSection := helpBar
	if tutorialBar != "" {
		bottomSection = tutorialBar + "\n" + helpBar
	}
	if toastLine != "" {
		bottomSection = toastLine + "\n" + bottomSection
	}

	// Use Place to position content at top, leaving room for bottom section
	bottomHeight := lipgloss.Height(bottomSection)
	topHeight := height - bottomHeight
	if topHeight < 1 {
		topHeight = 1
	}

	topContent := lipgloss.Place(width, topHeight,
		lipgloss.Left, lipgloss.Top,
		mainContent,
		lipgloss.WithWhitespaceChars(" "),
	)

	content := topContent + "\n" + bottomSection

	// Overlay saved projects modal if visible
	if m.savedProjectsModal.IsVisible() {
		modalView := m.savedProjectsModal.View(width, height)
		base := lipgloss.Place(width, height,
			lipgloss.Left, lipgloss.Top,
			content,
			lipgloss.WithWhitespaceChars(" "),
		)
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
			base+"\n"+modalView)
	}

	// Overlay config modal if visible
	if m.configModal.IsVisible() {
		modalView := m.configModal.View(width, height)
		base := lipgloss.Place(width, height,
			lipgloss.Left, lipgloss.Top,
			content,
			lipgloss.WithWhitespaceChars(" "),
		)
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
			base+"\n"+modalView)
	}

	// Overlay tutorial intro modal if at intro step
	if m.tutorial.IsIntroStep() {
		return m.tutorial.ViewIntroModal(width, height)
	}

	return content
}

func (m Model) renderHelpBar() string {
	k := common.HelpKeyStyle.Render
	d := common.HelpDescStyle.Render

	selectedCount := len(m.selected)
	var selectedText string
	if selectedCount > 0 {
		selectedText = d(fmt.Sprintf(" %d selected  ", selectedCount))
	} else {
		selectedText = " "
	}

	help := selectedText +
		k("spc") + d(":sel ") +
		k("a") + d("/") + k("A") + d(":all/clr ") +
		k("⏎") + d(":logs ") +
		k("u") + d("/") + k("s") + d("/") + k("r") + d(":up/stop/restart ") +
		k("b") + d(":build ") +
		k("p") + d(":projects ") +
		k("c") + d(":config ") +
		k("ctrl+g") + d(":debug logs ") +
		k("q") + d(":quit")

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
		if !item.isGroup && m.selected[selectionKey(item.container)] {
			containers = append(containers, item.container)
		}
	}
	return containers
}

// renderInlineToast renders the toast as an inline notification bar
func (m Model) renderInlineToast() string {
	// Get toast content from the Toast component
	toastContent := m.toast.RenderInline()

	width := m.width
	if width <= 0 {
		width = 80
	}

	// Right-align the toast for bottom-right appearance
	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Right).
		Render(toastContent)
}

// GetTutorial returns the current tutorial state
func (m Model) GetTutorial() common.Tutorial {
	return m.tutorial
}

