package logview

import (
	"strings"
	"testing"
	"time"

	"cm/internal/docker"
)

func TestRenderLogsKeepsANSIInWordWrapMode(t *testing.T) {
	pane := NewPane(docker.Container{ID: "c1", Name: "test"}, 80, 12)

	pane.AddLogLine(docker.LogLine{
		ContainerID: "c1",
		Timestamp:   time.Now(),
		Stream:      "stdout",
		Content:     "\x1b[32mcolored output line that is long enough to wrap across rows in viewport\x1b[0m",
	})

	pane.SetWordWrap(true)

	rendered := pane.renderLogs()
	if !strings.Contains(rendered, "\x1b[32m") {
		t.Fatalf("expected wrapped render to preserve ANSI color sequence")
	}
}

func TestRenderLogsNoWrapStaysSingleLineAfterToggle(t *testing.T) {
	pane := NewPane(docker.Container{ID: "c1", Name: "test"}, 50, 12)

	pane.AddLogLine(docker.LogLine{
		ContainerID: "c1",
		Timestamp:   time.Now(),
		Stream:      "stdout",
		Content:     "\x1b[32m012345678901234567890123456789012345678901234567890123456789\x1b[0m",
	})

	pane.SetWordWrap(true)
	pane.SetWordWrap(false)

	rendered := strings.TrimSuffix(pane.renderLogs(), "\n")
	lines := strings.Split(rendered, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one rendered line in non-wrap mode, got %d", len(lines))
	}
}
