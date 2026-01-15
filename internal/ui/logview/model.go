package logview

import (
	"context"
	"fmt"
	"time"

	"cm/internal/docker"
	"cm/internal/ui/common"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

const doubleClickThreshold = 400 * time.Millisecond

// Messages
type LogLineMsg struct {
	ContainerID string
	Line        docker.LogLine
}

type LogErrorMsg struct {
	ContainerID string
	Err         error
}

type BackToDiscoveryMsg struct{}

type ContainerActionMsg struct {
	ContainerID string
	Action      string
	Err         error
}

type ContainerActionStartMsg struct {
	ContainerID string
	Action      string
}

// streamInfo holds the channels for a container's log stream
type streamInfo struct {
	logChan <-chan docker.LogLine
	errChan <-chan error
}

// Model represents the log view screen
type Model struct {
	panes         []Pane
	streams       map[string]streamInfo // containerID -> stream channels
	layout        Layout
	focusedPane   int
	maximizedPane int // -1 if none maximized
	width, height int
	keys          common.KeyMap
	dockerClient  *docker.Client
	ctx           context.Context
	cancel        context.CancelFunc
	zone          *zone.Manager

	// For double-click detection
	lastClickTime   time.Time
	lastClickPaneID string
}

// New creates a new log view model
func New(containers []docker.Container, dockerClient *docker.Client, width, height int) Model {
	ctx, cancel := context.WithCancel(context.Background())

	m := Model{
		panes:         make([]Pane, len(containers)),
		streams:       make(map[string]streamInfo),
		focusedPane:   0,
		maximizedPane: -1,
		width:         width,
		height:        height,
		keys:          common.DefaultKeyMap(),
		dockerClient:  dockerClient,
		ctx:           ctx,
		cancel:        cancel,
		zone:          zone.New(),
	}

	// Calculate layout
	m.layout = CalculateLayout(len(containers))

	// Create panes
	paneWidth := m.layout.PaneWidth(width)
	paneHeight := m.layout.PaneHeight(height)

	for i, container := range containers {
		m.panes[i] = NewPane(container, paneWidth, paneHeight)
	}

	if len(m.panes) > 0 {
		m.panes[0].Active = true
	}

	return m
}

// Init initializes the model and starts log streaming
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd

	for _, pane := range m.panes {
		logChan, errChan := m.dockerClient.StreamLogs(m.ctx, pane.Container.ID)
		m.streams[pane.ID] = streamInfo{logChan: logChan, errChan: errChan}
		cmds = append(cmds, m.waitForLog(pane.ID, logChan))
		cmds = append(cmds, m.waitForError(pane.ID, errChan))
	}

	return tea.Batch(cmds...)
}

func (m Model) waitForLog(containerID string, logChan <-chan docker.LogLine) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-logChan
		if !ok {
			return nil
		}
		return LogLineMsg{ContainerID: containerID, Line: line}
	}
}

