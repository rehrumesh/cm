package logview

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Bar chart colors
var (
	cpuBarColor      = lipgloss.Color("39")  // Cyan
	memBarColor      = lipgloss.Color("42")  // Green
	barEmptyColor    = lipgloss.Color("238") // Dark gray
	barThresholdColor = lipgloss.Color("196") // Red for 100% marker
	barLabelColor    = lipgloss.Color("252") // Light gray
	barValueColor    = lipgloss.Color("255") // White
)

// RenderBarCharts renders CPU and Memory as two horizontal bar charts side by side
func RenderBarCharts(cpuPercent, memPercent float64, memUsage, memLimit string, width, height int) string {
	// Split width between two charts with spacing
	chartWidth := (width - 6) / 2 // -6 for spacing and borders
	if chartWidth < 20 {
		chartWidth = 20
	}

	// Render both bars
	cpuBar := renderSingleBar("CPU", cpuPercent, "%", cpuBarColor, chartWidth, height)
	memBar := renderSingleBar("Memory", memPercent, fmt.Sprintf("%% (%s/%s)", memUsage, memLimit), memBarColor, chartWidth, height)

	// Join horizontally
	cpuLines := strings.Split(cpuBar, "\n")
	memLines := strings.Split(memBar, "\n")

	var result strings.Builder
	maxLines := len(cpuLines)
	if len(memLines) > maxLines {
		maxLines = len(memLines)
	}

	for i := 0; i < maxLines; i++ {
		cpuLine := ""
		memLine := ""
		if i < len(cpuLines) {
			cpuLine = cpuLines[i]
		}
		if i < len(memLines) {
			memLine = memLines[i]
		}

		// Pad CPU line
		cpuLineWidth := lipgloss.Width(cpuLine)
		if cpuLineWidth < chartWidth {
			cpuLine += strings.Repeat(" ", chartWidth-cpuLineWidth)
		}

		result.WriteString(cpuLine)
		result.WriteString("      ") // Spacing between charts
		result.WriteString(memLine)
		if i < maxLines-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
}

// renderSingleBar renders a single horizontal bar chart
func renderSingleBar(label string, value float64, suffix string, color lipgloss.Color, width, height int) string {
	// Determine the max value (nearest 100 above the value)
	maxValue := 100.0
	if value > 100 {
		maxValue = float64(((int(value) / 100) + 1) * 100)
	}

	// Calculate bar width (leave space for labels)
	barWidth := width - 4
	if barWidth < 10 {
		barWidth = 10
	}

	// Calculate filled portion
	fillRatio := value / maxValue
	if fillRatio > 1 {
		fillRatio = 1
	}
	if fillRatio < 0 {
		fillRatio = 0
	}
	filledWidth := int(float64(barWidth) * fillRatio)

	// Calculate 100% marker position
	thresholdPos := int(float64(barWidth) * (100.0 / maxValue))
	if thresholdPos > barWidth {
		thresholdPos = barWidth
	}

	// Styles
	labelStyle := lipgloss.NewStyle().Foreground(barLabelColor).Bold(true)
	valueStyle := lipgloss.NewStyle().Foreground(barValueColor).Bold(true)
	filledStyle := lipgloss.NewStyle().Foreground(color)
	emptyStyle := lipgloss.NewStyle().Foreground(barEmptyColor)
	thresholdStyle := lipgloss.NewStyle().Foreground(barThresholdColor).Bold(true)

	var lines []string

	// Title line with value
	titleLine := fmt.Sprintf("  %s: %s",
		labelStyle.Render(label),
		valueStyle.Render(fmt.Sprintf("%.1f%s", value, suffix)))
	lines = append(lines, titleLine)
	lines = append(lines, "") // Empty line

	// Build the bar
	var bar strings.Builder
	bar.WriteString("  ")

	for i := 0; i < barWidth; i++ {
		// Check if this is the 100% threshold position
		if i == thresholdPos && maxValue > 100 {
			bar.WriteString(thresholdStyle.Render("│"))
		} else if i < filledWidth {
			bar.WriteString(filledStyle.Render("█"))
		} else {
			bar.WriteString(emptyStyle.Render("░"))
		}
	}
	lines = append(lines, bar.String())

	// Scale line
	var scale strings.Builder
	scale.WriteString("  ")
	scale.WriteString(emptyStyle.Render("0"))

	// Add markers
	if maxValue > 100 {
		// Show 100% marker
		padding1 := thresholdPos - 1 - 1 // -1 for "0", -1 for the marker itself
		if padding1 < 0 {
			padding1 = 0
		}
		scale.WriteString(strings.Repeat(" ", padding1))
		scale.WriteString(thresholdStyle.Render("100%"))

		// Show max marker
		padding2 := barWidth - thresholdPos - 4 - 1 // -4 for "100%", -1 for spacing
		if padding2 < 0 {
			padding2 = 0
		}
		scale.WriteString(strings.Repeat(" ", padding2))
		scale.WriteString(emptyStyle.Render(fmt.Sprintf("%.0f%%", maxValue)))
	} else {
		// Just show 100% at the end
		padding := barWidth - 4 - 1 // -4 for "100%", -1 for "0"
		if padding < 0 {
			padding = 0
		}
		scale.WriteString(strings.Repeat(" ", padding))
		scale.WriteString(emptyStyle.Render("100%"))
	}
	lines = append(lines, scale.String())

	return strings.Join(lines, "\n")
}

// FormatBytes formats bytes into a human-readable string
func FormatBytes(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fG", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1fM", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1fK", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// FormatPercent formats a percentage with one decimal place
func FormatPercent(percent float64) string {
	return fmt.Sprintf("%.1f%%", percent)
}
