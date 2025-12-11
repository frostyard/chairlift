pkgdatadir = join_paths(get_option('prefix'), get_option('datadir'), meson.project_name())
coredir = join_paths(pkgdatadir, 'chairlift/core')

sources = [
  '__init__.py',
  'backend.py',
  'homebrew.py',
]

install_data(sources, install_dir: coredir)