package i18n

// The msgids of the first run. Every string the first-run window shows goes
// through T with one of these constants. Ported msgids are byte-identical to
// frostyard/first-setup's, so their translations apply; the "new" ones start
// untranslated. TestEveryEmbeddedMsgidIsAFirstRunMsgid fails when an
// embedded .po file holds a msgid that is not in Messages.
const (
	// Header bar (ported).
	MsgBack = "Back"
	MsgNext = "Next"

	// Welcome step (ported). MsgHello carries one "{}" placeholder for the
	// user's name; use Format.
	MsgHello         = "Hello {}!"
	MsgWelcomeDesc   = "Let's set up your user account"
	MsgStart         = "Start"
	MsgAccessibility = "Accessibility"

	// Connectivity step (ported).
	MsgCheckingConnection  = "Checking Connection"
	MsgCheckingDesc        = "Please wait until the connection check is done"
	MsgOpenSettings        = "Open Settings"
	MsgNoInternet          = "No Internet Connection!"
	MsgConnectionAvailable = "Connection available"
	MsgConnectionDesc      = "You have a working internet connection"
	// Connectivity step (new).
	MsgOfflineDesc = "You can continue without a connection and install applications later from ChairLift"
	MsgCheckAgain  = "Check Again"

	// Style step (ported).
	MsgColorScheme     = "Color Scheme"
	MsgColorSchemeDesc = "Choose a color scheme for your system"
	MsgLight           = "Light"
	MsgLightMode       = "Light Mode"
	MsgDark            = "Dark"
	MsgDarkMode        = "Dark Mode"

	// Apps step (ported).
	MsgApplications     = "Applications"
	MsgApplicationsDesc = "Choose which applications to install."
	MsgSelectApps       = "Select the applications you want to install"
	MsgBrowsers         = "Browsers"
	MsgBrowsersDesc     = "Surf the web with your favorite browsers"
	MsgUtilities        = "Common Utilities"
	MsgUtilitiesDesc    = "Useful utilities to enhance your desktop experience"
	MsgOffice           = "Office"
	MsgOfficeDesc       = "The LibreOffice suite"

	// Progress step (ported).
	MsgProgress     = "Progress"
	MsgProgressDesc = "Some steps still take some time"
	MsgInstalling   = "Installing"
	// Progress step (new).
	MsgSettingUpFlathub = "Setting up Flathub"

	// Done step (ported).
	MsgFinished     = "Finished!"
	MsgFinishedDesc = "Your user account is ready to use"
	MsgExit         = "Exit"
	MsgSetupFailed  = "Setup failed: "
	MsgErrorLog     = "Error log"
	// Done step (new): the dry-run description (ADR-0009 preview wording).
	MsgDryRunFinished = "[DRY-RUN] Preview: first run finished — no changes made"
	// Done step (new): the completion could not be recorded.
	MsgCompletionFailed     = "Could not record that the first run is finished"
	MsgCompletionFailedDesc = "The first run starts again at your next login"
	// Done step (new): the completion failed, and the next login does not
	// start the first run again (a marker exists) or that is unknown (no
	// config home).
	MsgFinishedWithErrors = "Setup finished with errors"
)

// Messages lists every first-run msgid, ported and new.
var Messages = []string{
	MsgBack, MsgNext,
	MsgHello, MsgWelcomeDesc, MsgStart, MsgAccessibility,
	MsgCheckingConnection, MsgCheckingDesc, MsgOpenSettings, MsgNoInternet,
	MsgConnectionAvailable, MsgConnectionDesc, MsgOfflineDesc, MsgCheckAgain,
	MsgColorScheme, MsgColorSchemeDesc, MsgLight, MsgLightMode, MsgDark, MsgDarkMode,
	MsgApplications, MsgApplicationsDesc, MsgSelectApps,
	MsgBrowsers, MsgBrowsersDesc, MsgUtilities, MsgUtilitiesDesc, MsgOffice, MsgOfficeDesc,
	MsgProgress, MsgProgressDesc, MsgInstalling, MsgSettingUpFlathub,
	MsgFinished, MsgFinishedDesc, MsgExit, MsgSetupFailed, MsgErrorLog, MsgDryRunFinished,
	MsgCompletionFailed, MsgCompletionFailedDesc, MsgFinishedWithErrors,
}
