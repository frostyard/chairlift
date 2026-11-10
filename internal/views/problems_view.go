package views

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/problems"
	"github.com/frostyard/chairlift/internal/reporting"
	"github.com/frostyard/chairlift/internal/views/pageview"
	"github.com/frostyard/frostui"
)

// reportsColumnWidth is the width of the reporting card beside the
// problem lists.
const reportsColumnWidth = 320

// problemsView is the Problems page: the problem lists on the left, the
// reporting card on the right.
type problemsView struct{ p *Problems }

// NewProblemsPage returns the Problems page.
func NewProblemsPage(s *Store) Page { return &problemsView{p: s.Problems} }

// Badge is the number of problems shown, as a warning.
func (v *problemsView) Badge() (string, frostui.Tone) {
	if n := v.p.Summary().Count; n > 0 {
		return strconv.Itoa(n), frostui.Warning
	}
	return "", frostui.Neutral
}

// Commands are the page's palette entries: reload, turn reports on or off,
// and in Auto mode, send the pending reports.
func (v *problemsView) Commands() []frostui.Command {
	p := v.p
	var cmds []frostui.Command
	if p.dataOn() {
		cmds = append(cmds, frostui.Command{
			ID: "problems-reload", Title: "Reload problems", Group: "Problems", Icon: "view-refresh",
			Hint: "refresh snosi-health crashes", Disabled: p.loading || !p.started, Reason: "loading", Run: p.Reload,
		})
	}
	if p.reportingOn {
		title := "Turn problem reports on"
		if p.consentCopy().Allows() {
			title = "Turn problem reports off"
		}
		cmds = append(cmds, frostui.Command{
			ID: "problems-reporting", Title: title, Group: "Problems", Icon: "mail-send",
			Hint: "consent send problem reports", Disabled: !p.reportingLoaded, Reason: "loading",
			Run: func() { p.onReportingSwitched(!p.consentCopy().Allows()) },
		})
		if consent := p.consentCopy(); consent.Allows() && consent.Mode == reporting.ModeAuto {
			reason := p.sendPendingBlocked()
			cmds = append(cmds, frostui.Command{
				ID: "problems-send-pending", Title: pageview.ReportingSendPending, Group: "Problems", Icon: "mail-send",
				Disabled: reason != "", Reason: reason, Run: func() { p.runAutomaticReports(true) },
			})
		}
	}
	return cmds
}

// View draws the page.
func (v *problemsView) View(c *ui.Context) {
	p := v.p
	now := p.env.Now()
	shown := len(p.system) + len(p.apps)
	subtitle := pageview.ProblemsSubtitle(shown, p.loaded, p.dataOn(), p.reportingOn, p.consentCopy())
	frostui.PageHeader(c, "Problems", subtitle, func() {
		if p.dataOn() && frostui.IconButton(c, "view-refresh", "Reload problems").Disabled(p.loading || !p.started).Clicked() {
			p.Reload()
		}
	})
	ui.Row(c).Gap(frostui.SpaceL).AlignItems(ui.Start).Wrap().Children(func() {
		if p.dataOn() {
			ui.Column(c).Key("problem-lists").Grow(1).Basis(0).MinWidth(420).Gap(10).AlignItems(ui.Stretch).Children(func() {
				if p.systemOn {
					status := p.systemStatus(now)
					notes := pageview.ProblemsSystemNotes(p.outcome, p.generated, len(p.system), p.systemHidden, now, p.loc)
					p.section(c, "system", "computer", pageview.ProblemsSystemSection, pageview.ProblemsSystemSectionNote, notes, status, p.system, now)
				}
				if p.appsOn {
					status := p.appsStatus()
					notes := pageview.ProblemsAppsNotes(p.appsHidden)
					p.section(c, "apps", "avatar-default", pageview.ProblemsAppsSection, pageview.ProblemsAppsSectionNote, notes, status, p.apps, now)
				}
			})
		}
		if p.reportingOn {
			col := ui.Column(c).Key("reports").Gap(frostui.SpaceL).AlignItems(ui.Stretch)
			if p.dataOn() {
				col.Width(reportsColumnWidth)
			} else {
				col.Grow(1).MaxWidth(560)
			}
			col.Children(func() { p.reportsCard(c, now) })
		}
	})
	p.dialogView(c)
}

