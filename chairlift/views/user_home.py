# user_home.py
#
# Copyright 2024 mirkobrombin
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundationat version 3 of the License.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

from gi.repository import Gtk, Adw, Gio, GLib
from chairlift.core import homebrew

_ = __builtins__["_"]


@Gtk.Template(resource_path="/org/frostyard/ChairLift/gtk/user-home.ui")
class ChairLiftUserHome(Adw.Bin):
    __gtype_name__ = "ChairLiftUserHome"

    view_stack = Gtk.Template.Child()
    view_switcher_title = Gtk.Template.Child()
    view_switcher_bar = Gtk.Template.Child()
    system_page = Gtk.Template.Child()
    updates_page = Gtk.Template.Child()
    applications_page = Gtk.Template.Child()
    maintenance_page = Gtk.Template.Child()
    help_page = Gtk.Template.Child()

    def __init__(self, window, **kwargs):
        super().__init__(**kwargs)
        self.__window = window

        # Bind the view stack to the switchers
        self.view_switcher_title.set_stack(self.view_stack)
        self.view_switcher_bar.set_stack(self.view_stack)

        # Build the preference groups dynamically
        self.__build_system_page()
        self.__build_updates_page()
        self.__build_applications_page()
        self.__build_maintenance_page()
        self.__build_help_page()

    def __build_system_page(self):
        """Build the System tab preference groups"""
        # System Information group
        system_info_group = Adw.PreferencesGroup()
        system_info_group.set_title(_("System Information"))
        system_info_group.set_description(_("View system details and hardware information"))

        # Create an expander row for OS release information
        os_expander = Adw.ExpanderRow()
        os_expander.set_title(_("Operating System Details"))

        # Read /etc/os-release and dynamically add rows to the expander
        try:
            with open('/etc/os-release', 'r') as f:
                for line in f:
                    line = line.strip()
                    if line and '=' in line and not line.startswith('#'):
                        key, value = line.split('=', 1)
                        # Remove quotes from value
                        value = value.strip('"').strip("'")
                        # Convert key to readable format (e.g., PRETTY_NAME -> Pretty Name)
                        readable_key = key.replace('_', ' ').title().replace('Url', 'URL').replace('Id', 'ID')
                        row = Adw.ActionRow(title=readable_key, subtitle=value)

                        # Make URL rows clickable
                        if key.endswith('URL'):
                            row.set_activatable(True)
                            row.add_suffix(Gtk.Image.new_from_icon_name("adw-external-link-symbolic"))
                            row.connect("activated", self.__on_url_row_activated, value)

                        os_expander.add_row(row)
        except FileNotFoundError:
            # Fallback if /etc/os-release doesn't exist
            row = Adw.ActionRow(title=_("OS Information"), subtitle=_("Not available"))
            os_expander.add_row(row)
        except Exception as e:
            row = Adw.ActionRow(title=_("Error"), subtitle=str(e))
            os_expander.add_row(row)

        system_info_group.add(os_expander)
        self.system_page.add(system_info_group)


    def __build_updates_page(self):
        """Build the Updates tab preference groups"""
        # System Updates group
        updates_status_group = Adw.PreferencesGroup()
        updates_status_group.set_title(_("System Updates"))
        updates_status_group.set_description(_("Check for and install system updates"))
        self.updates_page.add(updates_status_group)

        # Brew Updates group
        brew_updates_group = Adw.PreferencesGroup()
        brew_updates_group.set_title(_("Homebrew Updates"))
        brew_updates_group.set_description(_("Check for and install Homebrew package updates"))
        self.updates_page.add(brew_updates_group)

        # Add update Homebrew button
        update_brew_row = Adw.ActionRow()
        update_brew_row.set_title(_("Update Homebrew"))
        update_brew_row.set_subtitle(_("Update Homebrew itself and all formulae definitions"))
        
        update_button = Gtk.Button()
        update_button.set_label(_("Update"))
        update_button.set_valign(Gtk.Align.CENTER)
        update_button.add_css_class("suggested-action")
        update_button.connect("clicked", self.__on_update_homebrew_clicked)
        
        update_brew_row.add_suffix(update_button)
        update_brew_row.set_activatable_widget(update_button)
        brew_updates_group.add(update_brew_row)

        # Add outdated packages expander
        outdated_expander = Adw.ExpanderRow()
        outdated_expander.set_title(_("Outdated Packages"))
        outdated_expander.set_subtitle(_("Loading..."))
        brew_updates_group.add(outdated_expander)

        # Load outdated packages asynchronously
        self.__load_outdated_packages(outdated_expander)

        # Update Settings group
        updates_settings_group = Adw.PreferencesGroup()
        updates_settings_group.set_title(_("Update Settings"))
        updates_settings_group.set_description(_("Configure update preferences"))
        self.updates_page.add(updates_settings_group)

    def __build_applications_page(self):
        """Build the Applications tab preference groups"""
        # Installed Applications group
        applications_installed_group = Adw.PreferencesGroup()
        applications_installed_group.set_title(_("Installed Applications"))
        applications_installed_group.set_description(_("Manage your installed applications"))
        self.applications_page.add(applications_installed_group)
        view_apps = Adw.ActionRow(
            title=_("Manage Flatpaks"),
            subtitle=_("Open the application manager to install and manage applications")
        )
        view_apps.set_activatable(True)
        view_apps.add_suffix(Gtk.Image.new_from_icon_name("adw-external-link-symbolic"))
        view_apps.connect("activated", self.__on_launch_app_row_activated, "io.github.kolunmi.Bazaar")
        applications_installed_group.add(view_apps)

        # Brew group
        brew_group = Adw.PreferencesGroup()
        brew_group.set_title(_("Homebrew"))
        brew_group.set_description(_("Manage Homebrew packages installed on your system"))
        self.applications_page.add(brew_group)

        # Create expander row for Homebrew formulae
        formulae_expander = Adw.ExpanderRow()
        formulae_expander.set_title(_("Formulae"))
        formulae_expander.set_subtitle(_("Loading..."))
        brew_group.add(formulae_expander)

        # Create expander row for Homebrew casks
        casks_expander = Adw.ExpanderRow()
        casks_expander.set_title(_("Casks"))
        casks_expander.set_subtitle(_("Loading..."))
        brew_group.add(casks_expander)

        # Load Homebrew packages asynchronously
        self.__load_homebrew_packages(formulae_expander, casks_expander)

        # Application Sources group
        applications_sources_group = Adw.PreferencesGroup()
        applications_sources_group.set_title(_("Preconfigured Bundles"))
        applications_sources_group.set_description(_("Install and manage preconfigured application bundles"))
        self.applications_page.add(applications_sources_group)

    def __build_maintenance_page(self):
        """Build the Maintenance tab preference groups"""
        # System Cleanup group
        maintenance_cleanup_group = Adw.PreferencesGroup()
        maintenance_cleanup_group.set_title(_("System Cleanup"))
        maintenance_cleanup_group.set_description(_("Clean up temporary files and free up disk space"))
        self.maintenance_page.add(maintenance_cleanup_group)

        # System Optimization group
        maintenance_optimization_group = Adw.PreferencesGroup()
        maintenance_optimization_group.set_title(_("System Optimization"))
        maintenance_optimization_group.set_description(_("Optimize system performance"))
        self.maintenance_page.add(maintenance_optimization_group)

    def __build_help_page(self):
        """Build the Help tab preference groups"""
        # Help Resources group
        help_resources_group = Adw.PreferencesGroup()
        help_resources_group.set_title(_("Help Resources"))
        help_resources_group.set_description(_("Access help and support resources"))
        self.help_page.add(help_resources_group)

    def __on_url_row_activated(self, row, url):
        """Open URL in default browser when a URL row is clicked"""
        try:
            Gio.AppInfo.launch_default_for_uri(url, None)
        except Exception as e:
            print(f"Error opening URL: {e}")

    def __on_launch_app_row_activated(self, row, app_id):
        """Launch a Flatpak application when a row is clicked"""
        try:
            app_info = Gio.DesktopAppInfo.new(f"{app_id}.desktop")
            if app_info:
                app_info.launch([], None)
            else:
                # If desktop file not found, try launching via the app ID directly
                Gio.AppInfo.launch_default_for_uri(f"appstream://{app_id}", None)
        except Exception as e:
            print(f"Error launching application {app_id}: {e}")
            # Fallback: try to open in software center
            try:
                Gio.AppInfo.launch_default_for_uri(f"appstream://{app_id}", None)
            except Exception as e2:
                print(f"Error opening in software center: {e2}")

    def __on_update_homebrew_clicked(self, button):
        """Handle Homebrew update button click"""
        # Disable button and show loading state
        button.set_sensitive(False)
        original_label = button.get_label()
        button.set_label(_("Updating..."))
        
        def update_in_thread():
            """Update Homebrew in a background thread"""
            try:
                if not homebrew.is_homebrew_installed():
                    return {
                        'success': False,
                        'message': _("Homebrew is not installed")
                    }
                
                homebrew.update_homebrew()
                return {
                    'success': True,
                    'message': _("Homebrew updated successfully")
                }
            except homebrew.HomebrewError as e:
                return {
                    'success': False,
                    'message': str(e)
                }
            except Exception as e:
                return {
                    'success': False,
                    'message': _("Failed to update Homebrew: {}").format(str(e))
                }
        
        def on_update_complete(result):
            """Handle update completion on main thread"""
            button.set_sensitive(True)
            button.set_label(original_label)
            
            # Show a toast notification
            if hasattr(self.__window, 'add_toast'):
                toast = Adw.Toast.new(result['message'])
                toast.set_timeout(3)
                self.__window.add_toast(toast)
            else:
                print(result['message'])
        
        # Run update in background thread
        import threading
        def run():
            result = update_in_thread()
            GLib.idle_add(lambda: on_update_complete(result))
        
        thread = threading.Thread(target=run, daemon=True)
        thread.start()

    def __load_outdated_packages(self, expander):
        """Load outdated Homebrew packages and populate the expander row"""
        def load_in_thread():
            """Load outdated packages in a background thread"""
            try:
                if not homebrew.is_homebrew_installed():
                    return {
                        'error': True,
                        'message': _("Homebrew is not installed")
                    }

                outdated = homebrew.list_outdated_packages(formula_only=False)
                return {
                    'error': False,
                    'packages': outdated
                }
            except homebrew.HomebrewError as e:
                return {
                    'error': True,
                    'message': str(e)
                }
            except Exception as e:
                return {
                    'error': True,
                    'message': _("Failed to load outdated packages: {}").format(str(e))
                }

        def on_packages_loaded(result):
            """Update UI with loaded packages on the main thread"""
            if result['error']:
                error_row = Adw.ActionRow(
                    title=_("Error"),
                    subtitle=result['message']
                )
                error_row.add_prefix(Gtk.Image.new_from_icon_name("dialog-error-symbolic"))
                expander.add_row(error_row)
                expander.set_subtitle(_("Failed to load"))
                return

            packages = result['packages']
            
            # Store rows for later removal
            if not hasattr(expander, 'package_rows'):
                expander.package_rows = []
            
            if packages:
                expander.set_subtitle(_("{} packages need updates").format(len(packages)))
                
                for pkg in sorted(packages, key=lambda x: x['name']):
                    pkg_row = Adw.ActionRow(
                        title=pkg['name'],
                        subtitle=_("{} → {}").format(pkg['current_version'], pkg['latest_version'])
                    )
                    
                    # Add pinned indicator
                    if pkg.get('pinned'):
                        pin_icon = Gtk.Image.new_from_icon_name("view-pin-symbolic")
                        pkg_row.add_prefix(pin_icon)
                    
                    # Add upgrade button
                    upgrade_button = Gtk.Button()
                    upgrade_button.set_label(_("Upgrade"))
                    upgrade_button.set_valign(Gtk.Align.CENTER)
                    upgrade_button.add_css_class("flat")
                    upgrade_button.connect("clicked", self.__on_upgrade_package_clicked, pkg['name'], expander)
                    
                    pkg_row.add_suffix(upgrade_button)
                    expander.add_row(pkg_row)
                    expander.package_rows.append(pkg_row)
            else:
                empty_row = Adw.ActionRow(
                    title=_("All packages are up to date"),
                    subtitle=_("No updates available")
                )
                empty_row.add_prefix(Gtk.Image.new_from_icon_name("checkbox-checked-symbolic"))
                expander.add_row(empty_row)
                expander.package_rows.append(empty_row)
                expander.set_subtitle(_("Up to date"))

        # Show loading state
        loading_row = Adw.ActionRow(
            title=_("Loading..."),
            subtitle=_("Checking for outdated packages")
        )
        loading_spinner = Gtk.Spinner()
        loading_spinner.start()
        loading_row.add_prefix(loading_spinner)
        expander.add_row(loading_row)

        # Load packages in a thread
        import threading
        def run():
            result = load_in_thread()
            GLib.idle_add(lambda: (
                expander.remove(loading_row),
                on_packages_loaded(result)
            ))
        
        thread = threading.Thread(target=run, daemon=True)
        thread.start()

    def __on_upgrade_package_clicked(self, button, package_name, expander):
        """Handle package upgrade button click"""
        # Disable button and show loading state
        button.set_sensitive(False)
        button.set_label(_("Upgrading..."))
        
        def upgrade_in_thread():
            """Upgrade package in a background thread"""
            try:
                if not homebrew.is_homebrew_installed():
                    return {
                        'success': False,
                        'message': _("Homebrew is not installed")
                    }
                
                homebrew.upgrade_package(package_name)
                return {
                    'success': True,
                    'message': _("{} upgraded successfully").format(package_name)
                }
            except homebrew.HomebrewError as e:
                return {
                    'success': False,
                    'message': str(e)
                }
            except Exception as e:
                return {
                    'success': False,
                    'message': _("Failed to upgrade {}: {}").format(package_name, str(e))
                }
        
        def on_upgrade_complete(result):
            """Handle upgrade completion on main thread"""
            # Show toast notification
            if hasattr(self.__window, 'add_toast'):
                toast = Adw.Toast.new(result['message'])
                toast.set_timeout(3)
                self.__window.add_toast(toast)
            else:
                print(result['message'])
            
            if result['success']:
                # Reload the outdated packages list
                # Clear existing rows using stored references
                if hasattr(expander, 'package_rows'):
                    for row in expander.package_rows:
                        expander.remove(row)
                    expander.package_rows = []
                
                # Reload the list
                self.__load_outdated_packages(expander)
            else:
                # Re-enable button on error
                button.set_sensitive(True)
                button.set_label(_("Upgrade"))
        
        # Run upgrade in background thread
        import threading
        def run():
            result = upgrade_in_thread()
            GLib.idle_add(lambda: on_upgrade_complete(result))
        
        thread = threading.Thread(target=run, daemon=True)
        thread.start()

    def __load_homebrew_packages(self, formulae_expander, casks_expander):
        """Load Homebrew packages and populate the expander rows"""
        def load_in_thread():
            """Load packages in a background thread"""
            try:
                # Check if Homebrew is installed
                if not homebrew.is_homebrew_installed():
                    return {
                        'error': True,
                        'message': _("Homebrew is not installed")
                    }

                # Get installed formulae and casks
                formulae = homebrew.list_installed_packages(formula_only=True)
                all_packages = homebrew.list_installed_packages(formula_only=False)
                
                # Filter to get only casks (packages not in formulae)
                formula_names = {str(pkg['name']) for pkg in formulae if pkg['name']}
                casks_only = [pkg for pkg in all_packages if str(pkg['name']) not in formula_names]

                return {
                    'error': False,
                    'formulae': formulae,
                    'casks': casks_only
                }
            except homebrew.HomebrewError as e:
                return {
                    'error': True,
                    'message': str(e)
                }
            except Exception as e:
                return {
                    'error': True,
                    'message': _("Failed to load Homebrew packages: {}").format(str(e))
                }

        def on_packages_loaded(result):
            """Update UI with loaded packages on the main thread"""
            if result['error']:
                # Show error in both expanders
                for expander in [formulae_expander, casks_expander]:
                    error_row = Adw.ActionRow(
                        title=_("Error"),
                        subtitle=result['message']
                    )
                    error_row.add_prefix(Gtk.Image.new_from_icon_name("dialog-error-symbolic"))
                    expander.add_row(error_row)
                    expander.set_subtitle(_("Failed to load"))
                return

            formulae = result['formulae']
            casks = result['casks']
            
            # Populate formulae expander
            formulae_expander.set_subtitle(_("{} installed").format(len(formulae)))
            if formulae:
                for pkg in sorted(formulae, key=lambda x: x['name']):
                    pkg_row = Adw.ActionRow(
                        title=pkg['name'],
                        subtitle=_("Version: {}").format(pkg['version'])
                    )
                    if pkg.get('installed_on_request'):
                        pkg_row.add_suffix(Gtk.Image.new_from_icon_name("emblem-default-symbolic"))
                    formulae_expander.add_row(pkg_row)
            else:
                empty_row = Adw.ActionRow(
                    title=_("No formulae installed"),
                    subtitle=_("Install formulae using 'brew install <formula>'")
                )
                formulae_expander.add_row(empty_row)

            # Populate casks expander
            casks_expander.set_subtitle(_("{} installed").format(len(casks)))
            if casks:
                for pkg in sorted(casks, key=lambda x: x['name']):
                    pkg_row = Adw.ActionRow(
                        title=pkg['name'],
                        subtitle=_("Version: {}").format(pkg['version'])
                    )
                    if pkg.get('installed_on_request'):
                        pkg_row.add_suffix(Gtk.Image.new_from_icon_name("emblem-default-symbolic"))
                    casks_expander.add_row(pkg_row)
            else:
                empty_row = Adw.ActionRow(
                    title=_("No casks installed"),
                    subtitle=_("Install casks using 'brew install --cask <cask>'")
                )
                casks_expander.add_row(empty_row)

        # Show loading state in both expanders
        for expander in [formulae_expander, casks_expander]:
            loading_row = Adw.ActionRow(
                title=_("Loading..."),
                subtitle=_("Fetching installed packages")
            )
            loading_spinner = Gtk.Spinner()
            loading_spinner.start()
            loading_row.add_prefix(loading_spinner)
            expander.add_row(loading_row)
            expander.loading_row = loading_row  # Store reference for removal

        # Load packages in a thread
        import threading
        def run():
            result = load_in_thread()
            GLib.idle_add(lambda: (
                formulae_expander.remove(formulae_expander.loading_row),
                casks_expander.remove(casks_expander.loading_row),
                on_packages_loaded(result)
            ))
        
        thread = threading.Thread(target=run, daemon=True)
        thread.start()




    def set_page_inactive(self):
        return

    def finish(self):
        return True
