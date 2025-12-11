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

from gi.repository import Gtk, Adw, GLib

import chairlift.core.backend as backend

from chairlift.dialog import ChairLiftDialog

_ = __builtins__["_"]

@Gtk.Template(resource_path="/org/frostyard/ChairLift/gtk/window.ui")
class ChairLiftWindow(Adw.ApplicationWindow):
    __gtype_name__ = "ChairLiftWindow"

    stack = Gtk.Template.Child()
    toasts = Gtk.Template.Child()
    content_overlay = Gtk.Template.Child()
    style_manager = Adw.StyleManager().get_default()


    pages = []

    def __init__(self, moduledir: str,  **kwargs):
        super().__init__(**kwargs)

        self.moduledir = moduledir

        self.__build_ui()

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

        self.__view_welcome = ChairLiftUserHome(self)

        self.pages.append(self.__view_welcome)

        # Enable window controls in user mode
        self.set_deletable(True)
        # Make content fill the entire window vertically
        self.content_overlay.set_valign(Gtk.Align.FILL)

        for page in self.pages:
            self.stack.add_child(page)

        self.stack.set_visible_child(self.__view_welcome)









