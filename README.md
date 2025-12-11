<div align="center">
    <img src="data/icons/hicolor/scalable/apps/org.frostyard.ChairLift.svg">
    <h1>Vanilla OS First Setup</h1>
    <p>This utility is meant to be used in <a href="https://github.com/vanilla-os">Vanilla OS</a>
    as a first-setup wizard. Its purpose is to help the user to configure the
    system to their needs, e.g. by configuring hostname, theme, flatpak apps, etc.</p>
    <hr />
    <a href="https://hosted.weblate.org/projects/vanilla-os/first-setup/#information">
<img src="https://hosted.weblate.org/widgets/vanilla-os/-/first-setup/svg-badge.svg" alt="Translation status" />
</a>
    <br />
    <img src="data/screenshots/welcome-page.png">
</div>

## Forked from Vanilla OS

https://github.com/Vanilla-OS/first-setup

## Run without building for Testing

> [!IMPORTANT]
> You need to install all build and run dependencies first

```bash
python3 test.py -d
```

The "-d" option is the dry-run mode, without it, first-setup will make changes to your system.

Pass the "-c" flag to force the configure system mode.

### Test translations:

You can change the used language like this:

```bash
LANGUAGE=de python3 test.py -d
```

## Build

### Installing build dependencies

```bash
sudo apt-get update
sudo apt-get build-dep .
```

If you want to install the build dependencies manually, have a look in:
[debian/control](https://github.com/Vanilla-OS/first-setup/blob/main/debian/control)

### Building

> [!WARNING]
> dpkg-buildpackage places it's output files (Like the .deb file) into the parent folder.

```bash
dpkg-buildpackage
```

or manually with meson:

```bash
meson setup build
meson compile -C build
```

Here you can change the install folder (default is /usr/local), for example:

```bash
meson setup --prefix="$(pwd)/install" build
```

## Modes

first-setup has multiple "modes" for running:

- OEM Mode: sets language, keyboard, timezone
- Configure Mode: set hostname, creates first user
- Regular Mode: runs on first login of created user, installs flatpaks, configures user-level things
- Install mode: when `snow-linux.live=1` is detected in the kernel command line, this triggers install mode.

## Installing first-setup

### Installing runtime dependencies

These can be found here:
[debian/control](https://github.com/Vanilla-OS/first-setup/blob/main/debian/control)

> [!TIP]
> If you use apt-get to install the .deb file it will automatically install the dependencies.

### Installing

```bash
sudo apt-get install ./chairlift*.deb
```

or manually with meson:

```bash
meson install -C build
```

## Run

### Creating initial user

A special user is needed to run the initial setup for hostname, user-creation, locale, etc.

1. Create a user
2. Create the group chairlift (Changing the gid is recommended to avoid messing with user groups)
3. Add the user to group chairlift
4. Create the file `/var/lib/AccountsService/users/your_user`

```ini
[User]
Session=ChairLift
```

5. Create the file `/etc/gdm3/daemon.conf` (replace your_user)

```ini
[daemon]
AutomaticLogin=your_user
AutomaticLoginEnable=True
```

> [!WARNING]
> All users in this group will be deleted on the first reboot after a successful first setup.

### Running

```bash
chairlift
```

#### Flags:

- `--dry-run (-d)`: Don't make any changes to the system.
- `--force-configure-mode (-c)`: Force the configure system mode, independant of group.
- `--force-regular-mode (-r)`: Force the regular mode, independant of group.
- `--oem-mode (-o)`: Use the original equipment manufacturer mode with language, keyboard and timezone selection.

## Update translation file

To update the .pot file with newly added translation strings, run:

```bash
meson compile -C build chairlift-pot
```

## Adjust for Custom Image

### Adjusting the scripts

The scripts which are used to modify the system can be found in `/usr/share/org.frostyard.ChairLift/chairlift/scripts/`.

Please adjust (overwrite) them to your needs in your image.

### Non-GNOME desktops

If you are using a different desktop than GNOME, you will have to adjust `/usr/share/xsessions/ChairLift.desktop` and `/usr/share/wayland-sessions/ChairLift.desktop`.

This session is only used to create a new user account. It should be a restricted shell to prevent the user from making changes to the system that will be lost when logging into their own user account.

If your Desktop doesn't offer this feature, just copy the session of your desktop to ChairLift.desktop.