func (m Model) waitForError(containerID string, errChan <-chan error) tea.Cmd {
	return func() tea.Msg {
		err, ok := <-errChan
		if !ok {
			return nil
		}
		return LogErrorMsg{ContainerID: containerID, Err: err}
	}
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalculateLayout()

	case LogLineMsg:
		for i := range m.panes {
			if m.panes[i].ID == msg.ContainerID {
				m.panes[i].AddLogLine(msg.Line)
				// Continue listening on the SAME channel
				if stream, ok := m.streams[msg.ContainerID]; ok {
					cmds = append(cmds, m.waitForLog(msg.ContainerID, stream.logChan))
				}
				break
			}
		}

	case LogErrorMsg:
		for i := range m.panes {
			if m.panes[i].ID == msg.ContainerID {
				m.panes[i].Connected = false
				m.panes[i].AddLogLine(docker.LogLine{
					ContainerID: msg.ContainerID,
					Timestamp:   time.Now(),
					Stream:      "system",
					Content:     fmt.Sprintf("--- Stream ended: %v ---", msg.Err),
				})
				break
			}
		}

	case tea.MouseMsg:
		cmd := m.handleMouse(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Back):
			m.cancel()
			return m, func() tea.Msg { return BackToDiscoveryMsg{} }

		case key.Matches(msg, m.keys.Quit):
			m.cancel()
			return m, tea.Quit

		case key.Matches(msg, m.keys.NextPane):
			m.focusNextPane()

		case key.Matches(msg, m.keys.PrevPane):
			m.focusPrevPane()

		case key.Matches(msg, m.keys.Confirm):
			// Toggle maximize on current pane
			if m.maximizedPane == -1 {
				m.maximizedPane = m.focusedPane
			} else {
				m.maximizedPane = -1
			}
			m.recalculateLayout()

		// Arrow keys for grid navigation
		case key.Matches(msg, m.keys.Up):
			m.focusUp()

		case key.Matches(msg, m.keys.Down):
			m.focusDown()

		case key.Matches(msg, m.keys.Left):
			m.focusLeft()

		case key.Matches(msg, m.keys.Right):
			m.focusRight()

		// j/k for scrolling focused pane
		case key.Matches(msg, m.keys.ScrollUp):
			if m.focusedPane >= 0 && m.focusedPane < len(m.panes) {
				m.panes[m.focusedPane].Viewport.SetYOffset(m.panes[m.focusedPane].Viewport.YOffset - 3)
			}

		case key.Matches(msg, m.keys.ScrollDown):
			if m.focusedPane >= 0 && m.focusedPane < len(m.panes) {
				m.panes[m.focusedPane].Viewport.SetYOffset(m.panes[m.focusedPane].Viewport.YOffset + 3)
			}

		// Container actions
		case key.Matches(msg, m.keys.Restart):
			if m.focusedPane >= 0 && m.focusedPane < len(m.panes) {
				pane := &m.panes[m.focusedPane]
				pane.AddLogLine(docker.LogLine{
					ContainerID: pane.ID,
					Timestamp:   time.Now(),
					Stream:      "system",
					Content:     "--- Restarting container... ---",
				})
				cmds = append(cmds, m.restartContainer(pane.Container))
			}

		case key.Matches(msg, m.keys.ComposeRestart):
			if m.focusedPane >= 0 && m.focusedPane < len(m.panes) {
				pane := &m.panes[m.focusedPane]
				pane.AddLogLine(docker.LogLine{
					ContainerID: pane.ID,
					Timestamp:   time.Now(),
					Stream:      "system",
					Content:     "--- Running compose down/up... ---",
				})
				cmds = append(cmds, m.composeDownUp(pane.Container))
			}

		case key.Matches(msg, m.keys.ComposeBuild):
			if m.focusedPane >= 0 && m.focusedPane < len(m.panes) {
				pane := &m.panes[m.focusedPane]
				pane.AddLogLine(docker.LogLine{
					ContainerID: pane.ID,
					Timestamp:   time.Now(),
					Stream:      "system",
					Content:     "--- Building (no-cache) and starting... ---",
				})
				cmds = append(cmds, m.composeBuildUp(pane.Container))
			}
		}

	case ContainerActionMsg:
		// Handle action completion
		for i := range m.panes {
			if m.panes[i].ID == msg.ContainerID {
				if msg.Err != nil {
					m.panes[i].AddLogLine(docker.LogLine{
						ContainerID: msg.ContainerID,
						Timestamp:   time.Now(),
						Stream:      "stderr",
						Content:     fmt.Sprintf("--- %s failed: %v ---", msg.Action, msg.Err),
					})
				} else {
					m.panes[i].AddLogLine(docker.LogLine{
						ContainerID: msg.ContainerID,
						Timestamp:   time.Now(),
						Stream:      "system",
						Content:     fmt.Sprintf("--- %s complete ---", msg.Action),
					})
					// Restart log stream for this container
					cmds = append(cmds, m.restartLogStream(m.panes[i].Container))
				}
				break
			}
		}

	case restartStreamMsg:
		// Update pane with new container info and restart log stream
		for i := range m.panes {
			if m.panes[i].ID == msg.OldContainerID {
				// Clean up old stream reference
				if msg.OldContainerID != msg.NewContainer.ID {
					delete(m.streams, msg.OldContainerID)
				}

				// Update container info (ID might have changed)
				m.panes[i].ID = msg.NewContainer.ID
				m.panes[i].Container = msg.NewContainer
				m.panes[i].Connected = true

				// Clear old logs and reset viewport
				m.panes[i].LogLines = make([]docker.LogLine, 0, maxLogLines)
				m.panes[i].Viewport.SetContent("")
				m.panes[i].Viewport.GotoTop()

				// Add a system message indicating restart
				m.panes[i].AddLogLine(docker.LogLine{
					ContainerID: msg.NewContainer.ID,
					Timestamp:   time.Now(),
					Stream:      "system",
					Content:     "--- Container restarted, streaming logs... ---",
				})

				// Start new log stream
				logChan, errChan := m.dockerClient.StreamLogs(m.ctx, msg.NewContainer.ID)
				m.streams[msg.NewContainer.ID] = streamInfo{logChan: logChan, errChan: errChan}

				cmds = append(cmds, m.waitForLog(msg.NewContainer.ID, logChan))
				cmds = append(cmds, m.waitForError(msg.NewContainer.ID, errChan))
				break
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	// Handle mouse wheel scrolling
	if msg.Action == tea.MouseActionPress {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			// Find which pane the mouse is over and scroll it
			for i, pane := range m.panes {
				zoneID := fmt.Sprintf("pane-%s", pane.ID)
				if m.zone.Get(zoneID).InBounds(msg) {
					m.panes[i].Viewport.SetYOffset(m.panes[i].Viewport.YOffset - 3)
					return nil
				}
			}
		case tea.MouseButtonWheelDown:
			for i, pane := range m.panes {
				zoneID := fmt.Sprintf("pane-%s", pane.ID)
				if m.zone.Get(zoneID).InBounds(msg) {
					m.panes[i].Viewport.SetYOffset(m.panes[i].Viewport.YOffset + 3)
					return nil
				}
			}
		case tea.MouseButtonLeft:
			return m.handleMouseClick(msg)
		}
	}
	return nil
}

