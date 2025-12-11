# main.py
#
# Copyright 2025 mirkobrombin
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

import sys
import os
import signal
import locale
import gettext

from gi.repository import Gio

def main(version, moduledir: str, localedir: str):
    """The application's entry point."""
    if moduledir == "":
        print("Can't continue without a data directory.")
        sys.exit(1)
        return

    signal.signal(signal.SIGINT, signal.SIG_DFL)
    locale.bindtextdomain('chairlift', localedir)
    locale.textdomain('chairlift')
    gettext.install('chairlift', localedir)

    resource = Gio.Resource.load(os.path.join(moduledir, 'chairlift.gresource'))
    resource._register()

    import chairlift.core.backend as backend
    from chairlift.application import ChairLiftApplication

    backend.set_script_path(os.path.join(moduledir, "scripts"))
    app = ChairLiftApplication(moduledir)
    return app.run(sys.argv)
