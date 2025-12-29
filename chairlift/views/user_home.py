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
import yaml
import os
import json
import subprocess

_ = __builtins__["_"]


class ChairLiftUserHome:
    """Manager for all content pages in the NavigationSplitView"""

    def __init__(self, window):
        self.__window = window

        # Load configuration
        self.__config = self.__load_config()

        # Create individual pages (returns ToolbarView with embedded PreferencesPage)
        self.system_page_widget = self.__create_page()
        self.updates_page_widget = self.__create_page()
        self.applications_page_widget = self.__create_page()
        self.maintenance_page_widget = self.__create_page()
        self.help_page_widget = self.__create_page()

        # Store references to the actual preferences pages for building content
        self.system_page = self.system_page_widget.preferences_page
        self.updates_page = self.updates_page_widget.preferences_page
        self.applications_page = self.applications_page_widget.preferences_page
        self.maintenance_page = self.maintenance_page_widget.preferences_page
        self.help_page = self.help_page_widget.preferences_page

        # Build the preference groups dynamically
        self.__build_system_page()
        self.__build_updates_page()
        self.__build_applications_page()
        self.__build_maintenance_page()
        self.__build_help_page()

    def __create_page(self):
        """Create a page with toolbar view and scrolled content"""
        toolbar_view = Adw.ToolbarView()

        # Add header bar
        header_bar = Adw.HeaderBar()
        toolbar_view.add_top_bar(header_bar)

        # Create scrolled window with preferences page
        scrolled_window = Gtk.ScrolledWindow()
        scrolled_window.set_policy(Gtk.PolicyType.NEVER, Gtk.PolicyType.AUTOMATIC)
        scrolled_window.set_vexpand(True)

        preferences_page = Adw.PreferencesPage()
        scrolled_window.set_child(preferences_page)

        toolbar_view.set_content(scrolled_window)

        # Store the preferences_page in the toolbar_view for easy access
        toolbar_view.preferences_page = preferences_page

        return toolbar_view

    def get_page(self, page_name):
        """Get a page widget (ToolbarView) by name"""
        pages = {
            "system": self.system_page_widget,
            "updates": self.updates_page_widget,
            "applications": self.applications_page_widget,
            "maintenance": self.maintenance_page_widget,
            "help": self.help_page_widget,
        }
        return pages.get(page_name)

    def __load_config(self):
        """Load configuration from YAML file"""
        # Try multiple possible locations for the config file
        # Order matters: system-wide config takes precedence over installed defaults
        config_paths = [
            '/etc/chairlift/config.yml',                                    # System-wide override
            '/usr/share/chairlift/config.yml',                              # Package maintainer default
            os.path.join(os.path.dirname(__file__), '..', 'config.yml'),    # Development (source tree)
            os.path.join(os.path.dirname(__file__), 'config.yml'),          # Fallback
        ]

        for config_path in config_paths:
            if os.path.exists(config_path):
                try:
                    with open(config_path, 'r') as f:
                        return yaml.safe_load(f)
                except Exception as e:
                    print(f"Error loading config from {config_path}: {e}")

        # Return default config if no file found
        print("No config file found, using defaults (all groups enabled)")
        return {}

    def __is_group_enabled(self, page_name, group_name):
        """Check if a preference group is enabled in the configuration"""
        try:
            return self.__config.get(page_name, {}).get(group_name, {}).get('enabled', True)
        except Exception:
            # Default to enabled if there's any issue reading config
            return True

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
        if self.__is_group_enabled('system_page', 'system_info_group'):
            self.system_page.add(system_info_group)

        # NBC Status group - only show if NBC is booted
        if os.path.exists('/run/nbc-booted'):
            nbc_status_group = Adw.PreferencesGroup()
            nbc_status_group.set_title(_("NBC Status"))
            nbc_status_group.set_description(_("View NBC system status information"))

            # Create an expander row for NBC status
            nbc_expander = Adw.ExpanderRow()
            nbc_expander.set_title(_("NBC Status Details"))
            nbc_expander.set_subtitle(_("Loading..."))

            nbc_status_group.add(nbc_expander)
            if self.__is_group_enabled('system_page', 'nbc_status_group'):
                self.system_page.add(nbc_status_group)

            # Load NBC status data asynchronously
            self.__load_nbc_status(nbc_expander)

        health_group = Adw.PreferencesGroup()
        health_group.set_title(_("System Health"))
        health_group.set_description(_("Overview of system health and diagnostics"))

        # Add System Performance row
        system_performance_row = Adw.ActionRow(
            title=_("System Performance"),
            subtitle=_("Monitor CPU, memory, and system resources")
        )
        system_performance_row.set_activatable(True)
        system_performance_row.add_suffix(Gtk.Image.new_from_icon_name("adw-external-link-symbolic"))

        # Get app_id from config, default to Mission Center
        app_id = self.__config.get('system_page', {}).get('health_group', {}).get('app_id', 'io.missioncenter.MissionCenter')
        system_performance_row.connect("activated", self.__on_launch_app_row_activated, app_id)
        health_group.add(system_performance_row)

        if self.__is_group_enabled('system_page', 'health_group'):
            self.system_page.add(health_group)


    def __build_updates_page(self):
        """Build the Updates tab preference groups"""
        # System Updates group
        updates_status_group = Adw.PreferencesGroup()
        updates_status_group.set_title(_("System Updates"))
        updates_status_group.set_description(_("Check for and install system updates"))

        # Get actions from config
        updates_config = self.__config.get('updates_page', {}).get('updates_status_group', {})
        actions = updates_config.get('actions', [])

        # Add action rows for each configured action
        for action in actions:
            title = action.get('title', 'Unknown Action')
            script = action.get('script')
            requires_sudo = action.get('sudo', False)

            if script:
                action_row = Adw.ActionRow(
                    title=_(title),
                    subtitle=script
                )

                # Add sudo indicator if required
                if requires_sudo:
                    sudo_icon = Gtk.Image.new_from_icon_name("dialog-password-symbolic")
                    sudo_icon.set_tooltip_text(_("Requires administrator privileges"))
                    action_row.add_prefix(sudo_icon)

                # Add run button
                run_button = Gtk.Button()
                run_button.set_label(_("Run"))
                run_button.set_valign(Gtk.Align.CENTER)
                run_button.add_css_class("suggested-action")
                run_button.connect("clicked", self.__on_run_maintenance_action, title, script, requires_sudo)

                action_row.add_suffix(run_button)
                updates_status_group.add(action_row)

        if self.__is_group_enabled('updates_page', 'updates_status_group'):
            self.updates_page.add(updates_status_group)

        # Brew Updates group
        brew_updates_group = Adw.PreferencesGroup()
        brew_updates_group.set_title(_("Homebrew Updates"))
        brew_updates_group.set_description(_("Check for and install Homebrew package updates"))

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

        if self.__is_group_enabled('updates_page', 'brew_updates_group'):
            self.updates_page.add(brew_updates_group)

        # Update Settings group
        updates_settings_group = Adw.PreferencesGroup()
        updates_settings_group.set_title(_("Update Settings"))
        updates_settings_group.set_description(_("Configure update preferences"))
        if self.__is_group_enabled('updates_page', 'updates_settings_group'):
            self.updates_page.add(updates_settings_group)

    def __build_applications_page(self):
        """Build the Applications tab preference groups"""
        # Installed Applications group
        applications_installed_group = Adw.PreferencesGroup()
        applications_installed_group.set_title(_("Installed Applications"))
        applications_installed_group.set_description(_("Manage your installed applications"))
        view_apps = Adw.ActionRow(
            title=_("Manage Flatpaks"),
            subtitle=_("Open the application manager to install and manage applications")
        )
        view_apps.set_activatable(True)
        view_apps.add_suffix(Gtk.Image.new_from_icon_name("adw-external-link-symbolic"))

        # Get app_id from config, default to Bazaar
        app_id = self.__config.get('applications_page', {}).get('applications_installed_group', {}).get('app_id', 'io.github.kolunmi.Bazaar')
        view_apps.connect("activated", self.__on_launch_app_row_activated, app_id)
        applications_installed_group.add(view_apps)

        if self.__is_group_enabled('applications_page', 'applications_installed_group'):
            self.applications_page.add(applications_installed_group)

        # Brew group
        brew_group = Adw.PreferencesGroup()
        brew_group.set_title(_("Homebrew"))
        brew_group.set_description(_("Manage Homebrew packages installed on your system"))

        # Add Brew Bundle Dump button
        bundle_dump_row = Adw.ActionRow()
        bundle_dump_row.set_title(_("Brew Bundle Dump"))
        bundle_dump_row.set_subtitle(_("Export currently installed packages to ~/Brewfile"))

        dump_button = Gtk.Button()
        dump_button.set_label(_("Dump"))
        dump_button.set_valign(Gtk.Align.CENTER)
        dump_button.add_css_class("suggested-action")
        dump_button.connect("clicked", self.__on_brew_bundle_dump_clicked)

        bundle_dump_row.add_suffix(dump_button)
        bundle_dump_row.set_activatable_widget(dump_button)
        brew_group.add(bundle_dump_row)

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

        # Store references for reloading
        self.__formulae_expander = formulae_expander
        self.__casks_expander = casks_expander

        # Load Homebrew packages asynchronously
        self.__load_homebrew_packages(formulae_expander, casks_expander)

        if self.__is_group_enabled('applications_page', 'brew_group'):
            self.applications_page.add(brew_group)

        # Homebrew Search group
        brew_search_group = Adw.PreferencesGroup()
        brew_search_group.set_title(_("Search Homebrew"))
        brew_search_group.set_description(_("Search for and install Homebrew formulae"))

        # Search entry row
        search_entry_row = Adw.ActionRow()
        search_entry_row.set_title(_("Search for packages"))

        search_entry = Gtk.SearchEntry()
        search_entry.set_placeholder_text(_("Enter package name..."))
        search_entry.set_hexpand(True)
        search_entry.connect("activate", self.__on_homebrew_search)

        search_button = Gtk.Button()
        search_button.set_icon_name("system-search-symbolic")
        search_button.set_valign(Gtk.Align.CENTER)
        search_button.add_css_class("flat")
        search_button.connect("clicked", lambda btn: self.__on_homebrew_search(search_entry))

        search_box = Gtk.Box(orientation=Gtk.Orientation.HORIZONTAL, spacing=6)
        search_box.append(search_entry)
        search_box.append(search_button)
        search_box.set_hexpand(True)

        search_entry_row.add_suffix(search_box)
        brew_search_group.add(search_entry_row)

        # Search results expander
        search_results_expander = Adw.ExpanderRow()
        search_results_expander.set_title(_("Search Results"))
        search_results_expander.set_subtitle(_("No search performed"))
        search_results_expander.set_enable_expansion(False)
        brew_search_group.add(search_results_expander)

        # Store reference for updating
        self.__search_results_expander = search_results_expander
        self.__search_entry = search_entry

        if self.__is_group_enabled('applications_page', 'brew_search_group'):
            self.applications_page.add(brew_search_group)

        # Brew Bundles group
        brew_bundles_group = Adw.PreferencesGroup()
        brew_bundles_group.set_title(_("Curated Brew Bundles"))
        brew_bundles_group.set_description(_("Install and manage curated Homebrew bundles"))

        # Load and display available bundles
        self.__load_available_bundles(brew_bundles_group)

        if self.__is_group_enabled('applications_page', 'brew_bundles_group'):
            self.applications_page.add(brew_bundles_group)

    def __build_maintenance_page(self):
        """Build the Maintenance tab preference groups"""
        # System Cleanup group
        maintenance_cleanup_group = Adw.PreferencesGroup()
        maintenance_cleanup_group.set_title(_("System Cleanup"))
        maintenance_cleanup_group.set_description(_("Clean up temporary files and free up disk space"))

        # Get actions from config
        cleanup_config = self.__config.get('maintenance_page', {}).get('maintenance_cleanup_group', {})
        actions = cleanup_config.get('actions', [])

        # Add action rows for each configured action
        for action in actions:
            title = action.get('title', 'Unknown Action')
            script = action.get('script')
            requires_sudo = action.get('sudo', False)

            if script:
                action_row = Adw.ActionRow(
                    title=_(title),
                    subtitle=script
                )

                # Add sudo indicator if required
                if requires_sudo:
                    sudo_icon = Gtk.Image.new_from_icon_name("dialog-password-symbolic")
                    sudo_icon.set_tooltip_text(_("Requires administrator privileges"))
                    action_row.add_prefix(sudo_icon)

                # Add run button
                run_button = Gtk.Button()
                run_button.set_label(_("Run"))
                run_button.set_valign(Gtk.Align.CENTER)
                run_button.add_css_class("suggested-action")
                run_button.connect("clicked", self.__on_run_maintenance_action, title, script, requires_sudo)

                action_row.add_suffix(run_button)
                maintenance_cleanup_group.add(action_row)

        if self.__is_group_enabled('maintenance_page', 'maintenance_cleanup_group'):
            self.maintenance_page.add(maintenance_cleanup_group)

        # System Optimization group
        maintenance_optimization_group = Adw.PreferencesGroup()
        maintenance_optimization_group.set_title(_("System Optimization"))
        maintenance_optimization_group.set_description(_("Optimize system performance"))
        if self.__is_group_enabled('maintenance_page', 'maintenance_optimization_group'):
            self.maintenance_page.add(maintenance_optimization_group)

    def __build_help_page(self):
        """Build the Help tab preference groups"""
        # Help Resources group
        help_resources_group = Adw.PreferencesGroup()
        help_resources_group.set_title(_("Help Resources"))
        help_resources_group.set_description(_("Access help and support resources"))

        # Get URLs from config
        help_config = self.__config.get('help_page', {}).get('help_resources_group', {})
        website_url = help_config.get('website')
        issues_url = help_config.get('issues')
        chat_url = help_config.get('chat')

        # Add website row if URL is configured
        if website_url:
            website_row = Adw.ActionRow(
                title=_("Website"),
                subtitle=_("Visit the project website")
            )
            website_row.set_activatable(True)
            website_row.add_suffix(Gtk.Image.new_from_icon_name("adw-external-link-symbolic"))
            website_row.connect("activated", self.__on_url_row_activated, website_url)
            help_resources_group.add(website_row)

        # Add issues row if URL is configured
        if issues_url:
            issues_row = Adw.ActionRow(
                title=_("Report Issues"),
                subtitle=_("Report bugs and request features")
            )
            issues_row.set_activatable(True)
            issues_row.add_suffix(Gtk.Image.new_from_icon_name("adw-external-link-symbolic"))
            issues_row.connect("activated", self.__on_url_row_activated, issues_url)
            help_resources_group.add(issues_row)

        # Add chat row if URL is configured
        if chat_url:
            chat_row = Adw.ActionRow(
                title=_("Community Chat"),
                subtitle=_("Join discussions and get help from the community")
            )
            chat_row.set_activatable(True)
            chat_row.add_suffix(Gtk.Image.new_from_icon_name("adw-external-link-symbolic"))
            chat_row.connect("activated", self.__on_url_row_activated, chat_url)
            help_resources_group.add(chat_row)

        if self.__is_group_enabled('help_page', 'help_resources_group'):
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

    def __on_run_maintenance_action(self, button, title, script, requires_sudo):
        """Run a maintenance action script"""
        # Disable button and show loading state
        button.set_sensitive(False)
        original_label = button.get_label()
        button.set_label(_("Running..."))

        def run_in_thread():
            """Run the maintenance script in a background thread"""
            try:
                import subprocess
                import shlex

                # Split script into command and arguments
                script_parts = shlex.split(script)

                # Build command
                if requires_sudo:
                    # Use pkexec for graphical sudo prompt
                    cmd = ['pkexec'] + script_parts
                else:
                    cmd = script_parts

                # Run the script
                result = subprocess.run(
                    cmd,
                    capture_output=True,
                    text=True,
                    timeout=300  # 5 minute timeout
                )

                if result.returncode == 0:
                    return {
                        'success': True,
                        'message': _('{} completed successfully').format(title)
                    }
                else:
                    error_msg = result.stderr.strip() if result.stderr else _("Unknown error")
                    return {
                        'success': False,
                        'message': _('{} failed: {}').format(title, error_msg)
                    }
            except subprocess.TimeoutExpired:
                return {
                    'success': False,
                    'message': _('{} timed out').format(title)
                }
            except FileNotFoundError:
                return {
                    'success': False,
                    'message': _('{} not found: {}').format(title, script)
                }
            except Exception as e:
                return {
                    'success': False,
                    'message': _('{} failed: {}').format(title, str(e))
                }

        def on_complete(result):
            """Handle completion on main thread"""
            button.set_sensitive(True)
            button.set_label(original_label)

            # Show toast notification
            if hasattr(self.__window, 'add_toast'):
                toast = Adw.Toast.new(result['message'])
                toast.set_timeout(3)
                self.__window.add_toast(toast)
            else:
                print(result['message'])

        # Run in background thread
        import threading
        def run():
            result = run_in_thread()
            GLib.idle_add(lambda: on_complete(result))

        thread = threading.Thread(target=run, daemon=True)
        thread.start()

    def __on_brew_bundle_dump_clicked(self, button):
        """Handle brew bundle dump button click"""
        # Disable button and show loading state
        button.set_sensitive(False)
        button.set_label(_("Dumping..."))

        def dump_in_thread():
            """Dump bundle in a background thread"""
            try:
                if not homebrew.is_homebrew_installed():
                    return {
                        'success': False,
                        'message': _("Homebrew is not installed")
                    }

                brewfile_path = homebrew.dump_bundle()
                return {
                    'success': True,
                    'message': _("Brewfile dumped to {}").format(brewfile_path)
                }
            except homebrew.HomebrewError as e:
                return {
                    'success': False,
                    'message': str(e)
                }
            except Exception as e:
                return {
                    'success': False,
                    'message': _("Unexpected error: {}").format(str(e))
                }

        def on_dump_complete(result):
            """Handle completion of dump operation"""
            # Re-enable button
            button.set_sensitive(True)
            button.set_label(_("Dump"))

            # Show toast notification
            if hasattr(self.__window, 'add_toast'):
                toast = Adw.Toast.new(result['message'])
                toast.set_timeout(3)
                self.__window.add_toast(toast)

        # Run in thread
        import threading
        def run():
            result = dump_in_thread()
            GLib.idle_add(lambda: on_dump_complete(result))

        thread = threading.Thread(target=run, daemon=True)
        thread.start()

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

    def __load_nbc_status(self, expander):
        """Load NBC status information and populate the expander row"""
        def load_in_thread():
            """Load NBC status in a background thread"""
            try:
                # Execute nbc status --json
                result = subprocess.run(
                    ['nbc', 'status', '--json'],
                    capture_output=True,
                    text=True,
                    timeout=10
                )
                
                if result.returncode != 0:
                    return {
                        'error': True,
                        'message': _("Failed to get NBC status: {}").format(result.stderr.strip() or result.stdout.strip())
                    }
                
                # Parse JSON output
                try:
                    status_data = json.loads(result.stdout)
                    return {
                        'error': False,
                        'data': status_data
                    }
                except json.JSONDecodeError as e:
                    return {
                        'error': True,
                        'message': _("Failed to parse NBC status output: {}").format(str(e))
                    }
                    
            except FileNotFoundError:
                return {
                    'error': True,
                    'message': _("NBC command not found")
                }
            except subprocess.TimeoutExpired:
                return {
                    'error': True,
                    'message': _("NBC status command timed out")
                }
            except Exception as e:
                return {
                    'error': True,
                    'message': _("Failed to load NBC status: {}").format(str(e))
                }

        def on_status_loaded(result):
            """Update UI with loaded NBC status on the main thread"""
            if result['error']:
                error_row = Adw.ActionRow(
                    title=_("Error"),
                    subtitle=result['message']
                )
                error_row.add_prefix(Gtk.Image.new_from_icon_name("dialog-error-symbolic"))
                expander.add_row(error_row)
                expander.set_subtitle(_("Failed to load"))
                return

            status_data = result['data']
            
            # Parse the status data and add rows
            if isinstance(status_data, dict):
                # Count items for subtitle
                expander.set_subtitle(_("{} status items").format(len(status_data)))
                
                # Add each key-value pair as a row
                for key, value in sorted(status_data.items()):
                    # Convert key to readable format
                    readable_key = key.replace('_', ' ').title()
                    
                    # Convert value to string for display
                    if isinstance(value, (dict, list)):
                        value_str = json.dumps(value, indent=2)
                    else:
                        value_str = str(value)
                    
                    row = Adw.ActionRow(
                        title=readable_key,
                        subtitle=value_str
                    )
                    expander.add_row(row)
            elif isinstance(status_data, list):
                # If it's a list, add each item
                expander.set_subtitle(_("{} status items").format(len(status_data)))
                
                for idx, item in enumerate(status_data):
                    if isinstance(item, dict):
                        # If item is a dict, show first key-value or a summary
                        title = item.get('name') or item.get('title') or _("Item {}").format(idx + 1)
                        subtitle = item.get('status') or item.get('description') or json.dumps(item, indent=2)
                    else:
                        title = _("Item {}").format(idx + 1)
                        subtitle = str(item)
                    
                    row = Adw.ActionRow(
                        title=title,
                        subtitle=subtitle
                    )
                    expander.add_row(row)
            else:
                # Single value
                expander.set_subtitle(str(status_data))
                row = Adw.ActionRow(
                    title=_("Status"),
                    subtitle=str(status_data)
                )
                expander.add_row(row)

        # Show loading state
        loading_row = Adw.ActionRow(
            title=_("Loading..."),
            subtitle=_("Fetching NBC status")
        )
        loading_spinner = Gtk.Spinner()
        loading_spinner.start()
        loading_row.add_prefix(loading_spinner)
        expander.add_row(loading_row)

        # Load status in a thread
        import threading
        def run():
            result = load_in_thread()
            GLib.idle_add(lambda: (
                expander.remove(loading_row),
                on_status_loaded(result)
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
        # Clear existing rows if this is a reload
        for expander in [formulae_expander, casks_expander]:
            if hasattr(expander, 'package_rows'):
                for row in expander.package_rows:
                    expander.remove(row)
                expander.package_rows = []

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
            # Initialize package_rows tracking for both expanders
            if not hasattr(formulae_expander, 'package_rows'):
                formulae_expander.package_rows = []
            if not hasattr(casks_expander, 'package_rows'):
                casks_expander.package_rows = []

            if result['error']:
                # Show error in both expanders
                for expander in [formulae_expander, casks_expander]:
                    error_row = Adw.ActionRow(
                        title=_("Error"),
                        subtitle=result['message']
                    )
                    error_row.add_prefix(Gtk.Image.new_from_icon_name("dialog-error-symbolic"))
                    expander.add_row(error_row)
                    expander.package_rows.append(error_row)
                    expander.set_subtitle(_("Failed to load"))
                return

            formulae = result['formulae']
            casks = result['casks']

            # Populate formulae expander
            formulae_expander.set_subtitle(_("{} installed").format(len(formulae)))
            if formulae:
                for pkg in sorted(formulae, key=lambda x: x['name']):
                    # Sanitize description to handle HTML entities
                    desc = GLib.markup_escape_text(pkg.get('desc', ''))
                    pkg_row = Adw.ActionRow(
                        title=pkg['name'] + " - " + desc,
                        subtitle=_("Version: {}").format(pkg['version'])
                    )

                    # Add pin/unpin button based on current pinned status
                    pin_button = Gtk.Button()
                    pin_button.set_valign(Gtk.Align.CENTER)
                    pin_button.add_css_class("flat")

                    if pkg.get('pinned'):
                        # Package is pinned, show unpin button
                        pin_button.set_icon_name("changes-allow-symbolic")
                        pin_button.set_tooltip_text(_("Unpin package"))
                        pin_button.connect("clicked", self.__on_unpin_package_clicked, pkg['name'], formulae_expander)
                    else:
                        # Package is not pinned, show pin button
                        pin_button.set_icon_name("view-pin-symbolic")
                        pin_button.set_tooltip_text(_("Pin package"))
                        pin_button.connect("clicked", self.__on_pin_package_clicked, pkg['name'], formulae_expander)

                    pkg_row.add_suffix(pin_button)

                    # Add remove button
                    remove_button = Gtk.Button()
                    remove_button.set_icon_name("user-trash-symbolic")
                    remove_button.set_valign(Gtk.Align.CENTER)
                    remove_button.add_css_class("flat")
                    remove_button.add_css_class("destructive-action")
                    remove_button.set_tooltip_text(_("Uninstall package"))
                    remove_button.connect("clicked", self.__on_remove_package_clicked, pkg['name'], formulae_expander)
                    pkg_row.add_suffix(remove_button)

                    if pkg.get('installed_on_request'):
                        request_icon = Gtk.Image.new_from_icon_name("emblem-default-symbolic")
                        request_icon.set_tooltip_text(_("Installed on request"))
                        pkg_row.add_suffix(request_icon)

                    formulae_expander.add_row(pkg_row)
                    formulae_expander.package_rows.append(pkg_row)
            else:
                empty_row = Adw.ActionRow(
                    title=_("No formulae installed"),
                    subtitle=_("Install formulae using 'brew install <formula>'")
                )
                formulae_expander.add_row(empty_row)
                formulae_expander.package_rows.append(empty_row)

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
                    casks_expander.package_rows.append(pkg_row)
            else:
                empty_row = Adw.ActionRow(
                    title=_("No casks installed"),
                    subtitle=_("Install casks using 'brew install --cask <cask>'")
                )
                casks_expander.add_row(empty_row)
                casks_expander.package_rows.append(empty_row)

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

    def __on_pin_package_clicked(self, button, package_name, expander):
        """Handle package pin button click"""
        # Disable button during operation
        button.set_sensitive(False)

        def pin_in_thread():
            """Pin package in a background thread"""
            try:
                if not homebrew.is_homebrew_installed():
                    return {
                        'success': False,
                        'message': _("Homebrew is not installed")
                    }

                homebrew.pin_package(package_name)
                return {
                    'success': True,
                    'message': _("{} pinned successfully").format(package_name)
                }
            except homebrew.HomebrewError as e:
                return {
                    'success': False,
                    'message': str(e)
                }
            except Exception as e:
                return {
                    'success': False,
                    'message': _("Failed to pin {}: {}").format(package_name, str(e))
                }

        def on_pin_complete(result):
            """Handle pin completion on main thread"""
            # Show toast notification
            if hasattr(self.__window, 'add_toast'):
                toast = Adw.Toast.new(result['message'])
                toast.set_timeout(3)
                self.__window.add_toast(toast)
            else:
                print(result['message'])

            button.set_sensitive(True)

            # Reload packages to reflect new pinned state
            if result['success']:
                self.__load_homebrew_packages(self.__formulae_expander, self.__casks_expander)

        # Run pin in background thread
        import threading
        def run():
            result = pin_in_thread()
            GLib.idle_add(lambda: on_pin_complete(result))

        thread = threading.Thread(target=run, daemon=True)
        thread.start()

    def __on_unpin_package_clicked(self, button, package_name, expander):
        """Handle package unpin button click"""
        # Disable button during operation
        button.set_sensitive(False)

        def unpin_in_thread():
            """Unpin package in a background thread"""
            try:
                if not homebrew.is_homebrew_installed():
                    return {
                        'success': False,
                        'message': _("Homebrew is not installed")
                    }

                homebrew.unpin_package(package_name)
                return {
                    'success': True,
                    'message': _("{} unpinned successfully").format(package_name)
                }
            except homebrew.HomebrewError as e:
                return {
                    'success': False,
                    'message': str(e)
                }
            except Exception as e:
                return {
                    'success': False,
                    'message': _("Failed to unpin {}: {}").format(package_name, str(e))
                }

        def on_unpin_complete(result):
            """Handle unpin completion on main thread"""
            # Show toast notification
            if hasattr(self.__window, 'add_toast'):
                toast = Adw.Toast.new(result['message'])
                toast.set_timeout(3)
                self.__window.add_toast(toast)
            else:
                print(result['message'])

            button.set_sensitive(True)

            # Reload packages to reflect new unpinned state
            if result['success']:
                self.__load_homebrew_packages(self.__formulae_expander, self.__casks_expander)

        # Run unpin in background thread
        import threading
        def run():
            result = unpin_in_thread()
            GLib.idle_add(lambda: on_unpin_complete(result))

        thread = threading.Thread(target=run, daemon=True)
        thread.start()

    def __on_remove_package_clicked(self, button, package_name, expander):
        """Handle package remove button click"""
        # Disable button during operation
        button.set_sensitive(False)

        def remove_in_thread():
            """Remove package in a background thread"""
            try:
                if not homebrew.is_homebrew_installed():
                    return {
                        'success': False,
                        'message': _("Homebrew is not installed")
                    }

                homebrew.uninstall_package(package_name)
                return {
                    'success': True,
                    'message': _("{} uninstalled successfully").format(package_name)
                }
            except homebrew.HomebrewError as e:
                return {
                    'success': False,
                    'message': str(e)
                }
            except Exception as e:
                return {
                    'success': False,
                    'message': _("Failed to uninstall {}: {}").format(package_name, str(e))
                }

        def on_remove_complete(result):
            """Handle remove completion on main thread"""
            # Show toast notification
            if hasattr(self.__window, 'add_toast'):
                toast = Adw.Toast.new(result['message'])
                toast.set_timeout(3)
                self.__window.add_toast(toast)
            else:
                print(result['message'])

            if result['success']:
                # Reload the packages list
                # Clear existing rows
                if hasattr(expander, 'package_rows'):
                    for row in expander.package_rows:
                        expander.remove(row)
                    expander.package_rows = []

                # Get parent to find both expanders
                # For now just re-enable button on success
                # TODO: Reload the full list properly

            button.set_sensitive(True)

        # Run remove in background thread
        import threading
        def run():
            result = remove_in_thread()
            GLib.idle_add(lambda: on_remove_complete(result))

        thread = threading.Thread(target=run, daemon=True)
        thread.start()

    def __on_homebrew_search(self, entry):
        """Handle Homebrew search"""
        query = entry.get_text().strip()
        if not query:
            return

        expander = self.__search_results_expander

        # Clear previous results
        if hasattr(expander, 'search_rows'):
            for row in expander.search_rows:
                expander.remove(row)
            expander.search_rows = []
        else:
            expander.search_rows = []

        # Show loading state
        expander.set_subtitle(_("Searching..."))
        expander.set_enable_expansion(True)
        loading_row = Adw.ActionRow(
            title=_("Searching for '{}'...").format(query),
            subtitle=_("Please wait")
        )
        loading_spinner = Gtk.Spinner()
        loading_spinner.start()
        loading_row.add_prefix(loading_spinner)
        expander.add_row(loading_row)
        expander.search_rows.append(loading_row)

        def search_in_thread():
            """Search in background thread"""
            try:
                if not homebrew.is_homebrew_installed():
                    return {
                        'error': True,
                        'message': _("Homebrew is not installed")
                    }

                # Get search results
                results = homebrew.search_formula(query, limit=20)

                # Get installed packages to check against search results
                installed = homebrew.list_installed_packages(formula_only=False)
                installed_names = {pkg['name'] for pkg in installed}

                return {
                    'error': False,
                    'results': results,
                    'query': query,
                    'installed_names': installed_names
                }
            except homebrew.HomebrewError as e:
                return {
                    'error': True,
                    'message': str(e)
                }
            except Exception as e:
                return {
                    'error': True,
                    'message': _("Search failed: {}").format(str(e))
                }

        def on_search_complete(result):
            """Handle search completion on main thread"""
            # Clear loading row
            if expander.search_rows:
                for row in expander.search_rows:
                    expander.remove(row)
                expander.search_rows = []

            if result['error']:
                error_row = Adw.ActionRow(
                    title=_("Error"),
                    subtitle=result['message']
                )
                error_row.add_prefix(Gtk.Image.new_from_icon_name("dialog-error-symbolic"))
                expander.add_row(error_row)
                expander.search_rows.append(error_row)
                expander.set_subtitle(_("Search failed"))
                return

            results = result['results']
            query = result['query']
            installed_names = result.get('installed_names', set())

            if results:
                expander.set_subtitle(_("{} results for '{}'").format(len(results), query))

                for pkg in results:
                    # Sanitize description to handle HTML entities
                    desc = pkg.get('description', _("No description available"))
                    if desc:
                        desc = GLib.markup_escape_text(desc)
                    pkg_row = Adw.ActionRow(
                        title=pkg['name'],
                        subtitle=desc
                    )

                    # Check if package is already installed
                    is_installed = pkg['name'] in installed_names

                    # Add install button
                    install_button = Gtk.Button()
                    install_button.set_valign(Gtk.Align.CENTER)

                    if is_installed:
                        install_button.set_label(_("Installed"))
                        install_button.set_sensitive(False)
                        install_button.add_css_class("success")
                    else:
                        install_button.set_label(_("Install"))
                        install_button.add_css_class("suggested-action")
                        install_button.connect("clicked", self.__on_install_package_clicked, pkg['name'])

                    pkg_row.add_suffix(install_button)
                    expander.add_row(pkg_row)
                    expander.search_rows.append(pkg_row)
            else:
                empty_row = Adw.ActionRow(
                    title=_("No results found"),
                    subtitle=_("Try a different search term")
                )
                expander.add_row(empty_row)
                expander.search_rows.append(empty_row)
                expander.set_subtitle(_("No results for '{}'").format(query))

        # Run search in background thread
        import threading
        def run():
            result = search_in_thread()
            GLib.idle_add(lambda: on_search_complete(result))

        thread = threading.Thread(target=run, daemon=True)
        thread.start()

    def __on_install_package_clicked(self, button, package_name):
        """Handle package install button click"""
        # Disable button and show loading state
        button.set_sensitive(False)
        button.set_label(_("Installing..."))

        def install_in_thread():
            """Install package in a background thread"""
            try:
                if not homebrew.is_homebrew_installed():
                    return {
                        'success': False,
                        'message': _("Homebrew is not installed")
                    }

                # Use brew install command
                homebrew._run_brew_command(['install', package_name], timeout=600)
                return {
                    'success': True,
                    'message': _("{} installed successfully").format(package_name)
                }
            except homebrew.HomebrewError as e:
                return {
                    'success': False,
                    'message': str(e)
                }
            except Exception as e:
                return {
                    'success': False,
                    'message': _("Failed to install {}: {}").format(package_name, str(e))
                }

        def on_install_complete(result):
            """Handle install completion on main thread"""
            # Show toast notification
            if hasattr(self.__window, 'add_toast'):
                toast = Adw.Toast.new(result['message'])
                toast.set_timeout(3)
                self.__window.add_toast(toast)
            else:
                print(result['message'])

            if result['success']:
                # Change button to show installed state
                button.set_label(_("Installed"))
                button.add_css_class("success")
                # Reload the installed packages list
                self.__load_homebrew_packages(self.__formulae_expander, self.__casks_expander)
            else:
                # Re-enable button on error
                button.set_sensitive(True)
                button.set_label(_("Install"))

        # Run install in background thread
        import threading
        def run():
            result = install_in_thread()
            GLib.idle_add(lambda: on_install_complete(result))

        thread = threading.Thread(target=run, daemon=True)
        thread.start()

    def __load_available_bundles(self, group):
        """Load and display available Brewfile bundles"""
        try:
            # Get bundle paths from config, default to Snow Linux path
            bundles_config = self.__config.get('applications_page', {}).get('brew_bundles_group', {})
            bundles_paths = bundles_config.get('bundles_paths', ['/usr/share/snow/bundles'])

            # Collect all bundles from all paths
            all_bundles = []
            for path in bundles_paths:
                try:
                    bundles = homebrew.available_bundles(path)
                    all_bundles.extend(bundles)
                except Exception as e:
                    print(f"Error loading bundles from {path}: {e}")

            if not all_bundles:
                empty_row = Adw.ActionRow(
                    title=_("No bundles available"),
                    subtitle=_("No preconfigured bundles found")
                )
                group.add(empty_row)
                return

            for bundle in bundles:
                bundle_row = Adw.ActionRow(
                    title=bundle['filename'].replace('.Brewfile', ''),
                    subtitle=bundle['description']
                )

                # Add install button
                install_button = Gtk.Button()
                install_button.set_label(_("Install"))
                install_button.set_valign(Gtk.Align.CENTER)
                install_button.add_css_class("suggested-action")
                install_button.connect("clicked", self.__on_install_bundle_clicked, bundle['path'], bundle['filename'])

                bundle_row.add_suffix(install_button)
                group.add(bundle_row)

        except homebrew.HomebrewError as e:
            error_row = Adw.ActionRow(
                title=_("Error loading bundles"),
                subtitle=str(e)
            )
            error_row.add_prefix(Gtk.Image.new_from_icon_name("dialog-error-symbolic"))
            group.add(error_row)
        except Exception as e:
            error_row = Adw.ActionRow(
                title=_("Error loading bundles"),
                subtitle=_("Failed to load bundles: {}").format(str(e))
            )
            error_row.add_prefix(Gtk.Image.new_from_icon_name("dialog-error-symbolic"))
            group.add(error_row)

    def __on_install_bundle_clicked(self, button, bundle_path, bundle_name):
        """Handle bundle install button click"""
        # Disable button and show loading state
        button.set_sensitive(False)
        button.set_label(_("Installing..."))

        def install_in_thread():
            """Install bundle in a background thread"""
            try:
                if not homebrew.is_homebrew_installed():
                    return {
                        'success': False,
                        'message': _("Homebrew is not installed")
                    }

                homebrew.install_bundle(bundle_path)
                return {
                    'success': True,
                    'message': _("{} bundle installed successfully").format(bundle_name.replace('.Brewfile', ''))
                }
            except homebrew.HomebrewError as e:
                return {
                    'success': False,
                    'message': str(e)
                }
            except Exception as e:
                return {
                    'success': False,
                    'message': _("Failed to install bundle: {}").format(str(e))
                }

        def on_install_complete(result):
            """Handle install completion on main thread"""
            # Show toast notification
            if hasattr(self.__window, 'add_toast'):
                toast = Adw.Toast.new(result['message'])
                toast.set_timeout(3)
                self.__window.add_toast(toast)
            else:
                print(result['message'])

            if result['success']:
                # Change button to show installed state
                button.set_label(_("Installed"))
                button.add_css_class("success")
                # Reload the installed packages list
                self.__load_homebrew_packages(self.__formulae_expander, self.__casks_expander)
            else:
                # Re-enable button on error
                button.set_sensitive(True)
                button.set_label(_("Install"))

        # Run install in background thread
        import threading
        def run():
            result = install_in_thread()
            GLib.idle_add(lambda: on_install_complete(result))

        thread = threading.Thread(target=run, daemon=True)
        thread.start()


    def finish(self):
        return True
