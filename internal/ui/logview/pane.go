package logview

import (
	"fmt"
	"regexp"
	"strings"

	"cm/internal/debug"
	"cm/internal/docker"
	"cm/internal/ui/common"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wrap"
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
	// Word wrap setting
	wordWrap bool
	// Horizontal scroll offset (for non-wrapped mode)
	xOffset int
	// Pause state
	Paused       bool
	pausedBuffer []docker.LogLine

	// Search state
	searchQuery   string
	matchIndices  []int // line indices that match
	currentMatch  int   // index into matchIndices

	// Build mode state
	buildMode      bool
	buildLogs      []docker.OperationLog
	buildOperation string // "build", "up", "down"
	buildStatus    string // "running", "success", "error"

	// Tab state (for maximized mode)
	activeTab        TabType
	statsHistory     *StatsHistory
	processes        []docker.ContainerProcess
	containerDetails *docker.ContainerDetails
	detailsLoaded    bool
	// Scroll offset for tab content
	tabScrollOffset int
	// Show redacted environment variables
	showRedactedEnv bool
}

// NewPane creates a new log pane for a container
func NewPane(container docker.Container, width, height int) Pane {
	vp := viewport.New(width-2, height-3) // Account for border and title
	vp.Style = lipgloss.NewStyle()

	return Pane{
		ID:           container.ID,
		Container:    container,
		Viewport:     vp,
		LogLines:     make([]docker.LogLine, 0, maxLogLines),
		Active:       false,
		Connected:    true,
		lastWidth:    width,
		lastHeight:   height,
		activeTab:    TabLogs,
		statsHistory: NewStatsHistory(),
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

	// If paused, buffer the log line instead of displaying it
	if p.Paused {
		p.pausedBuffer = append(p.pausedBuffer, line)
		// Cap buffer size to prevent memory issues
		if len(p.pausedBuffer) > maxLogLines {
			p.pausedBuffer = p.pausedBuffer[len(p.pausedBuffer)-maxLogLines:]
		}
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

// TogglePause toggles the pause state of the pane
func (p *Pane) TogglePause() bool {
	p.Paused = !p.Paused

	if !p.Paused {
		// Flush buffered logs when unpausing
		for _, line := range p.pausedBuffer {
			p.LogLines = append(p.LogLines, line)
		}
		// Trim if too many lines
		if len(p.LogLines) > maxLogLines {
			p.LogLines = p.LogLines[len(p.LogLines)-maxLogLines:]
		}
		// Clear buffer
		p.pausedBuffer = nil
		// Update viewport
		p.Viewport.SetContent(p.renderLogs())
		p.Viewport.GotoBottom()
	}

	return p.Paused
}

// SetSearch sets the search query and finds matches
func (p *Pane) SetSearch(query string) (matchCount int) {
	p.searchQuery = query
	p.matchIndices = nil
	p.currentMatch = 0

	if query == "" {
		p.Viewport.SetContent(p.renderLogs())
		return 0
	}

	// Find matching lines
	queryLower := strings.ToLower(query)
	for i, line := range p.LogLines {
		if strings.Contains(strings.ToLower(line.Content), queryLower) {
			p.matchIndices = append(p.matchIndices, i)
		}
	}

	// Update viewport with search highlighting
	p.Viewport.SetContent(p.renderLogsWithSearch())

	// If we have matches, jump to the first one
	if len(p.matchIndices) > 0 {
		p.currentMatch = 1
		p.jumpToMatch(0)
	}

	return len(p.matchIndices)
}

// ClearSearch clears the search state
func (p *Pane) ClearSearch() {
	p.searchQuery = ""
	p.matchIndices = nil
	p.currentMatch = 0
	p.Viewport.SetContent(p.renderLogs())
}

// NextMatch jumps to the next search match
func (p *Pane) NextMatch() (current, total int) {
	if len(p.matchIndices) == 0 {
		return 0, 0
	}

	p.currentMatch++
	if p.currentMatch > len(p.matchIndices) {
		p.currentMatch = 1
	}
	p.jumpToMatch(p.currentMatch - 1)

	return p.currentMatch, len(p.matchIndices)
}

// PrevMatch jumps to the previous search match
func (p *Pane) PrevMatch() (current, total int) {
	if len(p.matchIndices) == 0 {
		return 0, 0
	}

	p.currentMatch--
	if p.currentMatch < 1 {
		p.currentMatch = len(p.matchIndices)
	}
	p.jumpToMatch(p.currentMatch - 1)

	return p.currentMatch, len(p.matchIndices)
}

// GetSearchInfo returns current match info
func (p *Pane) GetSearchInfo() (current, total int) {
	return p.currentMatch, len(p.matchIndices)
}

// HasMatches returns true if this pane has search matches
func (p *Pane) HasMatches() bool {
	return len(p.matchIndices) > 0
}

// IsAtLastMatch returns true if currently at the last match
func (p *Pane) IsAtLastMatch() bool {
	return p.currentMatch >= len(p.matchIndices)
}

// IsAtFirstMatch returns true if currently at the first match
func (p *Pane) IsAtFirstMatch() bool {
	return p.currentMatch <= 1
}

// JumpToFirstMatch jumps to the first match and returns match info
func (p *Pane) JumpToFirstMatch() (current, total int) {
	if len(p.matchIndices) == 0 {
		return 0, 0
	}
	p.currentMatch = 1
	p.jumpToMatch(0)
	return p.currentMatch, len(p.matchIndices)
}

// JumpToLastMatch jumps to the last match and returns match info
func (p *Pane) JumpToLastMatch() (current, total int) {
	if len(p.matchIndices) == 0 {
		return 0, 0
	}
	p.currentMatch = len(p.matchIndices)
	p.jumpToMatch(p.currentMatch - 1)
	return p.currentMatch, len(p.matchIndices)
}

// jumpToMatch scrolls the viewport to show a match
func (p *Pane) jumpToMatch(matchIdx int) {
	if matchIdx < 0 || matchIdx >= len(p.matchIndices) {
		return
	}

	lineIdx := p.matchIndices[matchIdx]

	// Calculate the display line (accounting for word wrap)
	displayLine := lineIdx
	if p.wordWrap {
		// In word wrap mode, we need to count wrapped lines
		displayLine = 0
		const timestampWidth = 9
		contentWidth := p.Viewport.Width - timestampWidth - 1
		if contentWidth < 10 {
			contentWidth = 10
		}
		for i := 0; i < lineIdx && i < len(p.LogLines); i++ {
			content := stripANSI(p.LogLines[i].Content)
			lines := (len(content) + contentWidth - 1) / contentWidth
			if lines < 1 {
				lines = 1
			}
			displayLine += lines
		}
	}

	// Center the match in the viewport
	offset := displayLine - p.Viewport.Height/2
	if offset < 0 {
		offset = 0
	}
	p.Viewport.SetYOffset(offset)

	// Re-render with highlighting
	p.Viewport.SetContent(p.renderLogsWithSearch())
}

// renderLogsWithSearch renders log lines with search highlighting
func (p *Pane) renderLogsWithSearch() string {
	if len(p.LogLines) == 0 {
		return common.SubtitleStyle.Render("Waiting for logs...")
	}

	if p.searchQuery == "" {
		return p.renderLogs()
	}

	const timestampWidth = 9
	contentWidth := p.Viewport.Width - timestampWidth - 1
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Search highlight style
	highlightStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("226")).
		Foreground(lipgloss.Color("0")).
		Bold(true)

	// Current match style (different color)
	currentHighlightStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("208")).
		Foreground(lipgloss.Color("0")).
		Bold(true)

	queryLower := strings.ToLower(p.searchQuery)
	currentMatchLine := -1
	if p.currentMatch > 0 && p.currentMatch <= len(p.matchIndices) {
		currentMatchLine = p.matchIndices[p.currentMatch-1]
	}

	var b strings.Builder

	for lineIdx, line := range p.LogLines {
		plainContent := stripANSI(line.Content)
		isCurrentMatch := lineIdx == currentMatchLine
		hasMatch := strings.Contains(strings.ToLower(plainContent), queryLower)

		var content string
		if hasMatch {
			// Highlight matching portions
			content = p.highlightMatches(plainContent, p.searchQuery, highlightStyle, currentHighlightStyle, isCurrentMatch)
		} else {
			// Apply normal styling
			switch line.Stream {
			case "stderr":
				content = common.StderrStyle.Render(plainContent)
			case "system":
				content = common.SubtitleStyle.Render(plainContent)
			default:
				content = plainContent
			}
		}

		if p.wordWrap {
			// Word wrap the content (simplified - highlight before wrap)
			wrappedLines := wrapText(content, contentWidth)
			for i, wline := range wrappedLines {
				var ts string
				if i == 0 {
					ts = common.TimestampStyle.Render(line.Timestamp.Format("15:04:05"))
				} else {
					ts = strings.Repeat(" ", 8)
				}
				b.WriteString(fmt.Sprintf("%s %s%s\n", ts, wline, ansiReset))
			}
		} else {
			ts := common.TimestampStyle.Render(line.Timestamp.Format("15:04:05"))
			b.WriteString(fmt.Sprintf("%s %s%s\n", ts, content, ansiReset))
		}
	}

	return b.String()
}

// highlightMatches highlights all occurrences of query in text
func (p *Pane) highlightMatches(text, query string, style, currentStyle lipgloss.Style, isCurrent bool) string {
	if query == "" {
		return text
	}

	queryLower := strings.ToLower(query)
	textLower := strings.ToLower(text)
	var result strings.Builder

	lastEnd := 0
	for {
		idx := strings.Index(textLower[lastEnd:], queryLower)
		if idx == -1 {
			result.WriteString(text[lastEnd:])
			break
		}

		// Add text before match
		result.WriteString(text[lastEnd : lastEnd+idx])

		// Add highlighted match
		matchText := text[lastEnd+idx : lastEnd+idx+len(query)]
		if isCurrent {
			result.WriteString(currentStyle.Render(matchText))
		} else {
			result.WriteString(style.Render(matchText))
		}

		lastEnd = lastEnd + idx + len(query)
	}

	return result.String()
}

// wrapText wraps text to a given width (simple version)
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	var lines []string
	for len(text) > width {
		lines = append(lines, text[:width])
		text = text[width:]
	}
	if len(text) > 0 {
		lines = append(lines, text)
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
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

// SetWordWrap enables or disables word wrapping and re-renders
func (p *Pane) SetWordWrap(enabled bool) {
	p.wordWrap = enabled
	p.xOffset = 0 // Reset horizontal scroll when toggling wrap
	p.Viewport.SetContent(p.renderLogs())
}

// ClearLogs clears all log lines from the pane
func (p *Pane) ClearLogs() {
	p.LogLines = make([]docker.LogLine, 0, maxLogLines)
	p.Viewport.SetContent(p.renderLogs())
	p.Viewport.GotoTop()
}

// ScrollLeft scrolls the viewport left (for non-wrapped mode)
func (p *Pane) ScrollLeft(amount int) {
	if p.wordWrap {
		return
	}
	p.xOffset -= amount
	if p.xOffset < 0 {
		p.xOffset = 0
	}
	p.Viewport.SetContent(p.renderLogs())
}

// ScrollRight scrolls the viewport right (for non-wrapped mode)
func (p *Pane) ScrollRight(amount int) {
	if p.wordWrap {
		return
	}
	p.xOffset += amount
	// Cap at reasonable max (will be adjusted in render if needed)
	if p.xOffset > 1000 {
		p.xOffset = 1000
	}
	p.Viewport.SetContent(p.renderLogs())
}

// GetScrollInfo returns scroll position info for scroll bar rendering
func (p *Pane) GetScrollInfo() (yOffset, totalLines, visibleLines int) {
	return p.Viewport.YOffset, p.Viewport.TotalLineCount(), p.Viewport.Height
}

// UpdateSelectionChar re-renders the pane with character-level selection highlighting
func (p *Pane) UpdateSelectionChar(startLine, startCol, endLine, endCol int) {
	debug.Log("Pane.UpdateSelectionChar: (%d,%d) to (%d,%d)", startLine, startCol, endLine, endCol)
	p.Viewport.SetContent(p.renderLogsWithCharSelection(startLine, startCol, endLine, endCol))
}

// ClearSelection clears selection highlighting
func (p *Pane) ClearSelection() {
	p.Viewport.SetContent(p.renderLogs())
}

// ANSI reset sequence to prevent color bleeding
const ansiReset = "\x1b[0m"

// renderScrollBar renders a vertical scroll bar
func (p *Pane) renderScrollBar(height, totalLines, offset int) string {
	if height <= 0 || totalLines <= 0 {
		return ""
	}

	// Calculate thumb size and position
	thumbSize := height * height / totalLines
	if thumbSize < 1 {
		thumbSize = 1
	}
	if thumbSize > height {
		thumbSize = height
	}

	// Calculate thumb position
	scrollableLines := totalLines - height
	if scrollableLines <= 0 {
		scrollableLines = 1
	}
	thumbPos := offset * (height - thumbSize) / scrollableLines
	if thumbPos < 0 {
		thumbPos = 0
	}
	if thumbPos > height-thumbSize {
		thumbPos = height - thumbSize
	}

	// Build scroll bar
	var sb strings.Builder
	trackChar := "│"
	thumbChar := "┃"

	trackStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	thumbStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	for i := 0; i < height; i++ {
		if i >= thumbPos && i < thumbPos+thumbSize {
			sb.WriteString(thumbStyle.Render(thumbChar))
		} else {
			sb.WriteString(trackStyle.Render(trackChar))
		}
		if i < height-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// renderLogs renders all log lines as a string
func (p *Pane) renderLogs() (result string) {
	return p.renderLogsWithSelection(-1, -1)
}

// renderLogsWithSelection renders log lines with optional selection highlighting
func (p *Pane) renderLogsWithSelection(selStartLine, selEndLine int) (result string) {
	// Recover from any panics during rendering
	defer func() {
		if r := recover(); r != nil {
			result = common.StderrStyle.Render(fmt.Sprintf("Render error: %v", r))
		}
	}()

	if len(p.LogLines) == 0 {
		return common.SubtitleStyle.Render("Waiting for logs...")
	}

	// Timestamp takes 8 chars (HH:MM:SS) + 1 space
	const timestampWidth = 9
	// Reserve 1 extra char for scroll bar (shown when content exceeds viewport)
	contentWidth := p.Viewport.Width - timestampWidth - 1
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Selection style (inverted colors)
	selStyle := lipgloss.NewStyle().Reverse(true)

	var b strings.Builder
	displayLine := 0 // Track display line for selection highlighting

	for _, line := range p.LogLines {
		// Get plain content
		plainContent := stripANSI(line.Content)

		// Determine styling based on stream type
		applyStyle := func(text string) string {
			switch line.Stream {
			case "stderr":
				if !strings.Contains(line.Content, "\x1b[") {
					return common.StderrStyle.Render(text)
				}
				return text
			case "system":
				return common.SubtitleStyle.Render(text)
			default:
				return text
			}
		}

		if p.wordWrap {
			// Word wrap mode: hard wrap content to fit width (breaks long words)
			wrapped := wrap.String(plainContent, contentWidth)
			wrappedLines := strings.Split(wrapped, "\n")

			for i, wline := range wrappedLines {
				isSelected := selStartLine >= 0 && displayLine >= selStartLine && displayLine <= selEndLine

				var ts string
				if i == 0 {
					ts = common.TimestampStyle.Render(line.Timestamp.Format("15:04:05"))
					if isSelected {
						ts = selStyle.Render(line.Timestamp.Format("15:04:05"))
					}
				} else {
					ts = strings.Repeat(" ", 8) // Indent continuation lines
				}

				styledLine := applyStyle(wline)
				if isSelected {
					styledLine = selStyle.Render(wline)
				}

				b.WriteString(fmt.Sprintf("%s %s%s\n", ts, styledLine, ansiReset))
				displayLine++
			}
		} else {
			// Non-wrap mode: apply horizontal scroll offset
			isSelected := selStartLine >= 0 && displayLine >= selStartLine && displayLine <= selEndLine

			ts := common.TimestampStyle.Render(line.Timestamp.Format("15:04:05"))
			if isSelected {
				ts = selStyle.Render(line.Timestamp.Format("15:04:05"))
			}

			// Apply horizontal scroll offset
			displayContent := plainContent
			if p.xOffset > 0 {
				runes := []rune(plainContent)
				if p.xOffset < len(runes) {
					displayContent = string(runes[p.xOffset:])
				} else {
					displayContent = ""
				}
			}

			content := applyStyle(displayContent)
			if isSelected {
				content = selStyle.Render(displayContent)
			}

			// If original had ANSI codes and not selected and no offset, use original
			if !isSelected && p.xOffset == 0 && strings.Contains(line.Content, "\x1b[") && line.Stream == "stdout" {
				content = line.Content
			}

			b.WriteString(fmt.Sprintf("%s %s%s\n", ts, content, ansiReset))
			displayLine++
		}
	}

	return b.String()
}

// renderLogsWithCharSelection renders log lines with character-level selection highlighting
func (p *Pane) renderLogsWithCharSelection(selStartLine, selStartCol, selEndLine, selEndCol int) (result string) {
	// Recover from any panics during rendering
	defer func() {
		if r := recover(); r != nil {
			result = common.StderrStyle.Render(fmt.Sprintf("Render error: %v", r))
		}
	}()

	if len(p.LogLines) == 0 {
		return common.SubtitleStyle.Render("Waiting for logs...")
	}

	// Timestamp takes 8 chars (HH:MM:SS) + 1 space
	const timestampWidth = 9
	contentWidth := p.Viewport.Width - timestampWidth - 1
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Selection style (inverted colors)
	selStyle := lipgloss.NewStyle().Reverse(true)

	var b strings.Builder
	displayLine := 0

	for _, line := range p.LogLines {
		plainContent := stripANSI(line.Content)
		tsPlain := line.Timestamp.Format("15:04:05")

		if p.wordWrap {
			wrapped := wrap.String(plainContent, contentWidth)
			wrappedLines := strings.Split(wrapped, "\n")

			for i, wline := range wrappedLines {
				var tsDisplay string
				if i == 0 {
					tsDisplay = tsPlain
				} else {
					tsDisplay = strings.Repeat(" ", 8)
				}

				// Build plain line for selection calculation
				plainLine := tsDisplay + " " + wline

				// Apply character-level selection and render
				renderedLine := p.applyCharSelectionPlain(plainLine, displayLine, selStartLine, selStartCol, selEndLine, selEndCol, selStyle, i == 0)
				b.WriteString(renderedLine + ansiReset + "\n")
				displayLine++
			}
		} else {
			displayContent := plainContent
			if p.xOffset > 0 {
				runes := []rune(plainContent)
				if p.xOffset < len(runes) {
					displayContent = string(runes[p.xOffset:])
				} else {
					displayContent = ""
				}
			}

			// Build plain line for selection calculation
			plainLine := tsPlain + " " + displayContent

			// Apply character-level selection and render
			renderedLine := p.applyCharSelectionPlain(plainLine, displayLine, selStartLine, selStartCol, selEndLine, selEndCol, selStyle, true)
			b.WriteString(renderedLine + ansiReset + "\n")
			displayLine++
		}
	}

	return b.String()
}

// applyCharSelectionPlain applies character-level selection to a plain text line
// and returns the line with proper styling (timestamp style + selection highlighting)
func (p *Pane) applyCharSelectionPlain(plainLine string, lineNum, selStartLine, selStartCol, selEndLine, selEndCol int, selStyle lipgloss.Style, hasTimestamp bool) string {
	runes := []rune(plainLine)
	lineLen := len(runes)

	// Check if this line is in the selection range
	if lineNum < selStartLine || lineNum > selEndLine {
		// No selection - apply normal styling
		if hasTimestamp && lineLen >= 9 {
			// Style timestamp (first 8 chars) + space + content
			ts := common.TimestampStyle.Render(string(runes[:8]))
			return ts + string(runes[8:])
		}
		return plainLine
	}

	debug.Log("applyCharSelectionPlain: lineNum=%d lineLen=%d sel=(%d,%d)-(%d,%d)", lineNum, lineLen, selStartLine, selStartCol, selEndLine, selEndCol)

	// Determine selection bounds for this specific line
	var startCol, endCol int
	if lineNum == selStartLine && lineNum == selEndLine {
		// Single line selection
		startCol = selStartCol
		endCol = selEndCol
	} else if lineNum == selStartLine {
		// First line of multi-line selection
		startCol = selStartCol
		endCol = lineLen
	} else if lineNum == selEndLine {
		// Last line of multi-line selection
		startCol = 0
		endCol = selEndCol
	} else {
		// Middle line - select entire line
		startCol = 0
		endCol = lineLen
	}

	// Clamp bounds
	if startCol < 0 {
		startCol = 0
	}
	if endCol > lineLen {
		endCol = lineLen
	}
	debug.Log("applyCharSelectionPlain: after clamp startCol=%d endCol=%d", startCol, endCol)

	if startCol >= endCol {
		// No actual selection on this line - apply normal styling
		if hasTimestamp && lineLen >= 9 {
			ts := common.TimestampStyle.Render(string(runes[:8]))
			return ts + string(runes[8:])
		}
		return plainLine
	}

	// Build the line with selection highlighting
	// We need to handle the timestamp specially (first 8 chars + space)
	var result strings.Builder

	for i := 0; i < lineLen; i++ {
		inSelection := i >= startCol && i < endCol

		if inSelection {
			result.WriteString(selStyle.Render(string(runes[i])))
		} else if hasTimestamp && i < 8 {
			// Timestamp character (not selected)
			result.WriteString(common.TimestampStyle.Render(string(runes[i])))
		} else {
			result.WriteRune(runes[i])
		}
	}

	return result.String()
}

// stripANSI removes all ANSI escape sequences from a string
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// GetPlainTextLogs returns all log lines as plain text (no ANSI codes)
func (p *Pane) GetPlainTextLogs() string {
	if len(p.LogLines) == 0 {
		return ""
	}

	var b strings.Builder
	for _, line := range p.LogLines {
		ts := line.Timestamp.Format("15:04:05")
		content := stripANSI(line.Content)
		b.WriteString(fmt.Sprintf("%s %s\n", ts, content))
	}
	return b.String()
}

// SetBuildMode enters build mode for a specific operation
func (p *Pane) SetBuildMode(operation string) {
	p.buildMode = true
	p.buildOperation = operation
	p.buildStatus = "running"
	p.buildLogs = nil
	p.Viewport.SetContent(p.renderBuildLogs())
	p.Viewport.GotoTop()
}

// AddBuildLog adds a log line while in build mode
func (p *Pane) AddBuildLog(log docker.OperationLog) {
	if !p.buildMode {
		return
	}
	p.buildLogs = append(p.buildLogs, log)
	// Trim if too many lines
	if len(p.buildLogs) > maxLogLines {
		p.buildLogs = p.buildLogs[len(p.buildLogs)-maxLogLines:]
	}
	p.Viewport.SetContent(p.renderBuildLogs())
	p.Viewport.GotoBottom()
}

// EndBuildMode marks build as complete with success/error status
func (p *Pane) EndBuildMode(success bool) {
	if success {
		p.buildStatus = "success"
	} else {
		p.buildStatus = "error"
	}
	p.Viewport.SetContent(p.renderBuildLogs())
}

// ClearBuildMode exits build mode and returns to normal log view
func (p *Pane) ClearBuildMode() {
	p.buildMode = false
	p.buildOperation = ""
	p.buildStatus = ""
	p.buildLogs = nil
	p.Viewport.SetContent(p.renderLogs())
	p.Viewport.GotoBottom()
}

// IsBuildMode returns whether the pane is in build mode
func (p *Pane) IsBuildMode() bool {
	return p.buildMode
}

// GetBuildStatus returns the current build status
func (p *Pane) GetBuildStatus() string {
	return p.buildStatus
}

// renderBuildLogs renders build log content
func (p *Pane) renderBuildLogs() string {
	if len(p.buildLogs) == 0 {
		return common.SubtitleStyle.Render("Waiting for output...")
	}

	const timestampWidth = 9
	contentWidth := p.Viewport.Width - timestampWidth - 1
	if contentWidth < 10 {
		contentWidth = 10
	}

	var b strings.Builder
	for _, log := range p.buildLogs {
		ts := common.TimestampStyle.Render(log.Timestamp.Format("15:04:05"))
		var content string
		switch log.Stream {
		case "stderr":
			content = common.StderrStyle.Render(log.Content)
		case "system":
			content = common.SubtitleStyle.Render(log.Content)
		default:
			content = log.Content
		}

		if p.wordWrap {
			wrapped := wrap.String(stripANSI(log.Content), contentWidth)
			wrappedLines := strings.Split(wrapped, "\n")
			for i, wline := range wrappedLines {
				if i == 0 {
					b.WriteString(fmt.Sprintf("%s %s%s\n", ts, wline, ansiReset))
				} else {
					b.WriteString(fmt.Sprintf("%s %s%s\n", strings.Repeat(" ", 8), wline, ansiReset))
				}
			}
		} else {
			b.WriteString(fmt.Sprintf("%s %s%s\n", ts, content, ansiReset))
		}
	}
	return b.String()
}

// GetTextInRange returns log lines in a given line range (0-indexed, relative to viewport)
func (p *Pane) GetTextInRange(startLine, endLine int) string {
	if len(p.LogLines) == 0 {
		return ""
	}

	// Adjust for viewport scroll offset
	offset := p.Viewport.YOffset
	actualStart := offset + startLine
	actualEnd := offset + endLine

	// Clamp to valid range
	if actualStart < 0 {
		actualStart = 0
	}
	if actualEnd >= len(p.LogLines) {
		actualEnd = len(p.LogLines) - 1
	}
	if actualStart > actualEnd || actualStart >= len(p.LogLines) {
		return ""
	}

	var b strings.Builder
	for i := actualStart; i <= actualEnd; i++ {
		line := p.LogLines[i]
		ts := line.Timestamp.Format("15:04:05")
		content := stripANSI(line.Content)
		b.WriteString(fmt.Sprintf("%s %s\n", ts, content))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// GetTextInRangeChar returns selected text with character-level precision
func (p *Pane) GetTextInRangeChar(startLine, startCol, endLine, endCol int) string {
	if len(p.LogLines) == 0 {
		return ""
	}

	// Build the display lines (same as render) to match what user sees
	const timestampWidth = 9
	contentWidth := p.Viewport.Width - timestampWidth - 1
	if contentWidth < 10 {
		contentWidth = 10
	}

	var displayLines []string
	for _, line := range p.LogLines {
		plainContent := stripANSI(line.Content)
		ts := line.Timestamp.Format("15:04:05")

		if p.wordWrap {
			wrapped := wrap.String(plainContent, contentWidth)
			wrappedLines := strings.Split(wrapped, "\n")
			for i, wline := range wrappedLines {
				if i == 0 {
					displayLines = append(displayLines, ts+" "+wline)
				} else {
					displayLines = append(displayLines, strings.Repeat(" ", 8)+" "+wline)
				}
			}
		} else {
			displayContent := plainContent
			if p.xOffset > 0 {
				runes := []rune(plainContent)
				if p.xOffset < len(runes) {
					displayContent = string(runes[p.xOffset:])
				} else {
					displayContent = ""
				}
			}
			displayLines = append(displayLines, ts+" "+displayContent)
		}
	}

	// Adjust for viewport scroll offset
	offset := p.Viewport.YOffset
	startLine += offset
	endLine += offset

	// Clamp to valid range
	if startLine < 0 {
		startLine = 0
		startCol = 0
	}
	if endLine >= len(displayLines) {
		endLine = len(displayLines) - 1
		if endLine >= 0 {
			endCol = len([]rune(displayLines[endLine]))
		}
	}
	if startLine > endLine || startLine >= len(displayLines) {
		return ""
	}

	// Extract selected text
	var result strings.Builder
	for i := startLine; i <= endLine; i++ {
		if i >= len(displayLines) {
			break
		}
		lineRunes := []rune(displayLines[i])
		lineLen := len(lineRunes)

		var lineStartCol, lineEndCol int
		if i == startLine && i == endLine {
			// Single line selection
			lineStartCol = startCol
			lineEndCol = endCol
		} else if i == startLine {
			// First line
			lineStartCol = startCol
			lineEndCol = lineLen
		} else if i == endLine {
			// Last line
			lineStartCol = 0
			lineEndCol = endCol
		} else {
			// Middle lines - full line
			lineStartCol = 0
			lineEndCol = lineLen
		}

		// Clamp
		if lineStartCol < 0 {
			lineStartCol = 0
		}
		if lineEndCol > lineLen {
			lineEndCol = lineLen
		}
		if lineStartCol < lineEndCol {
			result.WriteString(string(lineRunes[lineStartCol:lineEndCol]))
		}
		if i < endLine {
			result.WriteString("\n")
		}
	}

	return result.String()
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

	// Title bar - different in build mode
	var title string
	var status string

	if p.buildMode {
		// Build mode title
		opName := p.buildOperation
		if opName == "" {
			opName = "Build"
		}
		opName = strings.ToUpper(opName[:1]) + opName[1:]
		title = fmt.Sprintf("%s %s", opName, p.Container.DisplayName())

		// Status indicator based on build status
		switch p.buildStatus {
		case "running":
			status = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render("...")
		case "success":
			status = common.RunningStyle.Render(" OK")
		case "error":
			status = common.StoppedStyle.Render(" ERR")
		default:
			status = ""
		}
	} else {
		// Normal mode title
		title = p.Container.DisplayName()
		if !p.Connected {
			title += " (disconnected)"
		}
		if p.Paused {
			title += " [PAUSED]"
		}

		// Status indicator based on container state
		if p.Container.State == "running" {
			status = common.RunningStyle.Render("●")
		} else {
			status = common.StoppedStyle.Render("○")
		}
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
	totalLines := p.Viewport.TotalLineCount()
	var viewportContent string

	if totalLines > vpHeight && vpHeight > 0 {
		// Add scroll bar when content exceeds viewport height
		scrollBar := p.renderScrollBar(vpHeight, totalLines, p.Viewport.YOffset)
		vc := lipgloss.NewStyle().
			Width(innerWidth - 1).
			MaxWidth(innerWidth - 1).
			Height(vpHeight).
			MaxHeight(vpHeight).
			Render(p.Viewport.View())
		viewportContent = lipgloss.JoinHorizontal(lipgloss.Top, vc, scrollBar)
	} else {
		viewportContent = lipgloss.NewStyle().
			Width(innerWidth).
			MaxWidth(innerWidth).
			Height(vpHeight).
			MaxHeight(vpHeight).
			Render(p.Viewport.View())
	}

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

// Tab management methods

// GetActiveTab returns the current active tab
func (p *Pane) GetActiveTab() TabType {
	return p.activeTab
}

// SetActiveTab sets the active tab
func (p *Pane) SetActiveTab(tab TabType) {
	p.activeTab = tab
	p.tabScrollOffset = 0 // Reset scroll when changing tabs
}

// NextTab cycles to the next tab
func (p *Pane) NextTab() TabType {
	p.activeTab = TabType((int(p.activeTab) + 1) % TabCount())
	p.tabScrollOffset = 0
	return p.activeTab
}

// PrevTab cycles to the previous tab
func (p *Pane) PrevTab() TabType {
	p.activeTab = TabType((int(p.activeTab) - 1 + TabCount()) % TabCount())
	p.tabScrollOffset = 0
	return p.activeTab
}

// AddStats adds a stats sample to the history
func (p *Pane) AddStats(stats docker.ContainerStats) {
	if p.statsHistory != nil {
		p.statsHistory.Add(stats)
	}
}

// SetProcesses updates the process list
func (p *Pane) SetProcesses(processes []docker.ContainerProcess) {
	p.processes = processes
}

// SetContainerDetails sets the cached container details
func (p *Pane) SetContainerDetails(details *docker.ContainerDetails) {
	p.containerDetails = details
	p.detailsLoaded = true
}

// NeedsDetails returns true if container details haven't been loaded yet
func (p *Pane) NeedsDetails() bool {
	return !p.detailsLoaded
}

// ScrollTabUp scrolls the tab content up
func (p *Pane) ScrollTabUp(amount int) {
	p.tabScrollOffset -= amount
	if p.tabScrollOffset < 0 {
		p.tabScrollOffset = 0
	}
}

// ScrollTabDown scrolls the tab content down
func (p *Pane) ScrollTabDown(amount int) {
	p.tabScrollOffset += amount
}

// renderTabBar renders the tab navigation bar
func (p *Pane) renderTabBar(width int) string {
	var tabs []string

	for i, name := range TabNames {
		var style lipgloss.Style
		if TabType(i) == p.activeTab {
			style = common.TabActiveStyle
		} else {
			style = common.TabInactiveStyle
		}
		// Add number prefix
		tabLabel := fmt.Sprintf("%d:%s", i+1, name)
		tabs = append(tabs, style.Render(tabLabel))
	}

	tabBar := strings.Join(tabs, common.TabSeparatorStyle.Render(" "))

	// Center the tab bar
	tabBarWidth := lipgloss.Width(tabBar)
	if tabBarWidth < width {
		padding := (width - tabBarWidth) / 2
		tabBar = strings.Repeat(" ", padding) + tabBar
	}

	return common.TabBarStyle.Width(width).Render(tabBar)
}

// renderStatsTab renders the Stats tab content
func (p *Pane) renderStatsTab(width, height int) string {
	if p.statsHistory == nil || p.statsHistory.Len() == 0 {
		return common.SubtitleStyle.Render("  Waiting for stats data...")
	}

	latest := p.statsHistory.Latest()
	if latest == nil {
		return common.SubtitleStyle.Render("  No stats available")
	}

	var b strings.Builder

	// Render bar charts for CPU and Memory
	memUsage := FormatBytes(latest.MemoryUsage)
	memLimit := FormatBytes(latest.MemoryLimit)

	bars := RenderBarCharts(latest.CPUPercent, latest.MemoryPercent, memUsage, memLimit, width-4, height-6)
	b.WriteString(bars)
	b.WriteString("\n\n")

	// Stats summary at bottom
	labelStyle := common.StatsLabelStyle
	valueStyle := common.StatsValueStyle

	b.WriteString(fmt.Sprintf("  %s %s    %s %d\n",
		labelStyle.Render("Network I/O:"),
		valueStyle.Render(fmt.Sprintf("%s rx / %s tx", FormatBytes(latest.NetworkRx), FormatBytes(latest.NetworkTx))),
		labelStyle.Render("PIDs:"),
		latest.PIDs,
	))

	return b.String()
}

// ToggleRedactedEnv toggles showing/hiding redacted environment variables
func (p *Pane) ToggleRedactedEnv() {
	p.showRedactedEnv = !p.showRedactedEnv
}

// IsShowingRedactedEnv returns whether redacted env vars are being shown
func (p *Pane) IsShowingRedactedEnv() bool {
	return p.showRedactedEnv
}

// renderEnvTab renders the Env tab content
func (p *Pane) renderEnvTab(width, height int) string {
	if p.containerDetails == nil {
		return common.SubtitleStyle.Render("  Loading environment variables...")
	}

	if len(p.containerDetails.Env) == 0 {
		return common.SubtitleStyle.Render("  No environment variables set")
	}

	var b strings.Builder

	// Header with toggle hint
	header := "Environment Variables"
	if p.showRedactedEnv {
		header += " (showing secrets)"
	}
	b.WriteString(fmt.Sprintf("  %s\n", common.StatsLabelStyle.Render(header)))
	b.WriteString(fmt.Sprintf("  %s\n\n", common.MutedInlineStyle.Render("[s] show/hide secrets")))

	// Choose which env list to display
	lines := p.containerDetails.Env
	if p.showRedactedEnv {
		lines = p.containerDetails.RawEnv
	}

	// Apply scroll offset
	startIdx := p.tabScrollOffset
	if startIdx >= len(lines) {
		startIdx = len(lines) - 1
	}
	if startIdx < 0 {
		startIdx = 0
	}

	endIdx := startIdx + height - 6 // Account for header lines
	if endIdx > len(lines) {
		endIdx = len(lines)
	}

	for i := startIdx; i < endIdx; i++ {
		env := lines[i]
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			key := parts[0]
			value := parts[1]

			// Check if value is redacted (only in redacted mode)
			if value == "<redacted>" {
				b.WriteString(fmt.Sprintf("  %s=%s\n",
					common.EnvKeyStyle.Render(key),
					common.EnvRedactedStyle.Render(value),
				))
			} else {
				// Truncate long values
				maxValueLen := width - len(key) - 6
				if maxValueLen < 10 {
					maxValueLen = 10
				}
				if len(value) > maxValueLen {
					value = value[:maxValueLen-3] + "..."
				}
				b.WriteString(fmt.Sprintf("  %s=%s\n",
					common.EnvKeyStyle.Render(key),
					common.EnvValueStyle.Render(value),
				))
			}
		} else {
			b.WriteString(fmt.Sprintf("  %s\n", env))
		}
	}

	// Scroll indicator
	if len(lines) > height-6 {
		b.WriteString(fmt.Sprintf("\n  [%d-%d of %d] (use arrows to scroll)",
			startIdx+1, endIdx, len(lines)))
	}

	return b.String()
}

// renderConfigTab renders the Config tab content
func (p *Pane) renderConfigTab(width, height int) string {
	if p.containerDetails == nil {
		return common.SubtitleStyle.Render("Loading container configuration...")
	}

	d := p.containerDetails
	var lines []string

	// Build all config lines
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  %s  %s", common.StatsLabelStyle.Render("Container ID:"), d.ID))
	lines = append(lines, fmt.Sprintf("  %s  %s", common.StatsLabelStyle.Render("Name:        "), d.Name))
	lines = append(lines, fmt.Sprintf("  %s  %s", common.StatsLabelStyle.Render("Image:       "), d.Image))
	lines = append(lines, fmt.Sprintf("  %s  %s", common.StatsLabelStyle.Render("Status:      "), d.Status))
	lines = append(lines, "")

	// Times
	if !d.Created.IsZero() {
		lines = append(lines, fmt.Sprintf("  %s  %s", common.StatsLabelStyle.Render("Created:     "), d.Created.Format("2006-01-02 15:04:05")))
	}
	if !d.Started.IsZero() {
		lines = append(lines, fmt.Sprintf("  %s  %s", common.StatsLabelStyle.Render("Started:     "), d.Started.Format("2006-01-02 15:04:05")))
	}
	lines = append(lines, "")

	// Command info
	if d.Entrypoint != "" {
		lines = append(lines, fmt.Sprintf("  %s  %s", common.StatsLabelStyle.Render("Entrypoint:  "), truncateString(d.Entrypoint, width-20)))
	}
	if d.Command != "" {
		lines = append(lines, fmt.Sprintf("  %s  %s", common.StatsLabelStyle.Render("Command:     "), truncateString(d.Command, width-20)))
	}
	if d.WorkingDir != "" {
		lines = append(lines, fmt.Sprintf("  %s  %s", common.StatsLabelStyle.Render("WorkingDir:  "), d.WorkingDir))
	}
	if d.RestartPolicy != "" {
		lines = append(lines, fmt.Sprintf("  %s  %s", common.StatsLabelStyle.Render("Restart:     "), d.RestartPolicy))
	}
	lines = append(lines, "")

	// Ports
	if len(d.Ports) > 0 {
		lines = append(lines, fmt.Sprintf("  %s", common.StatsLabelStyle.Render("Ports:")))
		for _, port := range d.Ports {
			lines = append(lines, fmt.Sprintf("    %s", port))
		}
		lines = append(lines, "")
	}

	// Volumes
	if len(d.Volumes) > 0 {
		lines = append(lines, fmt.Sprintf("  %s", common.StatsLabelStyle.Render("Volumes:")))
		for _, vol := range d.Volumes {
			lines = append(lines, fmt.Sprintf("    %s", truncateString(vol, width-6)))
		}
		lines = append(lines, "")
	}

	// Networks
	if len(d.Networks) > 0 {
		lines = append(lines, fmt.Sprintf("  %s  %s", common.StatsLabelStyle.Render("Networks:    "), strings.Join(d.Networks, ", ")))
		lines = append(lines, "")
	}

	// Labels (limited)
	if len(d.Labels) > 0 {
		lines = append(lines, fmt.Sprintf("  %s", common.StatsLabelStyle.Render("Labels:")))
		count := 0
		for k, v := range d.Labels {
			if count >= 10 {
				lines = append(lines, fmt.Sprintf("    ... and %d more", len(d.Labels)-10))
				break
			}
			lines = append(lines, fmt.Sprintf("    %s=%s", k, truncateString(v, width-len(k)-8)))
			count++
		}
	}

	// Apply scroll offset
	startIdx := p.tabScrollOffset
	if startIdx >= len(lines) {
		startIdx = len(lines) - 1
	}
	if startIdx < 0 {
		startIdx = 0
	}

	endIdx := startIdx + height - 2
	if endIdx > len(lines) {
		endIdx = len(lines)
	}

	var b strings.Builder
	for i := startIdx; i < endIdx; i++ {
		b.WriteString(lines[i])
		b.WriteString("\n")
	}

	// Scroll indicator
	if len(lines) > height-2 {
		b.WriteString(fmt.Sprintf("\n  [%d-%d of %d] (use arrows to scroll)",
			startIdx+1, endIdx, len(lines)))
	}

	return b.String()
}

// renderTopTab renders the Top tab content
func (p *Pane) renderTopTab(width, height int) string {
	if len(p.processes) == 0 {
		return common.SubtitleStyle.Render("  Loading processes...\n\n  (Container must be running)")
	}

	var b strings.Builder

	// Header
	headerFmt := "  %-8s %-10s %-10s %s\n"
	b.WriteString(fmt.Sprintf(headerFmt,
		common.TopHeaderStyle.Render("PID"),
		common.TopHeaderStyle.Render("USER"),
		common.TopHeaderStyle.Render("TIME"),
		common.TopHeaderStyle.Render("COMMAND"),
	))
	b.WriteString("  " + strings.Repeat("-", width-4) + "\n")

	// Apply scroll offset
	startIdx := p.tabScrollOffset
	if startIdx >= len(p.processes) {
		startIdx = len(p.processes) - 1
	}
	if startIdx < 0 {
		startIdx = 0
	}

	endIdx := startIdx + height - 5
	if endIdx > len(p.processes) {
		endIdx = len(p.processes)
	}

	rowFmt := "  %-8s %-10s %-10s %s\n"
	for i := startIdx; i < endIdx; i++ {
		proc := p.processes[i]
		cmd := truncateString(proc.Command, width-35)
		b.WriteString(fmt.Sprintf(rowFmt, proc.PID, proc.User, proc.Time, cmd))
	}

	// Scroll indicator
	if len(p.processes) > height-5 {
		b.WriteString(fmt.Sprintf("\n  [%d-%d of %d processes] (use arrows to scroll)",
			startIdx+1, endIdx, len(p.processes)))
	}

	return b.String()
}

// ViewMaximized renders the pane in maximized mode with tabs
func (p *Pane) ViewMaximized(width, height int) string {
	// Tab bar takes 1 line, title takes 1 line, border takes 2 lines
	tabBar := p.renderTabBar(width - 2)

	// Container title/status line
	var status string
	if p.Container.State == "running" {
		status = common.RunningStyle.Render("●")
	} else {
		status = common.StoppedStyle.Render("○")
	}

	title := p.Container.DisplayName()
	if !p.Connected {
		title += " (disconnected)"
	}
	if p.Paused && p.activeTab == TabLogs {
		title += " [PAUSED]"
	}

	titleLine := fmt.Sprintf(" %s %s", status, title)
	titleLine = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("252")).
		Width(width - 2).
		Render(titleLine)

	// Content area height
	contentHeight := height - 5 // borders (2) + tab bar (1) + title (1) + help hint (1)
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Render content based on active tab
	var content string
	switch p.activeTab {
	case TabLogs:
		// Use existing viewport for logs
		p.Viewport.Height = contentHeight
		p.Viewport.Width = width - 4
		content = p.Viewport.View()
	case TabStats:
		content = p.renderStatsTab(width-4, contentHeight)
	case TabEnv:
		content = p.renderEnvTab(width-4, contentHeight)
	case TabConfig:
		content = p.renderConfigTab(width-4, contentHeight)
	case TabTop:
		content = p.renderTopTab(width-4, contentHeight)
	}

	// Constrain content to height
	content = lipgloss.NewStyle().
		Width(width - 4).
		Height(contentHeight).
		MaxHeight(contentHeight).
		Render(content)

	// Hint line
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	hint := hintStyle.Render(" [/]:tabs  [1-5]:jump  arrows:scroll  esc:minimize")

	// Combine all parts
	innerContent := lipgloss.JoinVertical(lipgloss.Left,
		tabBar,
		titleLine,
		content,
		hint,
	)

	// Add border
	return common.PaneActiveBorderStyle.
		Width(width - 2).
		Height(height - 2).
		Render(innerContent)
}

// truncateString truncates a string to a maximum length
func truncateString(s string, maxLen int) string {
	if maxLen <= 3 {
		return s
	}
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
