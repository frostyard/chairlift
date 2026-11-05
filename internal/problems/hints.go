package problems

import (
	"sort"
	"strings"

	"github.com/frostyard/chairlift/internal/selfreport"
)

// kindTitles are the row titles of the system kinds, in plain words. crash
// and app_problem titles name the program (Title).
var kindTitles = map[string]string{
	KindUnitFailed:         "A system service failed",
	KindUpdateStageFailed:  "An update could not be prepared",
	KindBootedOlderImage:   "The computer started an older system version",
	KindSysextUpdateFailed: "A system feature could not be updated",
	KindTPMUnlock:          "The disk needed extra steps to unlock",
	KindIntegrityError:     "System files failed an integrity check",
	KindCrash:              "A program stopped unexpectedly",
	KindAppProblem:         "An app reported a problem",
	KindCount:              "A usage count was recorded",
}

// unknownKindTitle is the title of a kind that schema v1 does not know.
const unknownKindTitle = "An unknown problem was recorded"

// kindHints are the first sentence of each kind's hint.
var kindHints = map[string]string{
	KindUnitFailed:         "A service that is part of the system stopped with an error.",
	KindUpdateStageFailed:  "The system tried to download and prepare a new version, and it did not finish.",
	KindBootedOlderImage:   "A newer system version is installed, but the computer started an older one.",
	KindSysextUpdateFailed: "A system feature (a system extension) was not updated.",
	KindTPMUnlock:          "The encrypted disk did not unlock with the security chip alone.",
	KindIntegrityError:     "The system found files that do not match the signed system image.",
	KindCrash:              "A program ended because of an error in the program.",
	KindAppProblem:         "A frostyard app recorded a problem.",
	KindCount:              "A count of active installations. It holds no problem.",
}

// unknownKindHint is the hint of a kind that schema v1 does not know.
const unknownKindHint = "A newer system recorded a kind of problem that this version of ChairLift does not know. It stays on this computer."

