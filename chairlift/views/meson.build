pkgdatadir = join_paths(get_option('prefix'), get_option('datadir'), meson.project_name())
viewsdir = join_paths(pkgdatadir, 'chairlift/views')

sources = [
  '__init__.py',
  'user_home.py',
]

install_data(sources, install_dir: viewsdir)
