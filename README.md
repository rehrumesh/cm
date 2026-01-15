# cm - Container Monitor

A terminal UI application for monitoring Docker container logs in real-time with multi-pane support.

![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/License-MIT-blue.svg)

## Features

- **Container Discovery** - Automatically discovers running Docker containers grouped by Compose project
- **Multi-Select** - Select multiple containers to monitor simultaneously
- **Tiled Log View** - View logs from multiple containers in a responsive grid layout
- **Real-time Streaming** - Logs stream in real-time with automatic scrolling
- **Double-Click Maximize** - Double-click any pane to maximize/restore
- **Container Actions** - Restart, rebuild, or down/up containers directly from the UI
- **Compose Integration** - Full Docker Compose support with project grouping
- **Stopped Services** - Shows stopped Compose services that can be started

## Prerequisites

### Required

- **Go 1.24+** - [Download Go](https://go.dev/dl/)
- **Docker** - Docker daemon must be running and accessible
- **Docker Compose** - For Compose-related features (usually included with Docker Desktop)

### Optional

- **golangci-lint** - For running linters during development
  ```bash
  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
  ```

- **air** - For hot reloading during development
  ```bash
  go install github.com/air-verse/air@latest
  ```

## Installation

### Homebrew (Recommended)

```bash
brew tap rehrumesh/tap
brew install cm
```

#### Updating

```bash
brew upgrade cm
```

### From Source

```bash
# Clone the repository
git clone https://github.com/rehrumesh/cm.git
cd cm

# Install dependencies and build
make install
```

This installs `cm` to `~/.local/bin`.

### Add to PATH

Add the following to your shell profile (`~/.bashrc`, `~/.zshrc`, etc.):

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Then reload your shell:
```bash
source ~/.zshrc  # or ~/.bashrc
```

### Verify Installation

```bash
cm --version
```

## Building

### Build Commands

```bash
# Build for current platform (output: dist/cm)
make build

# Build for all platforms (Linux, macOS - amd64, arm64)
make build-all

# Build and run
make run

# Clean build artifacts
make clean
```

### Build Output

Binaries are output to the `dist/` directory:
```
dist/
├── cm                    # Current platform
├── cm-linux-amd64        # Linux x86_64
├── cm-linux-arm64        # Linux ARM64
├── cm-darwin-amd64       # macOS Intel
└── cm-darwin-arm64       # macOS Apple Silicon
```

## Development

### Setup

```bash
# Clone and enter directory
git clone https://github.com/rehrumesh/cm.git
cd cm

# Download dependencies
go mod download

# Verify everything works
make build
```

### Available Make Targets

| Target | Description |
|--------|-------------|
| `make build` | Build for current platform |
| `make build-all` | Cross-compile for all platforms |
| `make install` | Build and install to ~/.local/bin |
| `make uninstall` | Remove from ~/.local/bin |
| `make run` | Build and run |
| `make dev` | Run with hot reload (requires air) |
| `make test` | Run tests with race detection |
| `make coverage` | Run tests with coverage report |
| `make lint` | Run golangci-lint |
| `make fmt` | Format code with go fmt |
| `make vet` | Run go vet |
| `make tidy` | Tidy go.mod dependencies |
| `make clean` | Remove build artifacts |
| `make help` | Show all targets |

### Hot Reloading

For development with automatic rebuilds on file changes:

```bash
# Install air (one-time)
go install github.com/air-verse/air@latest

# Run with hot reload
make dev
```

Air watches for file changes and automatically rebuilds and restarts the app. Configuration is in `.air.toml`.

### Code Quality

```bash
# Format code
make fmt

# Run linter (requires golangci-lint)
make lint

# Run tests
make test

# Generate coverage report (opens dist/coverage.html)
make coverage
```

## Usage

### Basic Usage

```bash
# Launch container discovery UI
cm

# Start monitoring specific containers directly
cm api worker database

# Show version
cm --version
```

### Discovery Screen

Navigate the container list and select containers to monitor:

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate list |
| `Space` | Toggle selection |
| `Enter` | Confirm and start monitoring |
| `r` | Refresh container list |
| `q` | Quit |

### Log View

Monitor logs from selected containers:

| Key | Action |
|-----|--------|
| `↑` / `↓` / `j` / `k` | Scroll active pane |
| `Tab` | Cycle between panes |
| `1-9` | Jump to specific pane |
| `r` | Restart focused container |
| `R` | Compose down/up focused service |
| `B` | Build (no-cache) and up focused service |
| `Esc` | Return to container selection |
| `q` | Quit |

| Mouse | Action |
|-------|--------|
| Click | Focus pane |
| Double-click | Maximize/restore pane |
| Scroll | Scroll pane logs |

## Configuration

cm stores configuration in `~/.cm/` directory with three separate files:

| File | Purpose |
|------|---------|
| `config.json` | General settings (notifications) |
| `keybindings.json` | Customizable key bindings |
| `projects.json` | Saved compose projects (auto-populated) |

### config.json

```json
{
  "notifications": {
    "mode": "terminal",
    "toast_duration": 3,
    "toast_position": "bottom-right"
  }
}
```

### keybindings.json

```json
{
  "up": "up,k",
  "down": "down,j",
  "select": "space",
  "confirm": "enter",
  "quit": "q,ctrl+c",
  "refresh": "ctrl+r",
  "start": "u",
  "stop": "s",
  "restart": "r",
  "compose_build": "b",
  "saved_projects_key": "p",
  "config": "c"
}
```

### projects.json

Automatically populated when cm detects compose projects:

```json
{
  "saved_projects": {
    "myapp": {
      "config_file": "/path/to/docker-compose.yml",
      "working_dir": "/path/to/project"
    }
  }
}
```

## Project Structure

```
cm/
├── main.go                      # Entry point, CLI flags, help text
├── go.mod                       # Go module definition
├── go.sum                       # Dependency checksums
├── Makefile                     # Build automation
├── LICENSE                      # MIT License
├── README.md                    # This file
├── .gitignore                   # Git ignore rules
├── .golangci.yml                # Linter configuration
├── .air.toml                    # Hot reload configuration
├── dist/                        # Build output (git-ignored)
├── tmp/                         # Air temp directory (git-ignored)
└── internal/
    ├── config/
    │   └── config.go            # Configuration management (3 files)
    ├── docker/
    │   ├── client.go            # Docker client, compose actions
    │   ├── container.go         # Container types and grouping
    │   └── logs.go              # Log streaming
    ├── notify/
    │   └── notify.go            # Toast notifications
    └── ui/
        ├── app.go               # Root application model
        ├── common/
        │   ├── keys.go          # Key bindings
        │   ├── styles.go        # UI styles (Lip Gloss)
        │   ├── toast.go         # Toast notification component
        │   ├── configmodal.go   # Configuration modal
        │   └── savedprojects.go # Saved projects modal
        ├── discovery/
        │   └── model.go         # Container selection screen
        └── logview/
            ├── model.go         # Multi-pane log viewer
            ├── pane.go          # Individual log pane
            └── layout.go        # Pane layout calculations
```

## Dependencies

### Core Libraries

| Library | Purpose |
|---------|---------|
| [Bubble Tea](https://github.com/charmbracelet/bubbletea) | TUI framework (Elm architecture) |
| [Bubbles](https://github.com/charmbracelet/bubbles) | TUI components (viewport, etc.) |
| [Lip Gloss](https://github.com/charmbracelet/lipgloss) | Terminal styling and layouts |
| [Bubblezone](https://github.com/lrstanley/bubblezone) | Mouse click zone detection |
| [Docker SDK](https://github.com/docker/docker) | Docker API client |

### All Dependencies

Managed via Go modules. See `go.mod` for the complete list.

```bash
# Download all dependencies
go mod download

# Update dependencies
go get -u ./...
go mod tidy
```

## Troubleshooting

### "Docker daemon not reachable"

Ensure Docker is running:
```bash
docker ps
```

### "golangci-lint not installed"

Install it:
```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

Make sure `~/go/bin` is in your PATH:
```bash
export PATH="$HOME/go/bin:$PATH"
```

### App gets killed (OOM)

This was fixed in recent versions. If you experience this, update to the latest version and ensure you're not running an old build.

## Acknowledgments

This project was almost entirely built using [Claude Code](https://claude.ai/claude-code), Anthropic's AI coding assistant. From architecture decisions to bug fixes, Claude helped write the vast majority of this codebase.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