// valueHints are hints for 1 value of 1 field of 1 kind, keyed
// "kind.field=value". TestEveryEnumValueHasAHint parses the enums of
// core's v1.json and requires an entry for each.
var valueHints = map[string]string{
	// S1 unit_failed result
	"unit_failed.result=exit-code":       "The service exited with an error code.",
	"unit_failed.result=signal":          "The service was stopped by a signal.",
	"unit_failed.result=timeout":         "The service did not start or stop in time.",
	"unit_failed.result=core-dump":       "The service crashed.",
	"unit_failed.result=start-limit-hit": "The service failed too often in a short time, so the system stopped trying.",
	"unit_failed.result=other":           "The service failed for another reason.",
	// S2 update_stage_failed error_class
	"update_stage_failed.error_class=network":    "The download failed. Check the network connection; the next check tries again.",
	"update_stage_failed.error_class=registry":   "The update server did not give the new version. The next check tries again.",
	"update_stage_failed.error_class=signature":  "The new version failed its signature check and was not used.",
	"update_stage_failed.error_class=disk_space": "There is not enough free disk space for the new version.",
	"update_stage_failed.error_class=lock":       "Another update was running at the same time. The next check tries again.",
	"update_stage_failed.error_class=other":      "The update failed for another reason.",
	// S3 booted_older_image how
	"booted_older_image.how=manual_rollback": "Someone rolled back to the previous version.",
	"booted_older_image.how=boot_menu":       "The older version was chosen in the boot menu.",
	"booted_older_image.how=unknown":         "It is not known how the older version was chosen.",
	"booted_older_image.how=other":           "The older version was chosen in another way.",
	// S4 sysext_update_failed outcome
	"sysext_update_failed.outcome=failed":              "The update of the feature failed.",
	"sysext_update_failed.outcome=held_by_rollback":    "The feature is held at its version because the system rolled back.",
	"sysext_update_failed.outcome=needs_image":         "The new feature version needs a newer system version first.",
	"sysext_update_failed.outcome=skipped_unreachable": "The feature server could not be reached, so the update was skipped.",
	"sysext_update_failed.outcome=skipped_busy":        "The feature was in use, so the update was skipped.",
	"sysext_update_failed.outcome=other":               "The feature update ended in another way.",
	// S4 sysext_update_failed error_class
	"sysext_update_failed.error_class=network":     "A network error stopped the download.",
	"sysext_update_failed.error_class=http_status": "The feature server answered with an error.",
	"sysext_update_failed.error_class=signature":   "The download failed its signature check and was not used.",
	"sysext_update_failed.error_class=checksum":    "The download was damaged and was not used.",
	"sysext_update_failed.error_class=disk_space":  "There is not enough free disk space.",
	"sysext_update_failed.error_class=link":        "The new version could not be put in place.",
	"sysext_update_failed.error_class=config":      "The feature configuration has an error.",
	"sysext_update_failed.error_class=other":       "Another error stopped the update.",
	// S5 tpm_unlock outcome
	"tpm_unlock.outcome=ok_after_retry": "The disk unlocked after more tries.",
	"tpm_unlock.outcome=recovery_key":   "The recovery key was needed to unlock the disk.",
	"tpm_unlock.outcome=passphrase":     "The passphrase was needed to unlock the disk.",
	"tpm_unlock.outcome=other":          "The disk was unlocked in another way.",
	// S5 tpm_unlock tpm_rc
	"tpm_unlock.tpm_rc=PCR_CHANGED": "The measured boot state changed, for example after a firmware or boot update.",
	"tpm_unlock.tpm_rc=POLICY_FAIL": "The security chip did not accept its unlock rule.",
	"tpm_unlock.tpm_rc=LOCKOUT":     "The security chip locked itself after too many failed tries.",
	"tpm_unlock.tpm_rc=other":       "The security chip reported another error.",
	// S6 integrity_error subsystem
	"integrity_error.subsystem=composefs": "The check of the system file tree (composefs) failed.",
	"integrity_error.subsystem=fs-verity": "The check of a single file (fs-verity) failed.",
	"integrity_error.subsystem=dm-verity": "The check of a disk block (dm-verity) failed. The disk can be damaged.",
	"integrity_error.subsystem=other":     "Another integrity check failed.",
	// S7 crash signal
	"crash.signal=SIGSEGV": "The program read or wrote memory that it may not use.",
	"crash.signal=SIGABRT": "The program stopped itself after it found an error.",
	"crash.signal=SIGBUS":  "The program used memory that could not be read.",
	"crash.signal=SIGILL":  "The program ran an instruction that the processor does not know.",
	"crash.signal=SIGFPE":  "The program made an arithmetic error, such as a division by zero.",
	"crash.signal=SIGTRAP": "The program stopped at a debug trap.",
	"crash.signal=SIGSYS":  "The program made a system call that is not allowed.",
	"crash.signal=other":   "The program ended because of another signal.",
	// S8 app_problem class
	"app_problem.class=panic": "The app stopped unexpectedly because of an error in the app.",
	"app_problem.class=error": "The app found an internal error and recorded it.",
}

// codeHints are hints for the known S8 codes: ChairLift's own codes and the
// first-wave codes of plan T857. An unknown code gets the hint of its class.
var codeHints = map[string]string{
	selfreport.CodeConfigLoadFailed:       "ChairLift could not use its configuration file, so it turned all features off. Fix the file it names and start ChairLift again.",
	selfreport.CodeProblemsSourceInvalid:  "ChairLift could not read the list of system problems.",
	selfreport.CodeFirstRunCompleteFailed: "ChairLift could not record that the first-run setup finished.",
	selfreport.CodeUpdexResultsUnreadable: "A feature update finished, but ChairLift could not read its results.",
	"store.migrate_failed":                "The app could not update its stored data to the new format.",
	"sync.apply_failed":                   "The app could not apply changes it received.",
	"mime.parse_failed":                   "The app could not read a message.",
	"secrets.unseal_failed":               "The app could not open its stored secrets.",
	"bridge.socket_lost":                  "The app lost the connection to one of its parts.",
	"merge.failed":                        "The app could not combine two versions of the same data.",
	"vault.write_failed":                  "The app could not save data to its vault.",
	"watch.overflow_rescan":               "The app missed file changes and had to scan again.",
	"sync.protocol_error":                 "The app got an answer it did not understand while it synchronized.",
	"extract.failed":                      "The app could not read the content of a file.",
	"render.too_large":                    "Content was too large for the app to show.",
	"lock.held":                           "Another copy of the app holds a lock.",
	"blobs.sweep_failed":                  "The app could not remove old stored files.",
}

