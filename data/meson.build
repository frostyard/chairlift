install_data(
    'org.frostyard.ChairLift.policy',
    install_dir: join_paths(get_option('prefix'), get_option('datadir'), 'polkit-1', 'actions')
)

install_data(
    'org.frostyard.ChairLift.rules',
    install_dir: join_paths(get_option('prefix'), get_option('datadir'), 'polkit-1', 'rules.d')
)

desktop_file = i18n.merge_file(
  input: 'org.frostyard.ChairLift.desktop.in',
  output: 'org.frostyard.ChairLift.desktop',
  type: 'desktop',
  po_dir: '../po',
  install: true,
  install_dir: join_paths(get_option('datadir'), 'applications'),
)
custom_target(
  'install-autostart-desktop',
  input: desktop_file,
  output: 'org.frostyard.ChairLift.autostart.desktop',
  command: ['cp', '@INPUT@', '@OUTPUT@'],
  install_dir: join_paths(get_option('sysconfdir'), 'skel', '.config', 'autostart'),
  build_by_default: true,
  install : true,
)

desktop_utils = find_program('desktop-file-validate', required: false)
if desktop_utils.found()
  test('Validate desktop file', desktop_utils,
    args: [desktop_file]
  )
endif



subdir('icons')
