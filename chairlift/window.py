# window.py
#
# Copyright 2023 mirkobrombin
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

import threading

from gi.repository import Gtk, Adw, GLib, Gio, Gdk

import chairlift.core.backend as backend

from chairlift.dialog import ChairLiftDialog

_ = __builtins__["_"]

@Gtk.Template(resource_path="/org/frostyard/ChairLift/gtk/window.ui")
class ChairLiftWindow(Adw.ApplicationWindow):
    __gtype_name__ = "ChairLiftWindow"

    split_view = Gtk.Template.Child()
    sidebar_list = Gtk.Template.Child()
    content_stack = Gtk.Template.Child()
    toasts = Gtk.Template.Child()
    style_manager = Adw.StyleManager().get_default()

    pages = {}

    def __init__(self, moduledir: str,  **kwargs):
        super().__init__(**kwargs)

        self.moduledir = moduledir

        self.__build_ui()
        self.__setup_actions()

        backend.subscribe_errors(self.__error_received)


    def __error_received(self, script_name: str, command: list[str], id: int):
        GLib.idle_add(self.__error_toast, _("Execution failed: ") + script_name, id)

    def add_toast(self, toast: any):

        self.toasts.add_toast(toast)


    def __error_toast(self, message: str, id: int):
        toast = Adw.Toast.new(message)
        toast.props.timeout = 0
        toast.props.button_label = _("Details")
        toast.connect("button-clicked", self.__error_toast_clicked, id)
        self.toasts.add_toast(toast)

    def __error_toast_clicked(self, widget, id: int):
        message = backend.errors[id]
        dialog = ChairLiftDialog(self, _("Error log"), message)
        dialog.present()




    def __build_ui(self):
        from chairlift.views.user_home import ChairLiftUserHome

        self.__view_home = ChairLiftUserHome(self)

        # Setup sidebar navigation items
        nav_items = [
            {"name": "applications", "title": _("Applications"), "icon": "application-x-executable-symbolic"},
            {"name": "maintenance", "title": _("Maintenance"), "icon": "emblem-system-symbolic"},
            {"name": "updates", "title": _("Updates"), "icon": "software-update-available-symbolic"},
            {"name": "system", "title": _("System"), "icon": "computer-symbolic"},
            {"name": "help", "title": _("Help"), "icon": "help-browser-symbolic"},
        ]

        for item in nav_items:
            row = Adw.ActionRow()
            row.set_title(item["title"])
            row.set_icon_name(item["icon"])
            row.set_activatable(True)
            row.page_name = item["name"]
            self.sidebar_list.append(row)
            
            # Get page from view_home
            page = self.__view_home.get_page(item["name"])
            if page:
                self.pages[item["name"]] = page
                self.content_stack.add_child(page)
        
        # Connect sidebar selection
        self.sidebar_list.connect("row-activated", self.__on_sidebar_row_activated)
        
        # Select first item by default
        self.sidebar_list.select_row(self.sidebar_list.get_row_at_index(0))
        if nav_items:
            first_page = self.pages.get(nav_items[0]["name"])
            if first_page:
                self.content_stack.set_visible_child(first_page)
        
        # Enable window controls
        self.set_deletable(True)
    
    def __on_sidebar_row_activated(self, listbox, row):
        """Handle sidebar navigation"""
        page_name = getattr(row, 'page_name', None)
        if page_name and page_name in self.pages:
            self.content_stack.set_visible_child(self.pages[page_name])
            # Show content page in narrow mode
            self.split_view.set_show_content(True)
    
    def __setup_actions(self):
        """Setup window actions"""
        # Keyboard shortcuts action
        shortcuts_action = Gio.SimpleAction.new("show-shortcuts", None)
        shortcuts_action.connect("activate", self.__on_show_shortcuts)
        self.add_action(shortcuts_action)
        
        # Help action
        help_action = Gio.SimpleAction.new("show-help", None)
        help_action.connect("activate", self.__on_show_help)
        self.add_action(help_action)
        
        # About action
        about_action = Gio.SimpleAction.new("show-about", None)
        about_action.connect("activate", self.__on_show_about)
        self.add_action(about_action)
        
        # Navigation actions
        for i, page_name in enumerate(["applications", "maintenance", "updates", "system", "help"], start=1):
            action = Gio.SimpleAction.new(f"navigate-{page_name}", None)
            action.connect("activate", self.__on_navigate_to_page, page_name)
            self.add_action(action)
    
    def __on_show_shortcuts(self, action, param):
        """Show keyboard shortcuts window"""
        builder = Gtk.Builder.new_from_string("""
<?xml version="1.0" encoding="UTF-8"?>
<interface>
  <object class="GtkShortcutsWindow" id="shortcuts_window">
    <property name="modal">true</property>
    <child>
      <object class="GtkShortcutsSection">
        <property name="section-name">shortcuts</property>
        <child>
          <object class="GtkShortcutsGroup">
            <property name="title" translatable="yes">General</property>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="title" translatable="yes">Show Keyboard Shortcuts</property>
                <property name="accelerator">&lt;Primary&gt;question</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="title" translatable="yes">Quit</property>
                <property name="accelerator">&lt;Primary&gt;q</property>
              </object>
            </child>
          </object>
        </child>
        <child>
          <object class="GtkShortcutsGroup">
            <property name="title" translatable="yes">Navigation</property>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="title" translatable="yes">Applications</property>
                <property name="accelerator">&lt;Alt&gt;1</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="title" translatable="yes">Maintenance</property>
                <property name="accelerator">&lt;Alt&gt;2</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="title" translatable="yes">Updates</property>
                <property name="accelerator">&lt;Alt&gt;3</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="title" translatable="yes">System</property>
                <property name="accelerator">&lt;Alt&gt;4</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="title" translatable="yes">Help</property>
                <property name="accelerator">&lt;Alt&gt;5</property>
              </object>
            </child>
          </object>
        </child>
      </object>
    </child>
  </object>
</interface>
""", -1)
        shortcuts_window = builder.get_object("shortcuts_window")
        shortcuts_window.set_transient_for(self)
        shortcuts_window.present()
    
    def __on_show_help(self, action, param):
        """Show help documentation"""
        Gtk.show_uri(self, "https://github.com/frostyard/chairlift", Gdk.CURRENT_TIME)
    
    def __on_show_about(self, action, param):
        """Show about dialog"""
        about = Adw.AboutDialog.new()
        about.set_application_name("ChairLift")
        about.set_application_icon("org.frostyard.ChairLift")
        about.set_version("VTESTING")
        about.set_developer_name("Brian Ketelsen")
        about.set_license_type(Gtk.License.GPL_3_0)
        about.set_comments(_("System management and configuration tool"))
        about.set_website("https://github.com/frostyard/chairlift")
        about.set_issue_url("https://github.com/frostyard/chairlift/issues")
        about.set_developers([
            "Brian Ketelsen https://github.com/bketelsen",
        ])
        about.set_copyright("© 2025 FrostYard")
        about.present(self)
    
    def __on_navigate_to_page(self, action, param, page_name):
        """Navigate to a specific page using keyboard shortcut"""
        if page_name in self.pages:
            self.content_stack.set_visible_child(self.pages[page_name])
            # Select the corresponding row in the sidebar
            for i, row in enumerate([self.sidebar_list.get_row_at_index(i) for i in range(5)]):
                if hasattr(row, 'page_name') and row.page_name == page_name:
                    self.sidebar_list.select_row(row)
                    break
            # Show content page in narrow mode
            self.split_view.set_show_content(True)









