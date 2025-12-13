<div align="center">
    <img src="data/icons/hicolor/scalable/apps/org.frostyard.ChairLift.svg">
    <h1>ChairLift</h1>
    <p>A modern system management tool for <a href="https://github.com/frostyard/snow">Snow Linux</a></p>
    <p>Manage your Homebrew packages, monitor system health, and maintain your system with ease.</p>
</div>

---

## Screenshots

![ChairLift Home Page](data/screenshots/home-page.png)

---

## Features

### 📦 Homebrew Package Management
- **View Installed Packages**: Browse all installed formulae and casks in organized expandable lists
- **Search & Install**: Search the Homebrew repository and install packages with one click
- **Update & Upgrade**: Keep Homebrew up-to-date and upgrade outdated packages individually
- **Pin Packages**: Pin packages to prevent accidental upgrades
- **Curated Bundles**: Install pre-configured package bundles for common use cases

### 🏥 System Health Monitoring
- **System Performance**: Quick access to Mission Center for detailed system monitoring
- **Health Overview**: Check system diagnostics and health status

### 🔧 Updates & Maintenance
- **Homebrew Updates**: Check for and install package updates
- **Outdated Packages**: View and upgrade packages that have newer versions available
- **System Maintenance**: Keep your system running smoothly

---

## Installation

### Building from Source

ChairLift uses the Meson build system:

```bash
# Clone the repository
git clone https://github.com/frostyard/chairlift.git
cd chairlift

# Build and install
meson setup build
meson compile -C build
sudo meson install -C build
```

### Dependencies

- Python 3.x
- GTK4
- libadwaita
- Homebrew (for package management features)
- Mission Center (optional, for system performance monitoring)

---

## Usage

Launch ChairLift from your application menu or run:

```bash
chairlift
```

### Main Sections

1. **System**: Monitor system health and performance
2. **Updates**: Manage Homebrew updates and outdated packages
3. **Applications**: View installed packages, search for new ones, and install curated bundles
4. **Maintenance**: System maintenance tools (coming soon)
5. **Help**: Documentation and support resources (coming soon)

### Managing Packages

- **Browse Installed**: Navigate to Applications → Brew Packages to see all installed formulae and casks
- **Search**: Use the search box to find packages by name or keyword
- **Install**: Click the install button next to search results or bundle items
- **Pin/Unpin**: Click the pin icon to lock/unlock a package version
- **Remove**: Click the trash icon to uninstall a package
- **Upgrade**: Click upgrade button next to outdated packages

### Bundle Installation

ChairLift supports installing curated package bundles (Brewfiles) located in `/usr/share/snow/bundles`. Each bundle is a pre-configured set of packages for specific use cases.

---

## Development

### Project Structure

```
chairlift/
├── chairlift/           # Main application code
│   ├── core/           # Core functionality (homebrew.py)
│   ├── views/          # UI views (user_home.py)
│   ├── gtk/            # GTK UI templates
│   └── assets/         # Application assets
├── data/               # Desktop files and icons
├── po/                 # Translation files
└── meson.build         # Build configuration
```

### Key Components

- **chairlift/core/homebrew.py**: Python library for Homebrew integration
  - Package listing and searching
  - Installation and removal
  - Pin/unpin functionality
  - Bundle management
  - Update and upgrade operations

- **chairlift/views/user_home.py**: Main UI implementation
  - GTK4/Adwaita interface
  - Async operations with threading
  - Toast notifications for user feedback

### Contributing

Contributions are welcome! Please feel free to submit issues and pull requests.

---

## Credits

ChairLift is adapted from [Vanilla OS First Setup](https://github.com/Vanilla-OS/first-setup).

### License

This program is free software: you can redistribute it and/or modify it under the terms of the GNU General Public License as published by the Free Software Foundation, version 3.

See [LICENSE](LICENSE) for details.

---

<div align="center">
    <p>Made with ❤️ for Snow Linux</p>
</div>
