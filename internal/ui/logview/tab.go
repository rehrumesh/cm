package logview

// TabType represents the different tab views in maximized mode
type TabType int

const (
	TabLogs TabType = iota
	TabStats
	TabEnv
	TabConfig
	TabTop
)

// TabNames contains the display names for each tab
var TabNames = []string{"Logs", "Stats", "Env", "Config", "Top"}

// String returns the display name for a tab
func (t TabType) String() string {
	if int(t) < len(TabNames) {
		return TabNames[t]
	}
	return "Unknown"
}

// TabCount returns the total number of tabs
func TabCount() int {
	return len(TabNames)
}
