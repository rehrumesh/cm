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

	// Title bar
	title := p.Container.DisplayName()
	if !p.Connected {
		title += " (disconnected)"
	}
	if p.Paused {
		title += " [PAUSED]"
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