// section draws one problem list: its header with the status notes, then
// "Loading…", the status row that stands for no items, or the cards.
func (p *Problems) section(c *ui.Context, key, icon, title, note string, notes []string, status pageview.ProblemsStatus, items []problems.Item, now time.Time) {
	ui.Column(c).Key(key).Gap(10).AlignItems(ui.Stretch).Padding(0, 0, 6, 0).Children(func() {
		frostui.SectionHeader(c, frostui.SectionHead{Icon: icon, Title: title, Note: note, Aside: strings.Join(notes, " · "), Quiet: true})
		switch {
		case !p.loaded:
			frostui.Loading(c, pageview.ProblemsLoading)
		case status.Empty != nil:
			p.emptyCard(c, *status.Empty, len(items) == 0 && p.outcome != problems.SourceUnreadable && p.outcome != problems.SourceMalformed)
		default:
			ids := map[string]int{}
			for _, it := range items {
				// A broken file may repeat an id: the key stays unique.
				n := ids[it.ID]
				ids[it.ID] = n + 1
				p.problemCard(c, problemKey{it.ID, n}, it, now)
			}
		}
	})
}

// problemKey identifies a card among its siblings.
type problemKey struct {
	id string
	n  int
}

// emptyCard stands for a list with no item: "No problems found", "System
// checks are not available on this system", or "could not be read".
func (p *Problems) emptyCard(c *ui.Context, row pageview.Row, fine bool) {
	icon := "dialog-information"
	if row.Title == pageview.ProblemsNoneFound {
		icon = "object-select"
	}
	if !fine {
		icon = "dialog-warning"
	}
	frostui.Card(c).Padding(4).Children(func() {
		frostui.Empty(c, icon, row.Title, row.Subtitle)
	})
}

// problemIcon is the icon of a problem's kind.
func problemIcon(it problems.Item) string {
	if !it.KnownKind {
		return "dialog-question"
	}
	switch it.Kind {
	case problems.KindIntegrityError:
		return "security-low"
	case problems.KindBootedOlderImage:
		return "document-open-recent"
	case problems.KindUnitFailed:
		return "network-server"
	case problems.KindTPMUnlock:
		return "dialog-password"
	case problems.KindCrash:
		return "application-x-executable"
	case problems.KindAppProblem:
		return "dialog-information"
	case problems.KindUpdateStageFailed:
		return "software-update-urgent"
	case problems.KindSysextUpdateFailed:
		return "application-x-addon"
	}
	return "dialog-warning"
}

// severityTone is the tone of a severity.
func severityTone(s pageview.Severity) frostui.Tone {
	switch s {
	case pageview.SeverityDanger:
		return frostui.Danger
	case pageview.SeverityWarning:
		return frostui.Warning
	}
	return frostui.Accent
}

// problemCard draws one problem as a card that a click expands: its icon,
// title and "subject · last seen" line, how often it happened, and when
// open, what it means, the recorded values, the local details and its
// actions.
func (p *Problems) problemCard(c *ui.Context, key problemKey, it problems.Item, now time.Time) {
	text := pageview.ProblemCardText(it, now, p.loc)
	open := p.open[it.ID]
	toggled := frostui.DisclosureCard(c.Key(key), &open, frostui.Disclosure{
		Icon: problemIcon(it), Title: text.Title, Subtitle: text.Subtitle,
		Trailing: func() { frostui.Pill(c, text.Count, severityTone(text.Severity)) },
		Body:     func() { p.problemBody(c, it, text) },
	})
	if toggled {
		if open {
			p.open[it.ID] = true
		} else {
			delete(p.open, it.ID)
		}
	}
}

// problemBody is an open card's content: pageview.ProblemDetails (the
// hint, the recorded values, what this computer adds), when it was first
// seen, and Copy details, Report… and Dismiss.
func (p *Problems) problemBody(c *ui.Context, it problems.Item, text pageview.ProblemCard) {
	t := c.Theme()
	details := pageview.ProblemDetails(it)
	if len(details) > 0 {
		frostui.Body(c, details[0].Subtitle).Wrap()
	}
	if len(details) > 1 {
		ui.Column(c).Padding(10, 12).Radius(frostui.RadiusChip).Background(t.Background).Gap(frostui.SpaceS).AlignItems(ui.Stretch).Children(func() {
			for i, row := range details[1:] {
				ui.Column(c.Key(i)).Gap(3).AlignItems(ui.Stretch).Children(func() {
					frostui.Caption(c, row.Title)
					frostui.Mono(c, row.Subtitle).FontSize(12).Wrap().Selectable()
				})
			}
		})
	}
	ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Wrap().Children(func() {
		frostui.Muted(c, text.FirstSeen).Grow(1)
		if frostui.LabeledButton(c, "edit-copy", "Copy details", false).Clicked() {
			p.copyDetails(c.WriteClipboard, it, text.Title)
		}
		switch {
		case p.reported[it.ID]:
			frostui.LabeledButton(c, "object-select", pageview.ReportedLabel, false).Disabled(true)
		case p.reportOffered(it):
			if frostui.LabeledButton(c, "mail-send", pageview.ReportButtonLabel, false).Disabled(p.reportBusy[it.ID]).Clicked() {
				p.reportItem(it, text.Title)
			}
		}
		if frostui.LabeledButton(c, "view-conceal", "Dismiss", false).Disabled(p.dismissing[it.ID]).Clicked() {
			p.dismiss(it, text.Title)
		}
	})
}

