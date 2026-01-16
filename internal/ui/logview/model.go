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
)

const (
	doubleClickThreshold = 400 * time.Millisecond
	resizeDebounceDelay  = 50 * time.Millisecond
)

type resizeTickMsg struct{}

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

	// For double-click detection
	lastClickTime   time.Time
	lastClickPaneID string

	// For resize debouncing
	pendingResize bool
	lastWidth     int
	lastHeight    int

	// Config modal
	configModal common.ConfigModal

	// Toast notifications
	toast common.Toast
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
		lastWidth:     width,
		lastHeight:    height,
		configModal:   common.NewConfigModal(),
		toast:         common.NewToast(),
	}

	// Calculate layout
	m.layout = CalculateLayout(len(containers))

	// Reserve 1 line for help bar
	availableHeight := height - 1

	// Calculate base dimensions and remainders for proper distribution
	baseWidth := width / m.layout.Cols
	extraWidth := width % m.layout.Cols
	baseHeight := availableHeight / m.layout.Rows
	extraHeight := availableHeight % m.layout.Rows

	// Create panes with correct sizes based on grid position
	paneIdx := 0
	for rowIdx := 0; rowIdx < m.layout.Rows && paneIdx < len(containers); rowIdx++ {
		paneHeight := baseHeight
		if rowIdx < extraHeight {
			paneHeight++
		}

		for colIdx := 0; colIdx < m.layout.Cols && paneIdx < len(containers); colIdx++ {
			paneWidth := baseWidth
			if colIdx < extraWidth {
				paneWidth++
			}

			m.panes[paneIdx] = NewPane(containers[paneIdx], paneWidth, paneHeight)
			paneIdx++
		}
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

	// Handle config modal messages first
	if m.configModal.IsVisible() {
		var cmd tea.Cmd
		m.configModal, cmd = m.configModal.Update(msg)
		return m, cmd
	}

	// Handle modal closed message
	if closed, ok := msg.(common.ConfigModalClosedMsg); ok {
		// Reload key bindings and toast settings in case they changed
		m.keys = common.DefaultKeyMap()
		if closed.ConfigChanged {
			m.toast.ReloadConfig()
		}
		return m, nil
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
		// Debounce resize to prevent flickering
		if !m.pendingResize {
			m.pendingResize = true
			return m, tea.Tick(resizeDebounceDelay, func(t time.Time) tea.Msg {
				return resizeTickMsg{}
			})
		}

	case resizeTickMsg:
		m.pendingResize = false
		// Only recalculate if dimensions actually changed
		if m.width != m.lastWidth || m.height != m.lastHeight {
			m.lastWidth = m.width
			m.lastHeight = m.height
			m.recalculateLayout()
		}

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
			// If a pane is maximized, un-maximize it first
			if m.maximizedPane != -1 {
				m.maximizedPane = -1
				m.recalculateLayout()
				return m, nil
			}
			// Otherwise go back to discovery
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

		// Arrow keys: scroll when maximized, navigate when tiled
		case key.Matches(msg, m.keys.Up):
			if m.maximizedPane != -1 {
				// Scroll up when maximized
				if m.maximizedPane >= 0 && m.maximizedPane < len(m.panes) {
					m.panes[m.maximizedPane].Viewport.SetYOffset(m.panes[m.maximizedPane].Viewport.YOffset - 1)
				}
			} else {
				m.focusUp()
			}

		case key.Matches(msg, m.keys.Down):
			if m.maximizedPane != -1 {
				// Scroll down when maximized
				if m.maximizedPane >= 0 && m.maximizedPane < len(m.panes) {
					m.panes[m.maximizedPane].Viewport.SetYOffset(m.panes[m.maximizedPane].Viewport.YOffset + 1)
				}
			} else {
				m.focusDown()
			}

		case key.Matches(msg, m.keys.Left):
			m.focusLeft()

		case key.Matches(msg, m.keys.Right):
			m.focusRight()

		// ctrl+u/d for scrolling focused pane (faster scroll)
		case key.Matches(msg, m.keys.ScrollUp):
			paneIdx := m.focusedPane
			if m.maximizedPane != -1 {
				paneIdx = m.maximizedPane
			}
			if paneIdx >= 0 && paneIdx < len(m.panes) {
				m.panes[paneIdx].Viewport.SetYOffset(m.panes[paneIdx].Viewport.YOffset - 3)
			}

		case key.Matches(msg, m.keys.ScrollDown):
			paneIdx := m.focusedPane
			if m.maximizedPane != -1 {
				paneIdx = m.maximizedPane
			}
			if paneIdx >= 0 && paneIdx < len(m.panes) {
				m.panes[paneIdx].Viewport.SetYOffset(m.panes[paneIdx].Viewport.YOffset + 3)
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

		case key.Matches(msg, m.keys.Config):
			return m, m.configModal.Open()
		}

	case ContainerActionMsg:
		// Handle action completion
		for i := range m.panes {
			if m.panes[i].ID == msg.ContainerID {
				serviceName := m.panes[i].Container.DisplayName()
				if msg.Err != nil {
					m.panes[i].AddLogLine(docker.LogLine{
						ContainerID: msg.ContainerID,
						Timestamp:   time.Now(),
						Stream:      "stderr",
						Content:     fmt.Sprintf("--- %s failed: %v ---", msg.Action, msg.Err),
					})
					cmds = append(cmds, m.toast.Show(msg.Action+" Failed", serviceName, common.ToastError))
				} else {
					m.panes[i].AddLogLine(docker.LogLine{
						ContainerID: msg.ContainerID,
						Timestamp:   time.Now(),
						Stream:      "system",
						Content:     fmt.Sprintf("--- %s complete ---", msg.Action),
					})
					cmds = append(cmds, m.toast.Show(msg.Action+" Complete", serviceName, common.ToastSuccess))
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

// getPaneAtPosition returns the pane index at the given mouse position, or -1 if none
func (m *Model) getPaneAtPosition(x, y int) int {
	// If maximized, the maximized pane covers the whole screen
	if m.maximizedPane >= 0 {
		return m.maximizedPane
	}

	// Reserve 1 line for help bar
	availableHeight := m.height - 1

	// Calculate base dimensions and remainders (same as renderTiledView)
	baseWidth := m.width / m.layout.Cols
	extraWidth := m.width % m.layout.Cols

	baseHeight := availableHeight / m.layout.Rows
	extraHeight := availableHeight % m.layout.Rows

	// Calculate cumulative positions to determine which cell the mouse is in
	// Find the column
	colX := 0
	targetCol := -1
	for col := 0; col < m.layout.Cols; col++ {
		paneWidth := baseWidth
		if col < extraWidth {
			paneWidth++
		}
		if x >= colX && x < colX+paneWidth {
			targetCol = col
			break
		}
		colX += paneWidth
	}

	// Find the row
	rowY := 0
	targetRow := -1
	for row := 0; row < m.layout.Rows; row++ {
		paneHeight := baseHeight
		if row < extraHeight {
			paneHeight++
		}
		if y >= rowY && y < rowY+paneHeight {
			targetRow = row
			break
		}
		rowY += paneHeight
	}

	// If we found a valid cell, return the pane index
	if targetRow >= 0 && targetCol >= 0 {
		paneIdx := m.layout.PaneMap[targetRow][targetCol]
		if paneIdx >= 0 && paneIdx < len(m.panes) {
			return paneIdx
		}
	}

	return -1
}

func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	// Handle mouse wheel scrolling
	if msg.Action == tea.MouseActionPress {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			// Find which pane the mouse is over using grid position
			paneIdx := m.getPaneAtPosition(msg.X, msg.Y)
			if paneIdx >= 0 {
				m.panes[paneIdx].Viewport.SetYOffset(m.panes[paneIdx].Viewport.YOffset - 3)
			}
			return nil
		case tea.MouseButtonWheelDown:
			paneIdx := m.getPaneAtPosition(msg.X, msg.Y)
			if paneIdx >= 0 {
				m.panes[paneIdx].Viewport.SetYOffset(m.panes[paneIdx].Viewport.YOffset + 3)
			}
			return nil
		case tea.MouseButtonLeft:
			return m.handleMouseClick(msg)
		}
	}
	return nil
}

func (m *Model) handleMouseClick(msg tea.MouseMsg) tea.Cmd {
	// Check which pane was clicked using grid position
	paneIdx := m.getPaneAtPosition(msg.X, msg.Y)
	if paneIdx < 0 {
		return nil
	}

	pane := m.panes[paneIdx]
	now := time.Now()

	// Check for double-click
	if m.lastClickPaneID == pane.ID &&
		now.Sub(m.lastClickTime) < doubleClickThreshold {
		// Double-click detected - toggle maximize
		if m.maximizedPane == -1 {
			m.maximizedPane = paneIdx
		} else {
			m.maximizedPane = -1
		}
		m.recalculateLayout()
		m.lastClickPaneID = ""
		return nil
	}

	// Single click - focus pane
	m.setFocus(paneIdx)
	m.lastClickTime = now
	m.lastClickPaneID = pane.ID

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
	// Safety checks
	if len(m.panes) == 0 || m.width <= 0 || m.height <= 0 {
		return
	}

	// Reserve 1 line for help bar
	availableHeight := m.height - 1
	if availableHeight < 1 {
		availableHeight = 1
	}

	if m.maximizedPane >= 0 && m.maximizedPane < len(m.panes) {
		// Maximized mode - single pane uses full screen minus help bar
		m.panes[m.maximizedPane].SetSize(m.width, availableHeight)
	} else {
		// Tiled mode - calculate layout and set pane sizes with proper remainder distribution
		m.layout = CalculateLayout(len(m.panes))

		// Safety check for layout
		if m.layout.Cols <= 0 || m.layout.Rows <= 0 {
			return
		}

		// Calculate base dimensions and remainders (same as renderTiledView)
		baseWidth := m.width / m.layout.Cols
		extraWidth := m.width % m.layout.Cols
		baseHeight := availableHeight / m.layout.Rows
		extraHeight := availableHeight % m.layout.Rows

		// Ensure minimum dimensions
		if baseWidth < 4 {
			baseWidth = 4
		}
		if baseHeight < 3 {
			baseHeight = 3
		}

		// Set each pane's size based on its grid position
		for rowIdx := 0; rowIdx < m.layout.Rows; rowIdx++ {
			paneHeight := baseHeight
			if rowIdx < extraHeight {
				paneHeight++
			}

			for colIdx := 0; colIdx < m.layout.Cols; colIdx++ {
				paneWidth := baseWidth
				if colIdx < extraWidth {
					paneWidth++
				}

				paneIdx := m.layout.PaneMap[rowIdx][colIdx]
				if paneIdx >= 0 && paneIdx < len(m.panes) {
					m.panes[paneIdx].SetSize(paneWidth, paneHeight)
				}
			}
		}
	}
}

// View renders the model
func (m Model) View() (result string) {
	// Recover from any panics to prevent crashes
	defer func() {
		if r := recover(); r != nil {
			result = fmt.Sprintf("Render error: %v - press 'q' to quit", r)
		}
	}()

	if len(m.panes) == 0 {
		return "No containers selected"
	}

	var content string

	// Maximized view
	if m.maximizedPane >= 0 && m.maximizedPane < len(m.panes) {
		pane := m.panes[m.maximizedPane]
		paneView := pane.View(m.width, m.height-1, true)
		helpBar := m.renderHelpBar()
		content = lipgloss.JoinVertical(lipgloss.Left, paneView, helpBar)
	} else {
		// Tiled view
		content = m.renderTiledView()
	}

	// Overlay config modal if visible
	if m.configModal.IsVisible() {
		modalView := m.configModal.View(m.width, m.height)
		return lipgloss.Place(m.width, m.height,
			lipgloss.Left, lipgloss.Top,
			content,
			lipgloss.WithWhitespaceChars(" "),
		) + "\n" + modalView
	}

	// Add toast notification if visible
	if m.toast.IsVisible() {
		toastContent := m.toast.RenderInline()
		// Right-align toast
		toastLine := lipgloss.NewStyle().
			Width(m.width).
			Align(lipgloss.Right).
			Render(toastContent)
		content = content + "\n" + toastLine
	}

	return content
}

func (m Model) renderTiledView() string {
	// Safety checks to prevent panics
	if m.layout.Cols <= 0 || m.layout.Rows <= 0 || len(m.panes) == 0 {
		return m.renderHelpBar()
	}
	if m.width <= 0 || m.height <= 0 {
		return "Invalid dimensions"
	}

	// Reserve 1 line for help bar
	availableHeight := m.height - 1
	if availableHeight < 1 {
		availableHeight = 1
	}

	// Calculate base dimensions and remainders for even distribution
	baseWidth := m.width / m.layout.Cols
	extraWidth := m.width % m.layout.Cols

	baseHeight := availableHeight / m.layout.Rows
	extraHeight := availableHeight % m.layout.Rows

	// Ensure minimum dimensions
	if baseWidth < 4 {
		baseWidth = 4
	}
	if baseHeight < 3 {
		baseHeight = 3
	}

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
				paneContent := pane.View(paneWidth, paneHeight, focused)
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

	var help string
	if m.maximizedPane != -1 {
		// Maximized pane view
		help = " " + key("r") + desc(":restart") +
			desc("  ") + key("R") + desc(":down/up") +
			desc("  ") + key("b") + desc(":build") +
			desc("  ") + key("↑↓") + desc(":scroll") +
			desc("  ") + key("enter") + desc("/") + key("esc") + desc(":min") +
			desc("  ") + key("c") + desc(":config") +
			desc("  ") + key("q") + desc(":quit")
	} else {
		// Tiled panes view
		help = " " + key("r") + desc(":restart") +
			desc("  ") + key("R") + desc(":down/up") +
			desc("  ") + key("b") + desc(":build") +
			desc("  ") + key("←↑↓→") + desc(":nav") +
			desc("  ") + key("ctrl+u/d") + desc(":scroll") +
			desc("  ") + key("enter") + desc(":max") +
			desc("  ") + key("c") + desc(":config") +
			desc("  ") + key("esc") + desc(":back") +
			desc("  ") + key("q") + desc(":quit")
	}

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
