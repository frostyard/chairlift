package views

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
	"github.com/frostyard/chairlift/internal/selfreport"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/pageview"

	sgtk "github.com/frostyard/snowkit/gtk"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gtk"
)

// buildProblemsPage builds the Problems page (ADR-0015): the system
// problems of snosi-health, the user's crashes and app problem records, and
// the reporting group. It only shows records on this computer; nothing is
// sent. All text comes from internal/views/pageview and internal/problems.
func (uh *UserHome) buildProblemsPage() {
	page := uh.problemsPrefsPage
	if page == nil {
		return
	}

	systemOn := uh.config.IsGroupEnabled("problems_page", "problems_system_group")
	appsOn := uh.config.IsGroupEnabled("problems_page", "problems_apps_group")

	if systemOn {
		group := adw.NewPreferencesGroup()
		group.SetTitle(pageview.ProblemsSystemTitle)
		group.SetDescription(pageview.ProblemsLoading)
		page.Add(group)
		uh.problemsSystemGroup = group
	}

	if appsOn {
		group := adw.NewPreferencesGroup()
		group.SetTitle(pageview.ProblemsAppsTitle)
		group.SetDescription(pageview.ProblemsLoading)
		page.Add(group)
		uh.problemsAppsGroup = group
	}

	if uh.config.IsGroupEnabled("problems_page", "problems_reporting_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle(pageview.ProblemsReportingTitle)
		group.Add(problemsTextRow(pageview.ProblemsReporting()))
		page.Add(group)
	}

	if !systemOn && !appsOn {
		log.Println(problems.BuiltMarker(0, 0, "disabled"))
		return
	}
	go uh.loadProblems(systemOn, appsOn)
}

// loadProblems reads every source off the GTK thread, then fills the
// groups on the main thread and logs the page's built marker.
func (uh *UserHome) loadProblems(systemOn, appsOn bool) {
	dryRun := IsDryRun()
	fixtures := problems.DecideFixtures(os.Getenv(problems.FixturesEnv), dryRun)
	if fixtures.Log != "" {
		log.Println(fixtures.Log)
	}

	seen := problems.NewSeen("")
	if path, err := problems.SeenPath(os.Getenv); err != nil {
		log.Printf("problems: dismissals are not kept: %v", err)
	} else if loaded, err := problems.LoadSeen(path); err != nil {
		log.Printf("problems: dismiss store not read: %v", err)
		seen = loaded
	} else {
		seen = loaded
	}

	now := time.Now()
	snap := problems.Load(context.Background(), problems.Options{
		Now:      now,
		UID:      os.Getuid(),
		System:   systemOn,
		Apps:     appsOn,
		Fixtures: fixtures,
		Seen:     seen,
	})
	for _, line := range snap.LogLines() {
		log.Println(line)
	}
	if err := snap.SourceInvalid(); err != nil {
		selfreport.Error(selfreport.CodeProblemsSourceInvalid, err)
	}

	sgtk.RunOnMainThread(func() {
		systemCount, appsCount, source := 0, 0, "disabled"
		if group := uh.problemsSystemGroup; group != nil {
			status := pageview.ProblemsSystemStatus(snap.System.Outcome, snap.System.Generated,
				len(snap.SystemItems), snap.SystemHidden, now, time.Local)
			uh.showProblems(group, status, snap.SystemItems, seen, now)
			systemCount, source = len(snap.SystemItems), snap.System.Outcome.String()
		}
		if group := uh.problemsAppsGroup; group != nil {
			status := pageview.ProblemsAppsStatus(len(snap.AppItems), snap.AppsHidden)
			uh.showProblems(group, status, snap.AppItems, seen, now)
			appsCount = len(snap.AppItems)
		}
		log.Println(problems.BuiltMarker(systemCount, appsCount, source))
	})
}

// showProblems sets a group's description and adds its rows. Main thread
// only.
func (uh *UserHome) showProblems(group *adw.PreferencesGroup, status pageview.ProblemsStatus, items []problems.Item, seen *problems.Seen, now time.Time) {
	group.SetDescription(glib.MarkupEscapeText(status.Description, -1))
	if status.Empty != nil {
		group.Add(problemsTextRow(*status.Empty))
	}
	for _, item := range items {
		uh.addProblemRow(group, item, seen, now)
	}
}

// addProblemRow adds 1 expander row for item, with its hint, its values,
// its local details, and the Copy details and Dismiss buttons.
func (uh *UserHome) addProblemRow(group *adw.PreferencesGroup, item problems.Item, seen *problems.Seen, now time.Time) {
	text := pageview.Problem(item, now, time.Local)
	expander := adw.NewExpanderRow()
	expander.SetTitle(glib.MarkupEscapeText(text.Title, -1))
	expander.SetSubtitle(glib.MarkupEscapeText(text.Subtitle, -1))

	for _, detail := range pageview.ProblemDetails(item) {
		expander.AddRow(problemsTextRow(detail))
	}

	actions := adw.NewActionRow()
	copyBtn := gtk.NewButtonWithLabel("Copy details")
	copyBtn.SetValign(gtk.AlignCenterValue)
	copyClickedCb := func(btn gtk.Button) {
		btn.GetClipboard().SetText(problems.DetailsText(item))
		uh.toastAdder.ShowToast(actionmsg.ProblemDetailsCopied(text.Title))
	}
	copyBtn.ConnectClicked(&copyClickedCb)
	actions.AddSuffix(&copyBtn.Widget)

	dismissBtn := gtk.NewButtonWithLabel("Dismiss")
	dismissBtn.SetValign(gtk.AlignCenterValue)
	dismissClickedCb := func(btn gtk.Button) {
		decision := actionmsg.ProblemDismiss(IsDryRun(), item.ID, text.Title)
		if decision.Log != "" {
			log.Println(decision.Log)
		}
		if !decision.Persist {
			uh.toastAdder.ShowToast(decision.Toast)
			return
		}
		dismissBtn.SetSensitive(false)
		at := problems.DismissAt(item, time.Now())
		go func() {
			err := seen.Dismiss(item.ID, at)
			sgtk.RunOnMainThread(func() {
				if err != nil {
					log.Printf("problems: dismiss %s: %v", item.ID, err)
					dismissBtn.SetSensitive(true)
					uh.toastAdder.ShowErrorToast(actionmsg.ProblemDismissFailed(text.Title, err))
					return
				}
				if decision.Hide {
					group.Remove(&expander.Widget)
				}
				uh.toastAdder.ShowToast(decision.Toast)
			})
		}()
	}
	dismissBtn.ConnectClicked(&dismissClickedCb)
	actions.AddSuffix(&dismissBtn.Widget)
	expander.AddRow(&actions.Widget)

	group.Add(&expander.Widget)
}

// problemsTextRow returns a plain row; its text is escaped.
func problemsTextRow(text pageview.Row) *gtk.Widget {
	row := adw.NewActionRow()
	row.SetTitle(glib.MarkupEscapeText(text.Title, -1))
	if text.Subtitle != "" {
		row.SetSubtitle(glib.MarkupEscapeText(text.Subtitle, -1))
	}
	return &row.Widget
}
