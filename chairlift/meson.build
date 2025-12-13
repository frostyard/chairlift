pkgdatadir = join_paths(get_option('prefix'), get_option('datadir'), meson.project_name())
moduledir = join_paths(pkgdatadir, 'chairlift')
gnome = import('gnome')

gnome.compile_resources('chairlift',
  'chairlift.gresource.xml',
  gresource_bundle: true,
  install: true,
  install_dir: moduledir,
)

python = import('python')

conf = configuration_data()
conf.set('PYTHON', python.find_installation('python3').full_path())
conf.set('VERSION', meson.project_version())
conf.set('localedir', join_paths(get_option('prefix'), get_option('localedir')))
conf.set('pkgdatadir', pkgdatadir)
conf.set('moduledir', moduledir)

configure_file(
  input: 'chairlift.in',
  output: 'chairlift',
  install_mode: 'rwxr-xr-x',
  configuration: conf,
  install: true,
  install_dir: get_option('bindir'),
)

subdir('core')
subdir('scripts')
subdir('views')

chairlift_sources = [
  '__init__.py',
  'application.py',
  'main.py',
  'window.py',
  'dialog.py',
  'config.yml']

install_data(chairlift_sources, install_dir: moduledir)