func (m *Model) handleMouseClick(msg tea.MouseMsg) tea.Cmd {
	// Check which pane was clicked
	for i, pane := range m.panes {
		zoneID := fmt.Sprintf("pane-%s", pane.ID)
		if m.zone.Get(zoneID).InBounds(msg) {
			now := time.Now()

			// Check for double-click
			if m.lastClickPaneID == pane.ID &&
				now.Sub(m.lastClickTime) < doubleClickThreshold {
				// Double-click detected - toggle maximize
				if m.maximizedPane == -1 {
					m.maximizedPane = i
				} else {
					m.maximizedPane = -1
				}
				m.recalculateLayout()
				m.lastClickPaneID = ""
				return nil
			}

			// Single click - focus pane
			m.setFocus(i)
			m.lastClickTime = now
			m.lastClickPaneID = pane.ID
			break
		}
	}

	return nil
}

func (m *Model) setFocus(index int) {
	if index >= 0 && index < len(m.panes) {
		for i := range m.panes {
			m.panes[i].Active = (i == index)
		}
		m.focusedPane = index
	}
}

func (m *Model) focusNextPane() {
	if len(m.panes) == 0 {
		return
	}
	m.setFocus((m.focusedPane + 1) % len(m.panes))
}

func (m *Model) focusPrevPane() {
	if len(m.panes) == 0 {
		return
	}
	newFocus := m.focusedPane - 1
	if newFocus < 0 {
		newFocus = len(m.panes) - 1
	}
	m.setFocus(newFocus)
}

// getPaneGridPosition returns the row and column of a pane index
func (m *Model) getPaneGridPosition(paneIdx int) (row, col int) {
	for r := 0; r < m.layout.Rows; r++ {
		for c := 0; c < m.layout.Cols; c++ {
			if m.layout.PaneMap[r][c] == paneIdx {
				return r, c
			}
		}
	}
	return 0, 0
}

