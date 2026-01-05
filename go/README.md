# ChairLift (Go/puregotk version)

A modern GTK4/Libadwaita system management tool written in Go using [puregotk](https://github.com/jwijenbergh/puregotk) bindings.

## Features

- **Homebrew package management** - Install, update, and manage Homebrew formulae and casks
- **System health monitoring** - View system information and health status
- **System updates** - Check for and install system updates
- **Maintenance tools** - System cleanup and optimization utilities
- **Configuration-driven UI** - YAML-based configuration for portability

## Why puregotk?

This is an experimental port of ChairLift from Python to Go using puregotk. Benefits include:

- **No CGO required** - Pure Go implementation using [purego](https://github.com/ebitengine/purego)
- **Fast compilation** - ~40 seconds vs 15+ minutes with CGO-based GTK bindings
- **Easy cross-compilation** - No C toolchain needed
- **Single binary** - Deploy as a single executable

## Requirements

- Go 1.22 or later
- GTK 4
- libadwaita 1
- Homebrew (optional, for package management features)

### Installing GTK4 and libadwaita

**Debian/Ubuntu:**

```bash
sudo apt install libgtk-4-dev libadwaita-1-dev
```

**Fedora:**

```bash
sudo dnf install gtk4-devel libadwaita-devel
```

**Arch Linux:**

```bash
sudo pacman -S gtk4 libadwaita
```

## Building

```bash
cd go

# Download dependencies
make deps

# Build the application
make build

# Run the application
make run
```

Or manually:

```bash
cd go
go mod download
CGO_ENABLED=0 go build -o build/chairlift ./cmd/chairlift
./build/chairlift
```

## Project Structure

```
go/
├── cmd/
│   └── chairlift/
│       └── main.go          # Application entry point
├── internal/
│   ├── app/
│   │   └── app.go           # Application setup
│   ├── config/
│   │   └── config.go        # YAML configuration loading
│   ├── homebrew/
│   │   └── homebrew.go      # Homebrew interface
│   ├── views/
│   │   └── userhome.go      # Page content views
│   └── window/
│       └── window.go        # Main window
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Configuration

ChairLift uses a YAML configuration file to control UI behavior. The config file is searched in these locations (in order):

1. `/etc/chairlift/config.yml` (system-wide)
2. `/usr/share/chairlift/config.yml` (package maintainer defaults)
3. `config.yml` (current directory)

See [config.yml](config.yml) for the default configuration.

## Differences from Python version

- Uses Go's goroutines instead of Python threading
- Uses `glib.IdleAdd()` for UI updates from background threads
- No Python dependencies required
- Single statically-linked binary output
- Configuration is embedded in code as fallback defaults

## Known Limitations

puregotk is experimental and some APIs may not work correctly. Known issues:

- Some GTK4 APIs that use struct arguments (not pointers) may not work
- Widget memory management requires explicit `Unref()` calls
- Signal callbacks use function pointers for internal re-use

## License

GPL-3.0 - see [LICENSE](../LICENSE)