// reportsCard is the reporting card: the consent switch, the mode, the
// limits, the send or weekly count action, the privacy link and the sent
// reports.
func (p *Problems) reportsCard(c *ui.Context, now time.Time) {
	t := c.Theme()
	consent := p.consentCopy()
	state := p.stateCopy()
	description := pageview.ReportingDescription(consent)
	if p.reportingEndpointErr != nil {
		description = pageview.ReportingEndpointInvalid
	}
	frostui.Card(c).Gap(14).Children(func() {
		frostui.CardHeader(c, pageview.ReportingCardTitle, description, nil)

		on := consent.Allows()
		if frostui.SwitchRow(c.Key("reporting-switch"), pageview.ReportingSwitchTitle, "", &on).Disabled(!p.reportingLoaded).Changed() {
			p.onReportingSwitched(on)
		}

		mode := modeIndex(consent.Mode)
		ui.Column(c).Gap(6).AlignItems(ui.Stretch).Children(func() {
			frostui.Caption(c, pageview.ReportingModeTitle)
			if ui.Segmented(c.Key("reporting-mode"), &mode, pageview.ReportingModeAsk, pageview.ReportingModeAuto).
				Label(pageview.ReportingModeTitle).Disabled(!p.reportingLoaded || !consent.Allows()).Changed() {
				p.onReportingModeSelected(modeOf(mode))
			}
		})
		frostui.Muted(c, pageview.ReportingLimits()).Wrap()

		if consent.Allows() && consent.Mode == reporting.ModeAuto {
			blocked := p.sendPendingBlocked()
			b := frostui.LabeledButton(c, "mail-send", pageview.ReportingSendPending, false).Disabled(blocked != "")
			if blocked != "" {
				b.Tooltip("Not now: " + blocked)
			}
			if b.Clicked() {
				p.runAutomaticReports(true)
			}
		}
		if row, offered := pageview.ReportingCountRow(consent, state, now); offered {
			ui.Row(c).Gap(frostui.SpaceM).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					frostui.Strong(c, row.Title)
					frostui.Muted(c, row.Subtitle).Wrap()
				})
				if ui.Button(c, pageview.ReportingCountAction).Clicked() {
					p.reviewWeeklyCount()
				}
			})
		}

		what := pageview.ReportingWhat()
		ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
			frostui.Icon(c, "web-browser").TextColor(t.TextMuted)
			ui.Link(c, what.Title, what.Subtitle).Tooltip(what.Subtitle)
		})

		frostui.RowDivider(c)
		p.sentList(c, consent, state)
	})
}

// sentList shows how many reports were sent and, opened, each with its
// date, kind and report ID (pageview.SentRows: newest first, at most 50).
func (p *Problems) sentList(c *ui.Context, consent reporting.Consent, state *reporting.State) {
	t := c.Theme()
	rows := pageview.SentRows(state, p.loc)
	summary := pageview.ReportingSentSummary(state)
	if len(rows) == 0 {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			frostui.Icon(c, "document-send").TextColor(t.TextMuted)
			frostui.Muted(c, summary)
		})
		return
	}
	ui.Collapsible(c.Key("sent-reports"), summary, &p.sentOpen, func() {
		ui.Scroll(c).MaxHeight(260).Children(func() {
			ui.Column(c).Gap(frostui.SpaceS).AlignItems(ui.Stretch).Padding(frostui.SpaceS, 0, 0, 0).Children(func() {
				for i, row := range rows {
					ui.Column(c.Key(i)).Gap(2).AlignItems(ui.Stretch).Children(func() {
						frostui.Body(c, row.Title).FontSize(12.5)
						frostui.Mono(c, row.Subtitle).FontSize(11.5).TextColor(t.TextMuted).Selectable()
					})
				}
			})
		})
	}).Disabled(!consent.Allows())
}

// dialogView draws the Report dialog while it is open: the report's text,
// exactly the bytes Send posts, in a scrollable monospaced box.
func (p *Problems) dialogView(c *ui.Context) {
	if !p.dialog.open {
		return
	}
	d := p.dialog
	send, cancel := frostui.CodeDialog(c, d.title, pageview.ReportSummary(d.report.Kind), d.report.Text(), "Send")
	switch {
	case send:
		p.sendDialogReport()
	case cancel:
		p.cancelReport()
	}
}
