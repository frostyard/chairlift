# ChairLift (Go/puregotk version)

> **Historical document.** This file records the original Go-port proposal and
> is not current build or installation guidance. ChairLift is now wholly Go
> and puregotk-based. Use [README.md](README.md), [CONFIG.md](CONFIG.md), and
> [docs/index.md](docs/index.md) for the maintained instructions.

A modern GTK4/Libadwaita system management tool written in Go using [puregotk](https://codeberg.org/puregotk/puregotk) bindings.

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

- See [README.md](README.md) for the current Go version requirement
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

> Do not use build commands from this historical proposal. See
> [README.md](README.md#building-from-source) for the maintained instructions.

## Project Structure

The port is complete, and the historical layout described by this proposal no
longer exists. See [README.md](README.md) for the current repository layout.

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

GPL-3.0 - see [LICENSE](LICENSE)
