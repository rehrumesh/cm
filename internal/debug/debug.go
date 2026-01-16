package debug

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	enabled bool
	mu      sync.RWMutex
	logFile *os.File
	logPath string
)

// DefaultLogPath returns the default debug log path
func DefaultLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/cm_debug.log"
	}
	return filepath.Join(home, ".cm", "debug.log")
}

// Init initializes debug logging with the given state
func Init(enable bool) {
	mu.Lock()
	defer mu.Unlock()
	enabled = enable
	if enable {
		initLogFile()
	}
}

// initLogFile creates/opens the log file
func initLogFile() {
	if logFile != nil {
		return
	}
	logPath = DefaultLogPath()

	// Ensure directory exists
	dir := filepath.Dir(logPath)
	os.MkdirAll(dir, 0755)

	// Rotate log if it's too big (> 10MB)
	if info, err := os.Stat(logPath); err == nil && info.Size() > 10*1024*1024 {
		os.Rename(logPath, logPath+".old")
	}

	var err error
	logFile, err = os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logFile = nil
		return
	}

	// Write session start marker
	fmt.Fprintf(logFile, "\n=== Debug session started at %s ===\n", time.Now().Format(time.RFC3339))
}

// Enable turns on debug logging
func Enable() {
	mu.Lock()
	defer mu.Unlock()
	if enabled {
		return
	}
	enabled = true
	initLogFile()
	// Write directly to avoid deadlock (Log() would try to acquire RLock)
	if logFile != nil {
		fmt.Fprintf(logFile, "[%s] Debug logging enabled\n", time.Now().Format("15:04:05.000"))
	}
}

// Disable turns off debug logging
func Disable() {
	mu.Lock()
	defer mu.Unlock()
	if !enabled {
		return
	}
	// Write directly to avoid deadlock (Log() would try to acquire RLock)
	if logFile != nil {
		fmt.Fprintf(logFile, "[%s] Debug logging disabled\n", time.Now().Format("15:04:05.000"))
	}
	enabled = false
}

// Toggle switches the debug logging state and returns the new state
func Toggle() bool {
	mu.Lock()
	defer mu.Unlock()
	enabled = !enabled
	if enabled {
		initLogFile()
		// Write directly to avoid deadlock (Log() would try to acquire RLock)
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] Debug logging enabled via toggle\n", time.Now().Format("15:04:05.000"))
		}
	} else if logFile != nil {
		fmt.Fprintf(logFile, "[%s] Debug logging disabled via toggle\n", time.Now().Format("15:04:05.000"))
	}
	return enabled
}

// IsEnabled returns whether debug logging is enabled
func IsEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return enabled
}

// Log writes a debug message if debug mode is enabled
func Log(format string, args ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()

	if !enabled || logFile == nil {
		return
	}

	timestamp := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(logFile, "[%s] %s\n", timestamp, msg)
}

// LogPath returns the current log file path
func LogPath() string {
	return logPath
}

// Close closes the log file
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
}
