package logview

import (
	"cm/internal/debug"
)

// Selection tracks mouse text selection state with character-level precision
type Selection struct {
	Selecting bool // Whether a drag selection is in progress
	Selected  bool // Whether there is a finalized selection
	PaneIdx   int  // Which pane is being selected in
	PaneX     int  // Pane's X offset on screen
	PaneY     int  // Pane's Y offset on screen

	// Start position (where mouse was pressed)
	StartLine int // Line number relative to viewport content
	StartCol  int // Column (character) position within the line

	// End position (current drag position)
	EndLine int
	EndCol  int
}

// NewSelection creates a new empty selection
func NewSelection() Selection {
	return Selection{
		Selecting: false,
		Selected:  false,
		PaneIdx:   -1,
	}
}

// Start begins a new selection at the given screen position
func (s *Selection) Start(screenX, screenY, paneIdx, paneX, paneY int) {
	s.Selecting = true
	s.Selected = false
	s.PaneIdx = paneIdx
	s.PaneX = paneX
	s.PaneY = paneY

	// Convert screen coordinates to line/column
	s.StartLine, s.StartCol = s.screenToLineCol(screenX, screenY)
	s.EndLine = s.StartLine
	s.EndCol = s.StartCol
	debug.Log("Selection.Start: screen(%d,%d) pane(%d) panePos(%d,%d) -> line=%d col=%d",
		screenX, screenY, paneIdx, paneX, paneY, s.StartLine, s.StartCol)
}

// Update updates the selection end position
func (s *Selection) Update(screenX, screenY int) {
	if !s.Selecting {
		return
	}
	s.EndLine, s.EndCol = s.screenToLineCol(screenX, screenY)
	debug.Log("Selection.Update: screen(%d,%d) -> endLine=%d endCol=%d", screenX, screenY, s.EndLine, s.EndCol)
}

// Finalize completes the current drag selection and makes it persistent.
// Returns true if a non-empty selection exists after finalizing.
func (s *Selection) Finalize() bool {
	if !s.Selecting {
		return s.HasSelectedText()
	}
	s.Selected = s.hasRangeSelection()
	s.Selecting = false
	return s.Selected
}

// screenToLineCol converts screen coordinates to line and column
func (s *Selection) screenToLineCol(screenX, screenY int) (line, col int) {
	// Account for pane position, border (1 char) and title (1 line)
	contentStartX := s.PaneX + 1 // Border
	contentStartY := s.PaneY + 2 // Border + title line

	line = screenY - contentStartY
	col = screenX - contentStartX

	// Clamp to non-negative
	if line < 0 {
		line = 0
	}
	if col < 0 {
		col = 0
	}

	return line, col
}

// Clear clears the selection
func (s *Selection) Clear() {
	s.Selecting = false
	s.Selected = false
	s.PaneIdx = -1
}

// GetNormalizedRange returns the selection range with start before end
// Returns (startLine, startCol, endLine, endCol)
func (s *Selection) GetNormalizedRange() (startLine, startCol, endLine, endCol int) {
	if s.PaneIdx < 0 {
		return 0, 0, 0, 0
	}

	startLine, startCol = s.StartLine, s.StartCol
	endLine, endCol = s.EndLine, s.EndCol

	// Normalize so start comes before end
	if startLine > endLine || (startLine == endLine && startCol > endCol) {
		startLine, endLine = endLine, startLine
		startCol, endCol = endCol, startCol
	}

	debug.Log("Selection.GetNormalizedRange: (%d,%d) to (%d,%d)", startLine, startCol, endLine, endCol)
	return startLine, startCol, endLine, endCol
}

// HasSelectedText returns true if there's a selection with non-zero range.
func (s *Selection) HasSelectedText() bool {
	if !s.Selecting && !s.Selected {
		return false
	}
	return s.hasRangeSelection()
}

func (s *Selection) hasRangeSelection() bool {
	startLine, startCol, endLine, endCol := s.GetNormalizedRange()
	// Has selection if spans multiple lines or multiple columns
	return endLine > startLine || endCol > startCol
}

// GetLineRange returns just the line range (for compatibility)
func (s *Selection) GetLineRange() (startLine, endLine int) {
	sl, _, el, _ := s.GetNormalizedRange()
	return sl, el
}
