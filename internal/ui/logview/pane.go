package logview

import (
	"fmt"
	"regexp"
	"strings"

	"cm/internal/docker"
	"cm/internal/ui/common"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// Regex patterns for terminal control sequences to strip
var (
	// RIS - Reset to Initial State (ESC c) - THIS IS THE DANGEROUS ONE!
	risRe = regexp.MustCompile(`\x1bc`)
	// Screen clearing: CSI n J (ED - Erase in Display)
	clearScreenRe = regexp.MustCompile(`\x1b\[\d*J`)
	// Line clearing: CSI n K (EL - Erase in Line)
	clearLineRe = regexp.MustCompile(`\x1b\[\d*K`)
	// Cursor movement: CSI n;m H or CSI n;m f (CUP - Cursor Position)
	cursorPosRe = regexp.MustCompile(`\x1b\[\d*;?\d*[Hf]`)
	// Cursor up/down/forward/back: CSI n A/B/C/D
	cursorMoveRe = regexp.MustCompile(`\x1b\[\d*[ABCD]`)
	// Cursor horizontal absolute: CSI n G (CHA - move to column n)
	cursorColumnRe = regexp.MustCompile(`\x1b\[\d*G`)
	// Cursor save/restore: CSI s / CSI u or ESC 7 / ESC 8
	cursorSaveRestoreRe = regexp.MustCompile(`\x1b\[?[su78]`)
	// Scroll up/down: CSI n S / CSI n T
	scrollRe = regexp.MustCompile(`\x1b\[\d*[ST]`)
	// Set mode / Reset mode (including alt screen, cursor visibility): CSI ? n h / CSI ? n l
	modeRe = regexp.MustCompile(`\x1b\[\??\d*[hl]`)
	// Window manipulation: CSI n ; n ; n t
	windowRe = regexp.MustCompile(`\x1b\[\d*;\d*;\d*t`)
	// OSC sequences (title changes, etc.): ESC ] ... BEL or ESC ] ... ST
	oscRe = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	// Carriage return that could cause line overwriting
	carriageReturnRe = regexp.MustCompile(`\r`)
	// Device status reports and other CSI sequences we don't need
	miscCsiRe = regexp.MustCompile(`\x1b\[\d*[nqp]`)
	// Catch-all for other CSI sequences (except SGR which uses 'm')
	// This catches things like CSI ? sequences, CSI > sequences, etc.
	otherCsiRe = regexp.MustCompile(`\x1b\[[?>=!]?[\d;]*[^m\d;]`)
	// DCS (Device Control String) sequences
	dcsRe = regexp.MustCompile(`\x1bP[^\x1b]*\x1b\\`)
	// APC (Application Program Command) sequences
	apcRe = regexp.MustCompile(`\x1b_[^\x1b]*\x1b\\`)
	// PM (Privacy Message) sequences
	pmRe = regexp.MustCompile(`\x1b\^[^\x1b]*\x1b\\`)
	// SOS (Start of String) sequences
	sosRe = regexp.MustCompile(`\x1bX[^\x1b]*\x1b\\`)
	// SGR (color/style) sequences - we KEEP these for syntax highlighting
	// The ansiReset at end of each line prevents bleeding into borders
	// Single-character ESC sequences (like ESC c, ESC D, ESC M, etc.)
	singleEscRe = regexp.MustCompile(`\x1b[cDEHMNOPVWXZ7-9=>]`)
)

// sanitizeLogContent removes terminal control sequences that would mess up the viewport
// Also strips color codes - we apply our own consistent styling
func sanitizeLogContent(content string) string {
	// CRITICAL: Strip RIS (Reset to Initial State) first - this is the most dangerous!
	content = risRe.ReplaceAllString(content, "")
	// Strip other single-character ESC sequences
	content = singleEscRe.ReplaceAllString(content, "")

	// Strip problematic sequences
	content = clearScreenRe.ReplaceAllString(content, "")
	content = clearLineRe.ReplaceAllString(content, "")
	content = cursorPosRe.ReplaceAllString(content, "")
	content = cursorMoveRe.ReplaceAllString(content, "")
	content = cursorColumnRe.ReplaceAllString(content, "")
	content = cursorSaveRestoreRe.ReplaceAllString(content, "")
	content = scrollRe.ReplaceAllString(content, "")
	content = modeRe.ReplaceAllString(content, "")
	content = windowRe.ReplaceAllString(content, "")
	content = oscRe.ReplaceAllString(content, "")
	content = miscCsiRe.ReplaceAllString(content, "")
	content = carriageReturnRe.ReplaceAllString(content, "")

	// Additional sequences that might cause issues
	content = dcsRe.ReplaceAllString(content, "")
	content = apcRe.ReplaceAllString(content, "")
	content = pmRe.ReplaceAllString(content, "")
	content = sosRe.ReplaceAllString(content, "")

	// NOTE: We keep SGR (color/style) sequences for syntax highlighting
	// The ansiReset at end of each line in renderLogs() prevents bleeding

	// Catch-all for other CSI sequences (must be last CSI-related)
	content = otherCsiRe.ReplaceAllString(content, "")

	// Trim any leading/trailing whitespace that might result
	content = strings.TrimRight(content, " \t")

	// Limit line length to prevent rendering issues with very long lines
	if len(content) > 1000 {
		content = content[:1000] + "..."
	}

	return content
}

const maxLogLines = 1000

// Pane represents a single log pane
type Pane struct {
	ID        string
	Container docker.Container
	Viewport  viewport.Model
	LogLines  []docker.LogLine
	Active    bool
	Connected bool
	// Cached dimensions to avoid re-renders
	lastWidth  int
	lastHeight int
}

// NewPane creates a new log pane for a container
func NewPane(container docker.Container, width, height int) Pane {
	vp := viewport.New(width-2, height-3) // Account for border and title
	vp.Style = lipgloss.NewStyle()

	return Pane{
		ID:         container.ID,
		Container:  container,
		Viewport:   vp,
		LogLines:   make([]docker.LogLine, 0, maxLogLines),
		Active:     false,
		Connected:  true,
		lastWidth:  width,
		lastHeight: height,
	}
}

// AddLogLine adds a log line to the pane
func (p *Pane) AddLogLine(line docker.LogLine) {
	// Recover from any panics to prevent crashes
	defer func() {
		if r := recover(); r != nil {
			// Silently ignore panics from log processing
		}
	}()

	// Sanitize content immediately when adding to prevent any escape sequences
	// from corrupting the viewport or layout
	line.Content = sanitizeLogContent(line.Content)

	// Skip completely empty lines (after sanitization)
	if strings.TrimSpace(line.Content) == "" {
		return
	}

	p.LogLines = append(p.LogLines, line)

	// Trim if too many lines
	if len(p.LogLines) > maxLogLines {
		p.LogLines = p.LogLines[len(p.LogLines)-maxLogLines:]
	}

	// Update viewport content
	p.Viewport.SetContent(p.renderLogs())
	p.Viewport.GotoBottom()
}

// SetSize updates the pane dimensions
func (p *Pane) SetSize(width, height int) {
	// Skip if dimensions haven't changed
	if width == p.lastWidth && height == p.lastHeight {
		return
	}
	p.lastWidth = width
	p.lastHeight = height

	vpWidth := width - 2 // Account for border
	if vpWidth < 1 {
		vpWidth = 1
	}
	vpHeight := height - 3 // Account for border and title
	if vpHeight < 1 {
		vpHeight = 1
	}
	p.Viewport.Width = vpWidth
	p.Viewport.Height = vpHeight
	p.Viewport.SetContent(p.renderLogs())
}

// ANSI reset sequence to prevent color bleeding
const ansiReset = "\x1b[0m"

// renderLogs renders all log lines as a string
func (p *Pane) renderLogs() (result string) {
	// Recover from any panics during rendering
	defer func() {
		if r := recover(); r != nil {
			result = common.StderrStyle.Render(fmt.Sprintf("Render error: %v", r))
		}
	}()

	if len(p.LogLines) == 0 {
		return common.SubtitleStyle.Render("Waiting for logs...")
	}

	var b strings.Builder
	for _, line := range p.LogLines {
		// Format timestamp
		ts := common.TimestampStyle.Render(line.Timestamp.Format("15:04:05"))

		// Content is already sanitized when added via AddLogLine
		// Preserve original colors from the log output for syntax highlighting
		var content string
		switch line.Stream {
		case "stderr":
			// Only apply our style if content has no colors (simple heuristic)
			if !strings.Contains(line.Content, "\x1b[") {
				content = common.StderrStyle.Render(line.Content)
			} else {
				content = line.Content
			}
		case "system":
			content = common.SubtitleStyle.Render(line.Content)
		default:
			// Keep original colors for stdout
			content = line.Content
		}

		// Add ANSI reset after each line to prevent color bleeding into borders
		b.WriteString(fmt.Sprintf("%s %s%s\n", ts, content, ansiReset))
	}

	return b.String()
}

// View renders the pane using cached dimensions for stability
func (p *Pane) View(width, height int, focused bool) (result string) {
	// Recover from any panics to return a valid-sized placeholder
	defer func() {
		if r := recover(); r != nil {
			result = lipgloss.NewStyle().
				Width(width).
				Height(height).
				Render(fmt.Sprintf("Render error: %v", r))
		}
	}()

	// Ensure minimum dimensions
	if width < 4 {
		width = 4
	}
	if height < 3 {
		height = 3
	}

	// Title bar
	title := p.Container.DisplayName()
	if !p.Connected {
		title += " (disconnected)"
	}

	// Status indicator based on container state
	var status string
	if p.Container.State == "running" {
		status = common.RunningStyle.Render("●")
	} else {
		status = common.StoppedStyle.Render("○")
	}

	// Inner content width (excluding borders)
	innerWidth := width - 2
	if innerWidth < 1 {
		innerWidth = 1
	}

	// Build title with status
	fullTitle := fmt.Sprintf(" %s %s", status, title)
	fullTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("252")).
		Width(innerWidth).
		MaxWidth(innerWidth).
		Render(fullTitle)

	// Choose border style
	borderStyle := common.PaneBorderStyle
	if focused {
		borderStyle = common.PaneActiveBorderStyle
	}

	// Calculate inner height (excluding borders)
	innerHeight := height - 2
	if innerHeight < 1 {
		innerHeight = 1
	}

	// Viewport height (excluding title line)
	vpHeight := innerHeight - 1
	if vpHeight < 1 {
		vpHeight = 1
	}

	// Get viewport content - content is already sanitized when added
	// Ensure viewport content fits exactly within bounds
	viewportContent := lipgloss.NewStyle().
		Width(innerWidth).
		MaxWidth(innerWidth).
		Height(vpHeight).
		MaxHeight(vpHeight).
		Render(p.Viewport.View())

	// Combine title and viewport
	content := lipgloss.JoinVertical(lipgloss.Left,
		fullTitle,
		viewportContent,
	)

	// Render with exact dimensions to prevent layout shifts
	rendered := borderStyle.
		Width(innerWidth).
		Height(innerHeight).
		MaxWidth(width).
		MaxHeight(height).
		Render(content)

	// Final safety: ensure output is exactly the requested dimensions
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		MaxWidth(width).
		MaxHeight(height).
		Render(rendered)
}
