package logview

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"cm/internal/debug"
	"cm/internal/docker"
	"cm/internal/ui/common"

	"github.com/atotto/clipboard"
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

// ContainerRemovedMsg is sent when a container is successfully removed
type ContainerRemovedMsg struct {
	ContainerID string
	Err         error
}

type ContainerActionStartMsg struct {
	ContainerID string
	Action      string
}

// StreamClosedMsg is sent when a log stream channel closes (container stopped/restarted)
type StreamClosedMsg struct {
	ContainerID string
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

	// Help modal
	helpModal common.HelpModal

	// Inspect modal
	inspectModal common.InspectModal

	// Search modal
	searchModal common.SearchModal

	// Toast notifications
	toast common.Toast

	// Text selection for copy
	selection Selection

	// Word wrap toggle
	wordWrap bool
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
		helpModal:     common.NewHelpModal(),
		inspectModal:  common.NewInspectModal(),
		searchModal:   common.NewSearchModal(),
		toast:         common.NewToast(),
		selection:     NewSelection(),
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
			// Channel closed - container likely stopped or restarted
			return StreamClosedMsg{ContainerID: containerID}
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

	// Handle ContainerDetailsMsg even when inspect modal is visible
	if detailsMsg, ok := msg.(common.ContainerDetailsMsg); ok {
		m.inspectModal.SetDetails(detailsMsg.Details, detailsMsg.Err)
		return m, nil
	}

	// Handle search modal messages first
	if m.searchModal.IsVisible() {
		var cmd tea.Cmd
		m.searchModal, cmd = m.searchModal.Update(msg)
		return m, cmd
	}

	// Handle inspect modal messages first
	if m.inspectModal.IsVisible() {
		var cmd tea.Cmd
		m.inspectModal, cmd = m.inspectModal.Update(msg)
		return m, cmd
	}

	// Handle help modal messages first
	if m.helpModal.IsVisible() {
		var cmd tea.Cmd
		m.helpModal, cmd = m.helpModal.Update(msg)
		return m, cmd
	}

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
				debug.Log("LogLine received: container=%s stream=%s len=%d", msg.ContainerID[:12], msg.Line.Stream, len(msg.Line.Content))
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
					Content:     fmt.Sprintf("--- Stream error: %v ---", msg.Err),
				})
				m.panes[i].AddLogLine(docker.LogLine{
					ContainerID: msg.ContainerID,
					Timestamp:   time.Now(),
					Stream:      "system",
					Content:     "--- Waiting for container to restart... ---",
				})
				// Try to reconnect in case container was restarted externally
				cmds = append(cmds, m.tryReconnect(m.panes[i].Container))
				break
			}
		}

	case StreamClosedMsg:
		// Log stream channel closed - container likely stopped or restarted externally
		for i := range m.panes {
			if m.panes[i].ID == msg.ContainerID {
				// Only try to reconnect if we haven't already started
				if m.panes[i].Connected {
					m.panes[i].Connected = false
					m.panes[i].AddLogLine(docker.LogLine{
						ContainerID: msg.ContainerID,
						Timestamp:   time.Now(),
						Stream:      "system",
						Content:     "--- Stream ended ---",
					})
					m.panes[i].AddLogLine(docker.LogLine{
						ContainerID: msg.ContainerID,
						Timestamp:   time.Now(),
						Stream:      "system",
						Content:     "--- Waiting for container to restart... ---",
					})
					// Clean up old stream reference
					delete(m.streams, msg.ContainerID)
					// Try to reconnect
					cmds = append(cmds, m.tryReconnect(m.panes[i].Container))
				}
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
			// If maximized, show the newly focused pane
			if m.maximizedPane != -1 {
				m.maximizedPane = m.focusedPane
				m.recalculateLayout()
			}

		case key.Matches(msg, m.keys.PrevPane):
			m.focusPrevPane()
			// If maximized, show the newly focused pane
			if m.maximizedPane != -1 {
				m.maximizedPane = m.focusedPane
				m.recalculateLayout()
			}

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
			if m.maximizedPane != -1 {
				// Scroll left when maximized (horizontal scroll)
				if m.maximizedPane >= 0 && m.maximizedPane < len(m.panes) {
					m.panes[m.maximizedPane].ScrollLeft(10)
				}
			} else {
				m.focusLeft()
			}

		case key.Matches(msg, m.keys.Right):
			if m.maximizedPane != -1 {
				// Scroll right when maximized (horizontal scroll)
				if m.maximizedPane >= 0 && m.maximizedPane < len(m.panes) {
					m.panes[m.maximizedPane].ScrollRight(10)
				}
			} else {
				m.focusRight()
			}

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
				debug.Log("Restart requested for container: %s", pane.Container.DisplayName())
				pane.AddLogLine(docker.LogLine{
					ContainerID: pane.ID,
					Timestamp:   time.Now(),
					Stream:      "system",
					Content:     "--- Restarting container... ---",
				})
				cmds = append(cmds, m.restartContainer(pane.Container))
			}

		case key.Matches(msg, m.keys.Kill):
			if m.focusedPane >= 0 && m.focusedPane < len(m.panes) {
				pane := &m.panes[m.focusedPane]
				if pane.Container.State == "running" {
					debug.Log("Kill requested for container: %s", pane.Container.DisplayName())
					pane.AddLogLine(docker.LogLine{
						ContainerID: pane.ID,
						Timestamp:   time.Now(),
						Stream:      "system",
						Content:     "--- Killing container... ---",
					})
					cmds = append(cmds, m.killContainer(pane.Container))
				} else {
					cmds = append(cmds, m.toast.Show("Cannot kill", "Container not running", common.ToastError))
				}
			}

		case key.Matches(msg, m.keys.Remove):
			if m.focusedPane >= 0 && m.focusedPane < len(m.panes) {
				pane := &m.panes[m.focusedPane]
				debug.Log("Remove requested for container: %s", pane.Container.DisplayName())
				pane.AddLogLine(docker.LogLine{
					ContainerID: pane.ID,
					Timestamp:   time.Now(),
					Stream:      "system",
					Content:     "--- Removing container... ---",
				})
				cmds = append(cmds, m.removeContainer(pane.Container))
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

		case key.Matches(msg, m.keys.Help):
			return m, m.helpModal.Open()

		case key.Matches(msg, m.keys.Inspect):
			paneIdx := m.focusedPane
			if m.maximizedPane != -1 {
				paneIdx = m.maximizedPane
			}
			if paneIdx >= 0 && paneIdx < len(m.panes) {
				pane := &m.panes[paneIdx]
				m.inspectModal.Open(pane.Container.ID)
				cmds = append(cmds, m.inspectContainer(pane.Container))
			}

		case key.Matches(msg, m.keys.Search):
			return m, m.searchModal.Open()

		case key.Matches(msg, m.keys.CopyLogs):
			// Copy all logs from focused pane to clipboard
			if m.focusedPane >= 0 && m.focusedPane < len(m.panes) {
				pane := &m.panes[m.focusedPane]
				text := pane.GetPlainTextLogs()
				if text != "" {
					if err := clipboard.WriteAll(text); err == nil {
						lineCount := len(pane.LogLines)
						debug.Log("Copied %d lines (%d chars) from %s", lineCount, len(text), pane.Container.DisplayName())
						cmds = append(cmds, m.toast.Show("Copied", fmt.Sprintf("%d lines", lineCount), common.ToastSuccess))
					}
				}
			}

		case key.Matches(msg, m.keys.WordWrap):
			// Toggle word wrap
			m.wordWrap = !m.wordWrap
			debug.Log("Word wrap toggled: %v", m.wordWrap)
			// Re-render all panes with new wrap setting
			for i := range m.panes {
				m.panes[i].SetWordWrap(m.wordWrap)
			}
			status := "off"
			if m.wordWrap {
				status = "on"
			}
			cmds = append(cmds, m.toast.Show("Word Wrap", status, common.ToastSuccess))

		case key.Matches(msg, m.keys.DebugToggle):
			// Toggle debug logging
			enabled := debug.Toggle()
			status := "off"
			if enabled {
				status = "on"
				debug.Log("Debug toggled on from logview")
			}
			cmds = append(cmds, m.toast.Show("Debug Log", status, common.ToastSuccess))

		case key.Matches(msg, m.keys.Exec):
			// Open shell in focused container
			paneIdx := m.focusedPane
			if m.maximizedPane != -1 {
				paneIdx = m.maximizedPane
			}
			if paneIdx >= 0 && paneIdx < len(m.panes) {
				pane := &m.panes[paneIdx]
				if pane.Container.State == "running" {
					debug.Log("Opening shell in container: %s", pane.Container.DisplayName())
					return m, m.execShell(pane.Container)
				} else {
					cmds = append(cmds, m.toast.Show("Cannot exec", "Container not running", common.ToastError))
				}
			}

		case key.Matches(msg, m.keys.ClearLogs):
			// Clear logs from focused pane
			paneIdx := m.focusedPane
			if m.maximizedPane != -1 {
				paneIdx = m.maximizedPane
			}
			if paneIdx >= 0 && paneIdx < len(m.panes) {
				m.panes[paneIdx].ClearLogs()
				cmds = append(cmds, m.toast.Show("Cleared", m.panes[paneIdx].Container.DisplayName(), common.ToastSuccess))
			}

		case key.Matches(msg, m.keys.PauseLogs):
			// Toggle pause on focused pane
			paneIdx := m.focusedPane
			if m.maximizedPane != -1 {
				paneIdx = m.maximizedPane
			}
			if paneIdx >= 0 && paneIdx < len(m.panes) {
				paused := m.panes[paneIdx].TogglePause()
				status := "resumed"
				if paused {
					status = "paused"
				}
				cmds = append(cmds, m.toast.Show("Logs "+status, m.panes[paneIdx].Container.DisplayName(), common.ToastSuccess))
			}

		// Pane number shortcuts (1-9)
		case key.Matches(msg, m.keys.Pane1):
			if len(m.panes) >= 1 {
				m.setFocus(0)
				if m.maximizedPane != -1 {
					m.maximizedPane = 0
					m.recalculateLayout()
				}
			}
		case key.Matches(msg, m.keys.Pane2):
			if len(m.panes) >= 2 {
				m.setFocus(1)
				if m.maximizedPane != -1 {
					m.maximizedPane = 1
					m.recalculateLayout()
				}
			}
		case key.Matches(msg, m.keys.Pane3):
			if len(m.panes) >= 3 {
				m.setFocus(2)
				if m.maximizedPane != -1 {
					m.maximizedPane = 2
					m.recalculateLayout()
				}
			}
		case key.Matches(msg, m.keys.Pane4):
			if len(m.panes) >= 4 {
				m.setFocus(3)
				if m.maximizedPane != -1 {
					m.maximizedPane = 3
					m.recalculateLayout()
				}
			}
		case key.Matches(msg, m.keys.Pane5):
			if len(m.panes) >= 5 {
				m.setFocus(4)
				if m.maximizedPane != -1 {
					m.maximizedPane = 4
					m.recalculateLayout()
				}
			}
		case key.Matches(msg, m.keys.Pane6):
			if len(m.panes) >= 6 {
				m.setFocus(5)
				if m.maximizedPane != -1 {
					m.maximizedPane = 5
					m.recalculateLayout()
				}
			}
		case key.Matches(msg, m.keys.Pane7):
			if len(m.panes) >= 7 {
				m.setFocus(6)
				if m.maximizedPane != -1 {
					m.maximizedPane = 6
					m.recalculateLayout()
				}
			}
		case key.Matches(msg, m.keys.Pane8):
			if len(m.panes) >= 8 {
				m.setFocus(7)
				if m.maximizedPane != -1 {
					m.maximizedPane = 7
					m.recalculateLayout()
				}
			}
		case key.Matches(msg, m.keys.Pane9):
			if len(m.panes) >= 9 {
				m.setFocus(8)
				if m.maximizedPane != -1 {
					m.maximizedPane = 8
					m.recalculateLayout()
				}
			}
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

	case common.SearchModalClosedMsg:
		// Apply search to focused pane
		paneIdx := m.focusedPane
		if m.maximizedPane != -1 {
			paneIdx = m.maximizedPane
		}
		if paneIdx >= 0 && paneIdx < len(m.panes) {
			matchCount := m.panes[paneIdx].SetSearch(msg.Query)
			if msg.Query != "" {
				m.searchModal.SetMatchInfo(1, matchCount)
			}
		}
		return m, nil

	case common.SearchNextMsg:
		paneIdx := m.focusedPane
		if m.maximizedPane != -1 {
			paneIdx = m.maximizedPane
		}
		if paneIdx >= 0 && paneIdx < len(m.panes) {
			current, total := m.panes[paneIdx].NextMatch()
			m.searchModal.SetMatchInfo(current, total)
		}
		return m, nil

	case common.SearchPrevMsg:
		paneIdx := m.focusedPane
		if m.maximizedPane != -1 {
			paneIdx = m.maximizedPane
		}
		if paneIdx >= 0 && paneIdx < len(m.panes) {
			current, total := m.panes[paneIdx].PrevMatch()
			m.searchModal.SetMatchInfo(current, total)
		}
		return m, nil

	case common.SearchClearMsg:
		paneIdx := m.focusedPane
		if m.maximizedPane != -1 {
			paneIdx = m.maximizedPane
		}
		if paneIdx >= 0 && paneIdx < len(m.panes) {
			m.panes[paneIdx].ClearSearch()
		}
		return m, nil

	case ContainerRemovedMsg:
		// Handle container removal
		if msg.Err != nil {
			for i := range m.panes {
				if m.panes[i].ID == msg.ContainerID {
					m.panes[i].AddLogLine(docker.LogLine{
						ContainerID: msg.ContainerID,
						Timestamp:   time.Now(),
						Stream:      "stderr",
						Content:     fmt.Sprintf("--- Remove failed: %v ---", msg.Err),
					})
					cmds = append(cmds, m.toast.Show("Remove Failed", m.panes[i].Container.DisplayName(), common.ToastError))
					break
				}
			}
		} else {
			// Find and remove the pane
			var containerName string
			paneIdx := -1
			for i := range m.panes {
				if m.panes[i].ID == msg.ContainerID {
					containerName = m.panes[i].Container.DisplayName()
					paneIdx = i
					// Clean up stream reference
					delete(m.streams, msg.ContainerID)
					break
				}
			}
			if paneIdx >= 0 {
				// Remove pane from slice
				m.panes = append(m.panes[:paneIdx], m.panes[paneIdx+1:]...)
				// Recalculate layout
				m.layout = CalculateLayout(len(m.panes))
				// Adjust focused pane if needed
				if m.focusedPane >= len(m.panes) {
					m.focusedPane = len(m.panes) - 1
				}
				if m.focusedPane < 0 {
					m.focusedPane = 0
				}
				// Reset maximized pane if it was the removed one
				if m.maximizedPane == paneIdx {
					m.maximizedPane = -1
				} else if m.maximizedPane > paneIdx {
					m.maximizedPane--
				}
				// Update focus states
				for i := range m.panes {
					m.panes[i].Active = (i == m.focusedPane)
				}
				m.recalculateLayout()
				cmds = append(cmds, m.toast.Show("Removed", containerName, common.ToastSuccess))
			}
		}

	case reconnectFailedMsg:
		// All reconnection attempts failed
		for i := range m.panes {
			if m.panes[i].ID == msg.ContainerID {
				m.panes[i].AddLogLine(docker.LogLine{
					ContainerID: msg.ContainerID,
					Timestamp:   time.Now(),
					Stream:      "system",
					Content:     "--- Could not reconnect. Container may have stopped. ---",
				})
				break
			}
		}

	case shellExitMsg:
		// Shell session ended, show toast
		for i := range m.panes {
			if m.panes[i].ID == msg.ContainerID {
				if msg.Err != nil {
					cmds = append(cmds, m.toast.Show("Shell exited", msg.Err.Error(), common.ToastError))
				} else {
					cmds = append(cmds, m.toast.Show("Shell exited", m.panes[i].Container.DisplayName(), common.ToastSuccess))
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

// getPanePosition returns the top-left corner coordinates of a pane by its index
func (m *Model) getPanePosition(paneIdx int) (x, y int) {
	// If maximized, pane is at 0,0
	if m.maximizedPane >= 0 {
		return 0, 0
	}

	// Find the pane's row and column
	row, col := m.getPaneGridPosition(paneIdx)

	// Reserve 1 line for help bar
	availableHeight := m.height - 1

	// Calculate position based on grid
	baseWidth := m.width / m.layout.Cols
	extraWidth := m.width % m.layout.Cols
	baseHeight := availableHeight / m.layout.Rows
	extraHeight := availableHeight % m.layout.Rows

	// Sum up widths of columns before this one
	x = 0
	for c := 0; c < col; c++ {
		w := baseWidth
		if c < extraWidth {
			w++
		}
		x += w
	}

	// Sum up heights of rows before this one
	y = 0
	for r := 0; r < row; r++ {
		h := baseHeight
		if r < extraHeight {
			h++
		}
		y += h
	}

	return x, y
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
	switch msg.Action {
	case tea.MouseActionPress:
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

	case tea.MouseActionMotion:
		// Update selection if one is active
		if m.selection.Active {
			m.selection.Update(msg.X, msg.Y)
			// Update visual selection in the pane with character-level precision
			paneIdx := m.selection.PaneIdx
			if paneIdx >= 0 && paneIdx < len(m.panes) {
				startLine, startCol, endLine, endCol := m.selection.GetNormalizedRange()
				m.panes[paneIdx].UpdateSelectionChar(startLine, startCol, endLine, endCol)
			}
		}

	case tea.MouseActionRelease:
		// Finalize selection and copy to clipboard
		if m.selection.Active && m.selection.HasSelection() {
			paneIdx := m.selection.PaneIdx
			if paneIdx >= 0 && paneIdx < len(m.panes) {
				pane := &m.panes[paneIdx]
				startLine, startCol, endLine, endCol := m.selection.GetNormalizedRange()
				text := pane.GetTextInRangeChar(startLine, startCol, endLine, endCol)
				// Clear selection highlighting
				pane.ClearSelection()
				if text != "" {
					clipboard.WriteAll(text)
					// Count characters or lines for toast message
					charCount := len([]rune(text))
					m.selection.Clear()
					return m.toast.Show("Copied", fmt.Sprintf("%d chars", charCount), common.ToastSuccess)
				}
			}
		}
		// Clear selection highlighting if there was an active selection
		if m.selection.Active && m.selection.PaneIdx >= 0 && m.selection.PaneIdx < len(m.panes) {
			m.panes[m.selection.PaneIdx].ClearSelection()
		}
		m.selection.Clear()
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

	// Single click - focus pane and start selection for potential drag
	m.setFocus(paneIdx)
	m.lastClickTime = now
	m.lastClickPaneID = pane.ID

	// Start selection for potential drag
	paneX, paneY := m.getPanePosition(paneIdx)
	m.selection.Start(msg.X, msg.Y, paneIdx, paneX, paneY)

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

	// Overlay inspect modal if visible
	if m.inspectModal.IsVisible() {
		modalView := m.inspectModal.View(m.width, m.height)
		return lipgloss.Place(m.width, m.height,
			lipgloss.Left, lipgloss.Top,
			content,
			lipgloss.WithWhitespaceChars(" "),
		) + "\n" + modalView
	}

	// Overlay help modal if visible
	if m.helpModal.IsVisible() {
		modalView := m.helpModal.View(m.width, m.height)
		return lipgloss.Place(m.width, m.height,
			lipgloss.Left, lipgloss.Top,
			content,
			lipgloss.WithWhitespaceChars(" "),
		) + "\n" + modalView
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

	// Overlay search modal if visible
	if m.searchModal.IsVisible() {
		modalView := m.searchModal.View(m.width, m.height)
		content = lipgloss.JoinVertical(lipgloss.Left, content, modalView)
	}

	// Overlay toast notification if visible
	if m.toast.IsVisible() {
		content = m.overlayToast(content)
	}

	return content
}

// overlayToast composites the toast on top of the content
func (m Model) overlayToast(content string) string {
	toastContent := m.toast.RenderInline()
	if toastContent == "" {
		return content
	}

	toastWidth := lipgloss.Width(toastContent)
	toastHeight := lipgloss.Height(toastContent)

	// Position toast at bottom-right, above the help bar
	// Leave 1 line for help bar, 1 line padding from bottom
	toastX := m.width - toastWidth - 2
	toastY := m.height - toastHeight - 2

	if toastX < 0 {
		toastX = 0
	}
	if toastY < 0 {
		toastY = 0
	}

	// Split content and toast into lines
	contentLines := strings.Split(content, "\n")
	toastLines := strings.Split(toastContent, "\n")

	// Ensure content has enough lines
	for len(contentLines) < m.height {
		contentLines = append(contentLines, "")
	}

	// Overlay toast onto content
	for i, toastLine := range toastLines {
		targetY := toastY + i
		if targetY < 0 || targetY >= len(contentLines) {
			continue
		}

		// Use ANSI-aware truncation and padding
		contentLine := contentLines[targetY]
		contentLineWidth := lipgloss.Width(contentLine)

		// Build the new line: content up to toastX, then toast
		if contentLineWidth <= toastX {
			// Content is shorter than toast position, pad with spaces
			padding := strings.Repeat(" ", toastX-contentLineWidth)
			contentLines[targetY] = contentLine + padding + toastLine
		} else {
			// Need to truncate content - use ansi-aware truncation
			contentLines[targetY] = truncateWithAnsi(contentLine, toastX) + toastLine
		}
	}

	return strings.Join(contentLines, "\n")
}

// truncateWithAnsi truncates a string to a visual width, preserving ANSI codes
func truncateWithAnsi(s string, width int) string {
	if width <= 0 {
		return ""
	}

	var result strings.Builder
	visualWidth := 0
	inEscape := false
	escapeSeq := strings.Builder{}

	for _, r := range s {
		if inEscape {
			escapeSeq.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				// End of escape sequence
				result.WriteString(escapeSeq.String())
				escapeSeq.Reset()
				inEscape = false
			}
			continue
		}

		if r == '\x1b' {
			inEscape = true
			escapeSeq.WriteRune(r)
			continue
		}

		// Regular character
		if visualWidth >= width {
			break
		}
		result.WriteRune(r)
		visualWidth++
	}

	// Add any pending escape sequence
	if escapeSeq.Len() > 0 {
		result.WriteString(escapeSeq.String())
	}

	// Pad to exact width if needed
	for visualWidth < width {
		result.WriteRune(' ')
		visualWidth++
	}

	return result.String()
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
			desc("  ") + key("e") + desc(":shell") +
			desc("  ") + key("i") + desc(":inspect") +
			desc("  ") + key("/") + desc(":search") +
			desc("  ") + key("P") + desc(":pause") +
			desc("  ") + key("↑↓←→") + desc(":scroll") +
			desc("  ") + key("y") + desc(":copy") +
			desc("  ") + key("?") + desc(":help") +
			desc("  ") + key("esc") + desc(":min") +
			desc("  ") + key("q") + desc(":quit")
	} else {
		// Tiled panes view
		help = " " + key("r") + desc(":restart") +
			desc("  ") + key("R") + desc(":down/up") +
			desc("  ") + key("e") + desc(":shell") +
			desc("  ") + key("i") + desc(":inspect") +
			desc("  ") + key("/") + desc(":search") +
			desc("  ") + key("P") + desc(":pause") +
			desc("  ") + key("←↑↓→") + desc(":nav") +
			desc("  ") + key("y") + desc(":copy") +
			desc("  ") + key("?") + desc(":help") +
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

// killContainer forcefully kills a container
func (m Model) killContainer(cont docker.Container) tea.Cmd {
	return func() tea.Msg {
		err := m.dockerClient.KillContainer(m.ctx, cont.ID)
		return ContainerActionMsg{
			ContainerID: cont.ID,
			Action:      "Kill",
			Err:         err,
		}
	}
}

// removeContainer removes a container
func (m Model) removeContainer(cont docker.Container) tea.Cmd {
	return func() tea.Msg {
		err := m.dockerClient.RemoveContainer(m.ctx, cont.ID)
		return ContainerRemovedMsg{
			ContainerID: cont.ID,
			Err:         err,
		}
	}
}

// inspectContainer fetches container details
func (m Model) inspectContainer(cont docker.Container) tea.Cmd {
	return func() tea.Msg {
		details, err := m.dockerClient.InspectContainer(m.ctx, cont.ID)
		return common.ContainerDetailsMsg{
			Details: details,
			Err:     err,
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

// tryReconnect attempts to reconnect to a container after the stream ends
// This handles external restarts (docker compose down/up from another terminal)
func (m Model) tryReconnect(cont docker.Container) tea.Cmd {
	return func() tea.Msg {
		// Retry a few times with increasing delays
		delays := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second, 5 * time.Second}

		for attempt, delay := range delays {
			time.Sleep(delay)

			// Check if context was cancelled
			select {
			case <-m.ctx.Done():
				return nil
			default:
			}

			containers, err := m.dockerClient.ListContainers(m.ctx)
			if err != nil {
				debug.Log("Reconnect attempt %d failed to list containers: %v", attempt+1, err)
				continue
			}

			// First try: match by compose project + service (most reliable for compose)
			for _, c := range containers {
				if cont.ComposeProject != "" && cont.ComposeService != "" &&
					c.ComposeProject == cont.ComposeProject &&
					c.ComposeService == cont.ComposeService &&
					c.State == "running" {
					debug.Log("Reconnecting to container %s (was %s, now %s)", c.DisplayName(), cont.ID[:12], c.ID[:12])
					return restartStreamMsg{
						OldContainerID: cont.ID,
						NewContainer:   c,
					}
				}
			}

			// Second try: match by container name
			for _, c := range containers {
				if c.Name == cont.Name && c.State == "running" {
					debug.Log("Reconnecting to container %s by name (was %s, now %s)", c.Name, cont.ID[:12], c.ID[:12])
					return restartStreamMsg{
						OldContainerID: cont.ID,
						NewContainer:   c,
					}
				}
			}

			debug.Log("Reconnect attempt %d: container %s not found or not running", attempt+1, cont.DisplayName())
		}

		debug.Log("Giving up reconnection attempts for %s", cont.DisplayName())
		return reconnectFailedMsg{ContainerID: cont.ID}
	}
}

// reconnectFailedMsg is sent when all reconnection attempts have failed
type reconnectFailedMsg struct {
	ContainerID string
}

// shellExitMsg is sent when an exec shell session ends
type shellExitMsg struct {
	ContainerID string
	Err         error
}

// execShell opens an interactive shell in the container
func (m Model) execShell(container docker.Container) tea.Cmd {
	shortID := container.ID
	if len(container.ID) > 12 {
		shortID = container.ID[:12]
	}

	// Build the shell script that prints banner and execs into container
	// Get whoami, pwd inside the container for the banner
	script := fmt.Sprintf(`
# Get container info
USER=$(docker exec %s whoami 2>/dev/null || echo "unknown")
PWD=$(docker exec %s pwd 2>/dev/null || echo "/")

# Print banner
printf '\n'
printf '\033[1;36m┌──────────────────────────────────────────────────────────┐\033[0m\n'
printf '\033[1;36m│\033[0m  \033[1;33mContainer:\033[0m %%s\n' "%s"
printf '\033[1;36m│\033[0m  \033[1;33mID:\033[0m        %%s\n' "%s"
printf '\033[1;36m│\033[0m  \033[1;33mImage:\033[0m     %%s\n' "%s"
printf '\033[1;36m│\033[0m  \033[1;33mUser:\033[0m      %%s\n' "$USER"
printf '\033[1;36m│\033[0m  \033[1;33mWorkdir:\033[0m   %%s\n' "$PWD"
printf '\033[1;36m└──────────────────────────────────────────────────────────┘\033[0m\n'
printf '\n'

# Exec into container
docker exec -it %s sh
`, container.ID, container.ID, container.DisplayName(), shortID, container.Image, container.ID)

	c := exec.Command("sh", "-c", script)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return shellExitMsg{ContainerID: container.ID, Err: err}
	})
}