func (m *Model) focusUp() {
	if len(m.panes) == 0 || m.layout.Rows <= 1 {
		return
	}
	row, col := m.getPaneGridPosition(m.focusedPane)
	newRow := row - 1
	if newRow < 0 {
		newRow = m.layout.Rows - 1
	}
	// Find a valid pane in the new row
	for newRow >= 0 {
		if newIdx := m.layout.PaneMap[newRow][col]; newIdx >= 0 && newIdx < len(m.panes) {
			m.setFocus(newIdx)
			return
		}
		newRow--
	}
}

func (m *Model) focusDown() {
	if len(m.panes) == 0 || m.layout.Rows <= 1 {
		return
	}
	row, col := m.getPaneGridPosition(m.focusedPane)
	newRow := row + 1
	if newRow >= m.layout.Rows {
		newRow = 0
	}
	// Find a valid pane in the new row
	for newRow < m.layout.Rows {
		if newIdx := m.layout.PaneMap[newRow][col]; newIdx >= 0 && newIdx < len(m.panes) {
			m.setFocus(newIdx)
			return
		}
		newRow++
	}
}

func (m *Model) focusLeft() {
	if len(m.panes) == 0 || m.layout.Cols <= 1 {
		return
	}
	row, col := m.getPaneGridPosition(m.focusedPane)
	newCol := col - 1
	if newCol < 0 {
		newCol = m.layout.Cols - 1
	}
	// Find a valid pane in the new column
	for newCol >= 0 {
		if newIdx := m.layout.PaneMap[row][newCol]; newIdx >= 0 && newIdx < len(m.panes) {
			m.setFocus(newIdx)
			return
		}
		newCol--
	}
}

func (m *Model) focusRight() {
	if len(m.panes) == 0 || m.layout.Cols <= 1 {
		return
	}
	row, col := m.getPaneGridPosition(m.focusedPane)
	newCol := col + 1
	if newCol >= m.layout.Cols {
		newCol = 0
	}
	// Find a valid pane in the new column
	for newCol < m.layout.Cols {
		if newIdx := m.layout.PaneMap[row][newCol]; newIdx >= 0 && newIdx < len(m.panes) {
			m.setFocus(newIdx)
			return
		}
		newCol++
	}
}

func (m *Model) recalculateLayout() {
	// Reserve 1 line for help bar
	availableHeight := m.height - 1

	if m.maximizedPane >= 0 {
		// Maximized mode - single pane uses full screen minus help bar
		m.panes[m.maximizedPane].SetSize(m.width, availableHeight)
	} else {
		// Tiled mode
		m.layout = CalculateLayout(len(m.panes))
		paneWidth := m.layout.PaneWidth(m.width)
		paneHeight := m.layout.PaneHeight(availableHeight)

		for i := range m.panes {
			m.panes[i].SetSize(paneWidth, paneHeight)
		}
	}
}

// View renders the model
func (m Model) View() string {
	if len(m.panes) == 0 {
		return "No containers selected"
	}

	// Maximized view
	if m.maximizedPane >= 0 && m.maximizedPane < len(m.panes) {
		pane := m.panes[m.maximizedPane]
		paneView := m.zone.Mark(
			fmt.Sprintf("pane-%s", pane.ID),
			pane.View(m.width, m.height-1, true),
		)
		helpBar := m.renderHelpBar()
		return m.zone.Scan(lipgloss.JoinVertical(lipgloss.Left, paneView, helpBar))
	}

	// Tiled view
	return m.zone.Scan(m.renderTiledView())
}