// KnownCodes returns the codes that have their own hint, sorted.
func KnownCodes() []string {
	codes := make([]string, 0, len(codeHints))
	for code := range codeHints {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

// hintFields are the fields whose value hint each kind shows, in order.
var hintFields = map[string][]string{
	KindUnitFailed:         {"result"},
	KindUpdateStageFailed:  {"error_class"},
	KindBootedOlderImage:   {"how"},
	KindSysextUpdateFailed: {"outcome", "error_class"},
	KindTPMUnlock:          {"outcome", "tpm_rc"},
	KindIntegrityError:     {"subsystem"},
	KindCrash:              {"signal"},
	KindAppProblem:         {"class"},
}

// appNames are the display names of the allowlisted app IDs.
var appNames = map[string]string{
	"org.frostyard.ChairLift":  "ChairLift",
	"org.frostyard.nsl":        "nsl",
	"org.frostyard.intuneme":   "intuneme",
	"org.frostyard.pilothouse": "Pilothouse",
	"org.frostyard.Frostmail":  "Frostmail",
	"org.frostyard.Frostnotes": "Frostnotes",
	"org.frostyard.Frostread":  "Frostread",
}

// AppName returns the display name of an allowlisted app ID, or "".
func AppName(appID string) string {
	return appNames[appID]
}

// Title returns the row title of an item in plain words, such as "An update
// could not be prepared" or "Frostmail stopped unexpectedly".
func Title(it Item) string {
	if !it.KnownKind {
		return unknownKindTitle
	}
	switch it.Kind {
	case KindCrash:
		if name := ProgramName(it); name != "" {
			return name + " stopped unexpectedly"
		}
	case KindAppProblem:
		if name := AppName(it.ValidString("app_id")); name != "" {
			if it.ValidString("class") == "error" {
				return name + " reported an error"
			}
			return name + " stopped unexpectedly"
		}
	}
	return kindTitles[it.Kind]
}

// ProgramName returns the program name of a crash item: the exe field's
// text, else the basename of the local executable path.
func ProgramName(it Item) string {
	if f, ok := it.Field("exe"); ok && f.Display != "" {
		return f.Display
	}
	if it.Local.ExePath != "" {
		parts := strings.Split(it.Local.ExePath, "/")
		return cutDisplay(parts[len(parts)-1])
	}
	return ""
}

// Hint returns the hint of an item: the kind's sentence, then the sentence
// of each hinted field value, then, for an app error, the code's hint (or
// nothing more for an unknown code, whose class hint already stands).
func Hint(it Item) string {
	if !it.KnownKind {
		return unknownKindHint
	}
	parts := []string{kindHints[it.Kind]}
	for _, name := range hintFields[it.Kind] {
		value := it.ValidString(name)
		if value == "" {
			continue
		}
		if h, ok := valueHints[it.Kind+"."+name+"="+value]; ok {
			parts = append(parts, h)
		}
	}
	if it.Kind == KindAppProblem && it.ValidString("class") == "error" {
		if h, ok := codeHints[it.ValidString("code")]; ok {
			parts = append(parts, h)
		}
	}
	return strings.Join(parts, " ")
}
