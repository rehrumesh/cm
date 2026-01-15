package notify

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"cm/internal/config"
)

// Terminal detection constants
const (
	termITerm2  = "iterm2"
	termGhostty = "ghostty"
	termWarp    = "warp"
	termKitty   = "kitty"
	termUnknown = "unknown"
)

// notifier handles sending notifications based on config
type notifier struct {
	mode     config.NotificationMode
	terminal string
	tty      *os.File
}

var defaultNotifier *notifier

// Initialize sets up the notifier with config settings
// Should be called after config is loaded
func Initialize() {
	cfg, _ := config.Load()
	settings := config.DefaultNotificationSettings()
	if cfg != nil {
		settings = cfg.GetNotificationSettings()
	}

	defaultNotifier = &notifier{
		mode:     settings.Mode,
		terminal: detectTerminal(),
	}

	// Try to open /dev/tty for terminal notifications
	// This allows writing to terminal even when stdout is redirected
	if settings.Mode == config.NotifyTerminal {
		if tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
			defaultNotifier.tty = tty
		}
	}
}

// detectTerminal attempts to identify the current terminal emulator
func detectTerminal() string {
	// Check TERM_PROGRAM first (most reliable)
	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	switch {
	case strings.Contains(termProgram, "iterm"):
		return termITerm2
	case strings.Contains(termProgram, "ghostty"):
		return termGhostty
	case strings.Contains(termProgram, "warp"):
		return termWarp
	case strings.Contains(termProgram, "kitty"):
		return termKitty
	}

	// Check LC_TERMINAL (used by some terminals)
	lcTerminal := strings.ToLower(os.Getenv("LC_TERMINAL"))
	switch {
	case strings.Contains(lcTerminal, "iterm"):
		return termITerm2
	case strings.Contains(lcTerminal, "ghostty"):
		return termGhostty
	}

	// Check GHOSTTY_RESOURCES_DIR (Ghostty specific)
	if os.Getenv("GHOSTTY_RESOURCES_DIR") != "" {
		return termGhostty
	}

	// Check ITERM_SESSION_ID (iTerm2 specific)
	if os.Getenv("ITERM_SESSION_ID") != "" {
		return termITerm2
	}

	// Check KITTY_WINDOW_ID (Kitty specific)
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return termKitty
	}

	return termUnknown
}

// sendTerminalToast sends a notification using terminal escape sequences
func (n *notifier) sendTerminalToast(title, message string) bool {
	if n.tty == nil {
		return false
	}

	var sent bool

	switch n.terminal {
	case termITerm2:
		// iTerm2 supports OSC 9 for notifications
		// Format: \033]9;message\007
		fmt.Fprintf(n.tty, "\033]9;%s: %s\007", title, message)
		sent = true

	case termGhostty:
		// Ghostty supports OSC 9 (like iTerm2) and OSC 777
		// Try OSC 9 first
		fmt.Fprintf(n.tty, "\033]9;%s: %s\007", title, message)
		sent = true

	case termWarp:
		// Warp supports OSC 9
		fmt.Fprintf(n.tty, "\033]9;%s: %s\007", title, message)
		sent = true

	case termKitty:
		// Kitty supports OSC 99 for notifications
		// Format: \033]99;i=1:d=0;title\033\\message\033\\
		// Simpler format that also works: OSC 99 ; body ST
		fmt.Fprintf(n.tty, "\033]99;i=1:d=0:p=body;%s: %s\033\\", title, message)
		sent = true

	default:
		// Try common escape sequences for unknown terminals
		// OSC 9 is widely supported
		fmt.Fprintf(n.tty, "\033]9;%s: %s\007", title, message)
		// Also try OSC 777 (supported by some terminals)
		fmt.Fprintf(n.tty, "\033]777;notify;%s;%s\007", title, message)
		sent = true
	}

	return sent
}

// sendOSNotification sends a notification using OS-native mechanisms
func (n *notifier) sendOSNotification(title, message string) {
	switch runtime.GOOS {
	case "darwin":
		// macOS - use osascript for native notifications
		script := fmt.Sprintf(`display notification "%s" with title "%s"`,
			escapeAppleScript(message), escapeAppleScript(title))
		cmd := exec.Command("osascript", "-e", script)
		_ = cmd.Start()

	case "linux":
		// Linux - try notify-send (most common)
		cmd := exec.Command("notify-send", title, message)
		_ = cmd.Start()

	case "windows":
		// Windows - use PowerShell for toast notifications
		script := fmt.Sprintf(`
			[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
			$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
			$textNodes = $template.GetElementsByTagName("text")
			$textNodes.Item(0).AppendChild($template.CreateTextNode("%s")) | Out-Null
			$textNodes.Item(1).AppendChild($template.CreateTextNode("%s")) | Out-Null
			$toast = [Windows.UI.Notifications.ToastNotification]::new($template)
			[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("cm").Show($toast)
		`, escapePS(title), escapePS(message))
		cmd := exec.Command("powershell", "-Command", script)
		_ = cmd.Start()
	}
}

// escapeAppleScript escapes special characters for AppleScript strings
func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

// escapePS escapes special characters for PowerShell strings
func escapePS(s string) string {
	s = strings.ReplaceAll(s, "`", "``")
	s = strings.ReplaceAll(s, "\"", "`\"")
	return s
}

// Toast sends a notification using the configured method
func Toast(title, message string) {
	if defaultNotifier == nil {
		Initialize()
	}

	switch defaultNotifier.mode {
	case config.NotifyTerminal:
		// Try terminal toast first, fallback to OS if it fails
		if !defaultNotifier.sendTerminalToast(title, message) {
			defaultNotifier.sendOSNotification(title, message)
		}

	case config.NotifyOS:
		defaultNotifier.sendOSNotification(title, message)

	case config.NotifyNone:
		// Notifications disabled
		return
	}
}

// Success sends a success notification
func Success(message string) {
	Toast("cm", "✓ "+message)
}

// Error sends an error notification
func Error(message string) {
	Toast("cm", "✗ "+message)
}

// Info sends an info notification
func Info(message string) {
	Toast("cm", message)
}

// Close cleans up resources
func Close() {
	if defaultNotifier != nil && defaultNotifier.tty != nil {
		defaultNotifier.tty.Close()
		defaultNotifier.tty = nil
	}
}

// GetTerminal returns the detected terminal name (for debugging)
func GetTerminal() string {
	if defaultNotifier == nil {
		return detectTerminal()
	}
	return defaultNotifier.terminal
}