func (m Model) renderTiledView() string {
	// Reserve 1 line for help bar
	availableHeight := m.height - 1

	// Calculate base dimensions and remainders for even distribution
	baseWidth := m.width / m.layout.Cols
	extraWidth := m.width % m.layout.Cols

	baseHeight := availableHeight / m.layout.Rows
	extraHeight := availableHeight % m.layout.Rows

	var rows []string

	for rowIdx := 0; rowIdx < m.layout.Rows; rowIdx++ {
		// Distribute extra height to first rows
		paneHeight := baseHeight
		if rowIdx < extraHeight {
			paneHeight++
		}

		var cols []string
		for colIdx := 0; colIdx < m.layout.Cols; colIdx++ {
			// Distribute extra width to first columns
			paneWidth := baseWidth
			if colIdx < extraWidth {
				paneWidth++
			}

			paneIdx := m.layout.PaneMap[rowIdx][colIdx]
			if paneIdx >= 0 && paneIdx < len(m.panes) {
				pane := m.panes[paneIdx]
				focused := paneIdx == m.focusedPane
				paneContent := m.zone.Mark(
					fmt.Sprintf("pane-%s", pane.ID),
					pane.View(paneWidth, paneHeight, focused),
				)
				cols = append(cols, paneContent)
			} else {
				// Empty cell
				cols = append(cols, lipgloss.NewStyle().
					Width(paneWidth).
					Height(paneHeight).
					Render(""))
			}
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cols...))
	}

	// Add help bar
	helpBar := m.renderHelpBar()
	rows = append(rows, helpBar)

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m Model) renderHelpBar() string {
	key := common.HelpKeyStyle.Render
	desc := common.HelpDescStyle.Render

	help := " " + key("r") + desc(":restart") +
		desc("  ") + key("R") + desc(":down/up") +
		desc("  ") + key("B") + desc(":build") +
		desc("  ") + key("←↑↓→") + desc(":nav") +
		desc("  ") + key("j/k") + desc(":scroll") +
		desc("  ") + key("enter") + desc(":max") +
		desc("  ") + key("esc") + desc(":back") +
		desc("  ") + key("q") + desc(":quit")

	return common.HelpBarStyle.Width(m.width).Render(help)
}

// Cleanup cancels any running goroutines
func (m *Model) Cleanup() {
	if m.cancel != nil {
		m.cancel()
	}
}

// restartContainer restarts a container
func (m Model) restartContainer(cont docker.Container) tea.Cmd {
	return func() tea.Msg {
		err := m.dockerClient.RestartContainer(m.ctx, cont.ID)
		return ContainerActionMsg{
			ContainerID: cont.ID,
			Action:      "Restart",
			Err:         err,
		}
	}
}

// composeDownUp runs compose down/up for a container
func (m Model) composeDownUp(cont docker.Container) tea.Cmd {
	return func() tea.Msg {
		err := m.dockerClient.ComposeDownUp(m.ctx, cont)
		return ContainerActionMsg{
			ContainerID: cont.ID,
			Action:      "Compose down/up",
			Err:         err,
		}
	}
}

// composeBuildUp runs compose build --no-cache and up for a container
func (m Model) composeBuildUp(cont docker.Container) tea.Cmd {
	return func() tea.Msg {
		err := m.dockerClient.ComposeBuildUp(m.ctx, cont)
		return ContainerActionMsg{
			ContainerID: cont.ID,
			Action:      "Build & up",
			Err:         err,
		}
	}
}

// restartLogStream restarts the log stream for a container after an action
func (m Model) restartLogStream(cont docker.Container) tea.Cmd {
	return func() tea.Msg {
		// Small delay to let container restart
		time.Sleep(500 * time.Millisecond)

		// Get the new container ID (might have changed after compose down/up)
		containers, err := m.dockerClient.ListContainers(m.ctx)
		if err != nil {
			return nil
		}

		// Find the container by compose service name or original name
		for _, c := range containers {
			if c.ComposeProject == cont.ComposeProject && c.ComposeService == cont.ComposeService {
				return restartStreamMsg{
					OldContainerID: cont.ID,
					NewContainer:   c,
				}
			}
			if c.Name == cont.Name && c.State == "running" {
				return restartStreamMsg{
					OldContainerID: cont.ID,
					NewContainer:   c,
				}
			}
		}
		return nil
	}
}

type restartStreamMsg struct {
	OldContainerID string
	NewContainer   docker.Container
}
