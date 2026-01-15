package logview

import "math"

// Layout represents the grid layout for panes
type Layout struct {
	Rows    int
	Cols    int
	PaneMap [][]int // maps grid positions to pane indices (-1 for empty)
}

// CalculateLayout determines the optimal grid layout for N panes
func CalculateLayout(numPanes int) Layout {
	if numPanes == 0 {
		return Layout{Rows: 0, Cols: 0}
	}

	if numPanes == 1 {
		return Layout{
			Rows:    1,
			Cols:    1,
			PaneMap: [][]int{{0}},
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

	return Layout{
		Rows:    rows,
		Cols:    cols,
		PaneMap: paneMap,
	}
}

// PaneWidth calculates the width for each pane given screen width
func (l Layout) PaneWidth(screenWidth int) int {
	if l.Cols == 0 {
		return screenWidth
	}
	return screenWidth / l.Cols
}

// PaneHeight calculates the height for each pane given screen height
func (l Layout) PaneHeight(screenHeight int) int {
	if l.Rows == 0 {
		return screenHeight
	}
	return screenHeight / l.Rows
}
