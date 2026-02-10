package common

import (
	"fmt"
	"strings"

	"cm/internal/docker"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// InspectModalClosedMsg is sent when the inspect modal is closed
type InspectModalClosedMsg struct{}

// ContainerDetailsMsg is sent when container details are fetched
type ContainerDetailsMsg struct {
	Details *docker.ContainerDetails
	Err     error
}

// InspectModal represents the container inspect modal
type InspectModal struct {
	visible     bool
	width       int
	height      int
	details     *docker.ContainerDetails
	loading     bool
	err         error
	viewport    viewport.Model
	containerID string
}

// NewInspectModal creates a new inspect modal
func NewInspectModal() InspectModal {
	return InspectModal{
		visible: false,
	}
}

// Open opens the modal for a container
func (m *InspectModal) Open(containerID string) tea.Cmd {
	m.visible = true
	m.loading = true
	m.details = nil
	m.err = nil
	m.containerID = containerID
	m.viewport = viewport.New(60, 20)
	return nil
}

// SetDetails sets the container details
func (m *InspectModal) SetDetails(details *docker.ContainerDetails, err error) {
	m.loading = false
	m.details = details
	m.err = err
	if details != nil {
		m.viewport.SetContent(m.renderDetails())
	}
}

// Close closes the modal
func (m *InspectModal) Close() {
	m.visible = false
	m.details = nil
	m.loading = false
	m.err = nil
}

// IsVisible returns whether the modal is visible
func (m InspectModal) IsVisible() bool {
	return m.visible
}

// SetSize sets the modal dimensions
func (m *InspectModal) SetSize(width, height int) {
	m.width = width
	m.height = height
	// Update viewport size
	vpWidth := 60
	vpHeight := 20
	if width > 0 && height > 0 {
		vpWidth = width - 20
		if vpWidth > 80 {
			vpWidth = 80
		}
		if vpWidth < 40 {
			vpWidth = 40
		}
		vpHeight = height - 12
		if vpHeight > 30 {
			vpHeight = 30
		}
		if vpHeight < 10 {
			vpHeight = 10
		}
	}
	m.viewport.Width = vpWidth
	m.viewport.Height = vpHeight
}

// Update handles messages for the modal
func (m InspectModal) Update(msg tea.Msg) (InspectModal, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "i", "q"))):
			m.visible = false
			return m, func() tea.Msg { return InspectModalClosedMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			m.viewport.SetYOffset(m.viewport.YOffset - 1)

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			m.viewport.SetYOffset(m.viewport.YOffset + 1)

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+u"))):
			m.viewport.SetYOffset(m.viewport.YOffset - 5)

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+d"))):
			m.viewport.SetYOffset(m.viewport.YOffset + 5)
		}
	}

	return m, nil
}

