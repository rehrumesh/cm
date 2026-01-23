package logview

import "math"

// Layout represents the grid layout for panes
type Layout struct {
	Rows         int
	Cols         int
	PaneMap      [][]int   // maps grid positions to pane indices (-1 for empty)
	ColumnRatios []float64 // ratio of each column (sum = 1.0), nil means equal
	RowRatios    []float64 // ratio of each row (sum = 1.0), nil means equal
}

// MinPaneRatio is the minimum ratio a column or row can have
const MinPaneRatio = 0.1

// ResizeStep is the amount to change ratio per keyboard press
const ResizeStep = 0.05

// CalculateLayout determines the optimal grid layout for N panes
func CalculateLayout(numPanes int) Layout {
	if numPanes == 0 {
		return Layout{Rows: 0, Cols: 0}
	}

	if numPanes == 1 {
		return Layout{
			Rows:         1,
			Cols:         1,
			PaneMap:      [][]int{{0}},
			ColumnRatios: []float64{1.0},
			RowRatios:    []float64{1.0},
		}
	}

	// Calculate optimal grid dimensions
	// Goal: minimize empty cells while keeping aspect ratios reasonable
	cols := int(math.Ceil(math.Sqrt(float64(numPanes))))
	rows := int(math.Ceil(float64(numPanes) / float64(cols)))

	// Build pane map
	paneMap := make([][]int, rows)
	paneIdx := 0
	for r := 0; r < rows; r++ {
		paneMap[r] = make([]int, cols)
		for c := 0; c < cols; c++ {
			if paneIdx < numPanes {
				paneMap[r][c] = paneIdx
				paneIdx++
			} else {
				paneMap[r][c] = -1 // Empty cell
			}
		}
	}

	// Initialize equal ratios
	colRatios := make([]float64, cols)
	for i := range colRatios {
		colRatios[i] = 1.0 / float64(cols)
	}
	rowRatios := make([]float64, rows)
	for i := range rowRatios {
		rowRatios[i] = 1.0 / float64(rows)
	}

	return Layout{
		Rows:         rows,
		Cols:         cols,
		PaneMap:      paneMap,
		ColumnRatios: colRatios,
		RowRatios:    rowRatios,
	}
}

// EnsureRatios ensures ratios are initialized (for backwards compatibility)
func (l *Layout) EnsureRatios() {
	if l.Cols > 0 && (l.ColumnRatios == nil || len(l.ColumnRatios) != l.Cols) {
		l.ColumnRatios = make([]float64, l.Cols)
		for i := range l.ColumnRatios {
			l.ColumnRatios[i] = 1.0 / float64(l.Cols)
		}
	}
	if l.Rows > 0 && (l.RowRatios == nil || len(l.RowRatios) != l.Rows) {
		l.RowRatios = make([]float64, l.Rows)
		for i := range l.RowRatios {
			l.RowRatios[i] = 1.0 / float64(l.Rows)
		}
	}
}

// ResizeColumn adjusts the border between column col and col+1
// delta is positive to move right, negative to move left
func (l *Layout) ResizeColumn(col int, delta float64) bool {
	if col < 0 || col >= l.Cols-1 {
		return false
	}
	l.EnsureRatios()

	newLeft := l.ColumnRatios[col] + delta
	newRight := l.ColumnRatios[col+1] - delta

	// Enforce minimum ratios
	if newLeft < MinPaneRatio || newRight < MinPaneRatio {
		return false
	}

	l.ColumnRatios[col] = newLeft
	l.ColumnRatios[col+1] = newRight
	return true
}

// ResizeRow adjusts the border between row row and row+1
// delta is positive to move down, negative to move up
func (l *Layout) ResizeRow(row int, delta float64) bool {
	if row < 0 || row >= l.Rows-1 {
		return false
	}
	l.EnsureRatios()

	newTop := l.RowRatios[row] + delta
	newBottom := l.RowRatios[row+1] - delta

	// Enforce minimum ratios
	if newTop < MinPaneRatio || newBottom < MinPaneRatio {
		return false
	}

	l.RowRatios[row] = newTop
	l.RowRatios[row+1] = newBottom
	return true
}

// ResetRatios resets all ratios to equal distribution
func (l *Layout) ResetRatios() {
	if l.Cols > 0 {
		l.ColumnRatios = make([]float64, l.Cols)
		for i := range l.ColumnRatios {
			l.ColumnRatios[i] = 1.0 / float64(l.Cols)
		}
	}
	if l.Rows > 0 {
		l.RowRatios = make([]float64, l.Rows)
		for i := range l.RowRatios {
			l.RowRatios[i] = 1.0 / float64(l.Rows)
		}
	}
}

// GetColumnWidths calculates actual column widths from ratios
func (l *Layout) GetColumnWidths(totalWidth int) []int {
	l.EnsureRatios()
	widths := make([]int, l.Cols)
	remaining := totalWidth

	for i := 0; i < l.Cols-1; i++ {
		w := int(float64(totalWidth) * l.ColumnRatios[i])
		if w < 4 {
			w = 4
		}
		widths[i] = w
		remaining -= w
	}
	// Last column gets remaining to avoid rounding errors
	if l.Cols > 0 {
		if remaining < 4 {
			remaining = 4
		}
		widths[l.Cols-1] = remaining
	}
	return widths
}

// GetRowHeights calculates actual row heights from ratios
func (l *Layout) GetRowHeights(totalHeight int) []int {
	l.EnsureRatios()
	heights := make([]int, l.Rows)
	remaining := totalHeight

	for i := 0; i < l.Rows-1; i++ {
		h := int(float64(totalHeight) * l.RowRatios[i])
		if h < 3 {
			h = 3
		}
		heights[i] = h
		remaining -= h
	}
	// Last row gets remaining to avoid rounding errors
	if l.Rows > 0 {
		if remaining < 3 {
			remaining = 3
		}
		heights[l.Rows-1] = remaining
	}
	return heights
}

// GetColumnBorders returns x positions of vertical borders between columns
func (l *Layout) GetColumnBorders(totalWidth int) []int {
	widths := l.GetColumnWidths(totalWidth)
	borders := make([]int, l.Cols-1)
	x := 0
	for i := 0; i < l.Cols-1; i++ {
		x += widths[i]
		borders[i] = x
	}
	return borders
}

// GetRowBorders returns y positions of horizontal borders between rows
func (l *Layout) GetRowBorders(totalHeight int) []int {
	heights := l.GetRowHeights(totalHeight)
	borders := make([]int, l.Rows-1)
	y := 0
	for i := 0; i < l.Rows-1; i++ {
		y += heights[i]
		borders[i] = y
	}
	return borders
}

// PaneWidth calculates the default width for panes (used when ratios not set)
func (l Layout) PaneWidth(screenWidth int) int {
	if l.Cols == 0 {
		return screenWidth
	}
	width := screenWidth / l.Cols
	if width < 10 {
		width = 10 // Minimum width for usability
	}
	return width
}

// PaneHeight calculates the default height for panes (used when ratios not set)
func (l Layout) PaneHeight(screenHeight int) int {
	if l.Rows == 0 {
		return screenHeight
	}
	height := screenHeight / l.Rows
	if height < 5 {
		height = 5 // Minimum height for usability
	}
	return height
}
