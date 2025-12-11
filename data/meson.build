install_data(
    'org.frostyard.ChairLift.policy',
    install_dir: join_paths(get_option('prefix'), get_option('datadir'), 'polkit-1', 'actions')
)

install_data(
    'org.frostyard.ChairLift.rules',
    install_dir: join_paths(get_option('prefix'), get_option('datadir'), 'polkit-1', 'rules.d')
)

# Configure and install the wrapper script
wrapper_conf = configuration_data()
wrapper_conf.set('bindir', join_paths(get_option('prefix'), get_option('bindir')))

configure_file(
  input: 'chairlift-wrapper.sh',
  output: 'chairlift-wrapper',
  configuration: wrapper_conf,
  install: true,
  install_dir: get_option('bindir'),
  install_mode: 'rwxr-xr-x',
)

desktop_file = i18n.merge_file(
  input: 'org.frostyard.ChairLift.desktop.in',
  output: 'org.frostyard.ChairLift.desktop',
  type: 'desktop',
  po_dir: '../po',
  install: true,
  install_dir: join_paths(get_option('datadir'), 'applications'),
)
desktop_utils = find_program('desktop-file-validate', required: false)
if desktop_utils.found()
  test('Validate desktop file', desktop_utils,
    args: [desktop_file]
  )
endif



subdir('icons')