// renderDetails renders the container details as a string
func (m *InspectModal) renderDetails() string {
	if m.details == nil {
		return ""
	}

	d := m.details
	var b strings.Builder

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))

	// Basic info
	b.WriteString(sectionStyle.Render("Container Info"))
	b.WriteString("\n\n")

	writeField := func(label, value string) {
		if value != "" {
			b.WriteString(labelStyle.Render(fmt.Sprintf("  %-14s", label+":")))
			b.WriteString(valueStyle.Render(value))
			b.WriteString("\n")
		}
	}

	writeField("ID", d.ID)
	writeField("Name", d.Name)
	writeField("Image", d.Image)
	writeField("Status", d.Status)
	if !d.Created.IsZero() {
		writeField("Created", d.Created.Format("2006-01-02 15:04:05"))
	}
	if !d.Started.IsZero() {
		writeField("Started", d.Started.Format("2006-01-02 15:04:05"))
	}
	writeField("Restart", d.RestartPolicy)

	// Command
	if d.Entrypoint != "" || d.Command != "" {
		b.WriteString("\n")
		b.WriteString(sectionStyle.Render("Command"))
		b.WriteString("\n\n")
		if d.Entrypoint != "" {
			writeField("Entrypoint", d.Entrypoint)
		}
		if d.Command != "" {
			writeField("Cmd", d.Command)
		}
		if d.WorkingDir != "" {
			writeField("WorkDir", d.WorkingDir)
		}
	}

	// Ports
	if len(d.Ports) > 0 {
		b.WriteString("\n")
		b.WriteString(sectionStyle.Render("Ports"))
		b.WriteString("\n\n")
		for _, port := range d.Ports {
			b.WriteString(valueStyle.Render("  " + port))
			b.WriteString("\n")
		}
	}

	// Networks
	if len(d.Networks) > 0 {
		b.WriteString("\n")
		b.WriteString(sectionStyle.Render("Networks"))
		b.WriteString("\n\n")
		for _, net := range d.Networks {
			b.WriteString(valueStyle.Render("  " + net))
			b.WriteString("\n")
		}
	}

	// Volumes
	if len(d.Volumes) > 0 {
		b.WriteString("\n")
		b.WriteString(sectionStyle.Render("Volumes"))
		b.WriteString("\n\n")
		for _, vol := range d.Volumes {
			// Truncate long paths
			if len(vol) > 60 {
				vol = vol[:57] + "..."
			}
			b.WriteString(valueStyle.Render("  " + vol))
			b.WriteString("\n")
		}
	}

	// Environment variables
	if len(d.Env) > 0 {
		b.WriteString("\n")
		b.WriteString(sectionStyle.Render("Environment"))
		b.WriteString("\n\n")
		for _, env := range d.Env {
			// Truncate long values
			if len(env) > 60 {
				env = env[:57] + "..."
			}
			b.WriteString(valueStyle.Render("  " + env))
			b.WriteString("\n")
		}
	}

	// Labels (compose-related only)
	if len(d.Labels) > 0 {
		var composeLabels []string
		for k, v := range d.Labels {
			if strings.HasPrefix(k, "com.docker.compose") {
				label := strings.TrimPrefix(k, "com.docker.compose.")
				composeLabels = append(composeLabels, label+": "+v)
			}
		}
		if len(composeLabels) > 0 {
			b.WriteString("\n")
			b.WriteString(sectionStyle.Render("Compose Labels"))
			b.WriteString("\n\n")
			for _, label := range composeLabels {
				if len(label) > 60 {
					label = label[:57] + "..."
				}
				b.WriteString(valueStyle.Render("  " + label))
				b.WriteString("\n")
			}
		}
	}

	return b.String()
}

// View renders the modal
func (m InspectModal) View(screenWidth, screenHeight int) string {
	if !m.visible {
		return ""
	}

	var content strings.Builder

	// Title
	content.WriteString(ModalTitleStyle.Render("Container Details"))
	content.WriteString("\n\n")

	if m.loading {
		content.WriteString(MutedInlineStyle.Render("  Loading..."))
	} else if m.err != nil {
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("  Error: " + m.err.Error()))
	} else if m.details != nil {
		content.WriteString(m.viewport.View())
	}

	content.WriteString("\n\n")

	// Scroll indicator
	if m.details != nil && m.viewport.TotalLineCount() > m.viewport.Height {
		content.WriteString(MutedInlineStyle.Render("  j/k: scroll  "))
	}
	content.WriteString(MutedInlineStyle.Render("esc/i/q: close"))

	// Style the modal (no background fill; border-only overlay)
	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primaryColor).
		Padding(1, 2)
	modalContent := modalStyle.Render(content.String())

	// Get modal dimensions
	modalWidth := lipgloss.Width(modalContent)
	modalHeight := lipgloss.Height(modalContent)

	// Center the modal
	x := (screenWidth - modalWidth) / 2
	y := (screenHeight - modalHeight) / 2

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	// Create positioned modal
	positioned := lipgloss.NewStyle().
		MarginLeft(x).
		MarginTop(y).
		Render(modalContent)

	return positioned
}
