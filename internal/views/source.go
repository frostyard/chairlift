package views

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/frostui"
)

// SourceID names one of the five update sources, in the order the Updates
// page and "Update everything" run them.
type SourceID string

// The update sources. They are also the Source of their jobs and of their
// activity entries.
const (
	SourceImage    SourceID = "image"
	SourceFeatures SourceID = "features"
	SourceFlatpak  SourceID = "flatpak"
	SourceBrew     SourceID = "brew"
	SourceFirmware SourceID = "firmware"
)

// UpdateItem is one thing a source can update: an image, a feature, an app,
// a package, a device.
type UpdateItem struct {
	// Key identifies the item within its source; jobs carry it.
	Key    string
	Title  string
	Detail string // an ID, a tap, a vendor
	From   string // the version installed, "" when unknown
	To     string // the version offered, "" when unknown
	// Held items are left out of "Update everything" and their button is
	// off: a package of an untrusted tap, a pinned formula. HeldWhy says
	// why, in a few words.
	Held    bool
	HeldWhy string
	// Confirm items are updated one at a time after a confirmation and
	// never by "Update everything": firmware.
	Confirm bool
	// Action labels the row's button: "Update", "Stage", "Install…".
	Action string
}

// SourceStatus is the state of one source's last check.
type SourceStatus struct {
	// Available is false when the source does not apply here: its group
	// is disabled, its tool is missing, or the host is not that kind. The
	// Updates page leaves it out.
	Available bool
	// Checking is true while a check runs.
	Checking bool
	// Err is the last check's failure, "" when it worked.
	Err string
	// CheckedAt is when the last check ended, zero before one did.
	CheckedAt time.Time
	// Note is one line under the source's title, as "Staged 2026.11.08 ·
	// applies on restart". It may be empty.
	Note string
}

// UpdateSource is one of the five sources of the Updates page. Every
// method runs on the UI thread.
type UpdateSource interface {
	ID() SourceID
	Title() string
	Icon() string
	// Blurb says in one sentence what the source updates.
	Blurb() string
	Status() SourceStatus
	// Items are what can be updated now, as the last check found.
	Items() []UpdateItem
	// Refresh starts a check, unless one runs.
	Refresh()
	// Jobs returns the jobs that update the items with the given keys, in
	// order. With no keys it returns what "Update everything" covers: no
	// Held and no Confirm item. Each job's Done applies its result to the
	// source's state (and records activity); the caller queues them.
	Jobs(keys ...string) []*frostui.Job
	// Update updates one item from its row: it may ask first (firmware),
	// then queues the job.
	Update(key string)
	// Extra draws what only this source shows on the Updates page under
	// its card's header: a banner to trust a tap, the staged image,
	// blocked firmware. It may draw nothing.
	Extra(c *ui.Context)
}

// Page is one page of the rail. Every method runs on the UI thread.
type Page interface {
	// View draws the page's content; the shell scrolls it.
	View(c *ui.Context)
	// Commands are the page's entries in the command palette.
	Commands() []frostui.Command
	// Badge is the count at the page's rail item, "" for none.
	Badge() (string, frostui.Tone)
}

// ProblemSummary is what the Overview and the status bar show of the
// Problems page.
type ProblemSummary struct {
	// Known is false until the first load ended, or when the page's
	// groups are disabled.
	Known bool
	// Count is the number of problems shown; Attention those that need
	// the user (system problems of the current boot, say).
	Count, Attention int
}

// FeatureSummary is what the Overview shows of the Features page.
type FeatureSummary struct {
	Known     bool
	On, Total int
}

// Deployment is one system image of the deployment chain.
type Deployment struct {
	// Present is false for an empty slot (nothing staged).
	Present bool
	Version string
	Digest  string
	Pinned  bool
}

// SystemInfo is what the Overview hero, the status bar and Maintenance
// show of the host.
type SystemInfo struct {
	Loaded bool
	// Kind is "bootc", "native A/B" or "" for neither.
	Kind      string
	OSName    string // "Snow Linux"
	OSVersion string
	// Channel is the image's tag or update channel, as "stable".
	Channel  string
	ImageRef string
	Booted   Deployment
	Staged   Deployment
	Rollback Deployment
	Hostname string
	Kernel   string
	Model    string
	// SecureBoot is "on", "off" or "" when unknown.
	SecureBoot string
	// Uptime is "up 3d 4h 12m", "" when unknown.
	Uptime string
}
