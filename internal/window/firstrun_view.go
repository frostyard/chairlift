package window

import (
	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/firstrun"
	"github.com/frostyard/chairlift/internal/i18n"
	"github.com/frostyard/frostui"
)

// The first-run wizard's metrics, in DIPs. Its toasts (frostui.TopToasts)
// grow as wide as the card, frostui.ToastWidth.
const (
	wizardWidth     = frostui.ToastWidth
	wizardMaxHeight = 640
)

// View draws the window: one card centered on the window's background,
// with the step indicator, the step's icon, title and description, its
// content, and Back and Next under it.
func (w *FirstRun) View(c *ui.Context) {
	t := frostui.Adapt(c)
	w.start()
	for _, toast := range w.toasts {
		c.AddToast(toast)
	}
	w.toasts = nil
	// At the top, with a close button: MyGo's own toasts would cover Back
	// and Next, and the persistent configuration error would block Next.
	frostui.TopToasts(c, i18n.T(i18n.MsgClose))

	step := w.flow.Current()
	ui.Column(c).Fill().Background(t.Background).Padding(frostui.SpaceXL).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		// The kit's card: the panel face over a hairline, square and with
		// no shadow, as the brand draws every card (frostui ADR-0011).
		frostui.Card(c).WidthPercent(100).MaxWidth(wizardWidth).Grow(1).MaxHeight(wizardMaxHeight).
			Padding(28, 32, 24, 32).Gap(18).Children(func() {
			w.stepDots(c)
			// Keyed by step, so each step starts with fresh local state
			// (focus, scroll) and the Done step's Exit takes the focus.
			ui.Column(c.Key(step.String())).Grow(1).Basis(0).Gap(18).AlignItems(ui.Stretch).Children(func() {
				w.stepView(c, step)
			})
			w.footer(c, step)
		})
	})
}

// stepView draws the current step.
func (w *FirstRun) stepView(c *ui.Context, step firstrun.Step) {
	switch step {
	case firstrun.StepWelcome:
		w.welcomeStep(c)
	case firstrun.StepConnectivity:
		w.connectivityStep(c)
	case firstrun.StepStyle:
		frostui.Hero(c, frostui.HeroIcon{Name: "applications-graphics"}, i18n.T(i18n.MsgColorScheme), i18n.T(i18n.MsgColorSchemeDesc))
		w.styleStep(c)
	case firstrun.StepReporting:
		frostui.Hero(c, frostui.HeroIcon{Name: "send-to"}, i18n.T(i18n.MsgReportingTitle), i18n.T(i18n.MsgReportingDesc))
		w.reportingStep(c)
	case firstrun.StepApps:
		frostui.Hero(c, frostui.HeroIcon{Name: "system-software-install"}, i18n.T(i18n.MsgApplications), i18n.T(i18n.MsgApplicationsDesc))
		w.appsStep(c)
	case firstrun.StepProgress:
		frostui.Hero(c, frostui.HeroIcon{Name: "folder-download", Busy: !w.flow.ProgressFinished()}, i18n.T(i18n.MsgProgress), i18n.T(i18n.MsgProgressDesc))
		w.progressStep(c)
	case firstrun.StepDone:
		w.doneStep(c)
	}
}

// stepDots is the step indicator: one dot per visible step, the current
// one drawn long in the accent. Apps joins or leaves the row as the
// connectivity check decides.
func (w *FirstRun) stepDots(c *ui.Context) {
	steps := w.flow.VisibleSteps()
	current := 0
	for i, s := range steps {
		if s == w.flow.Current() {
			current = i
		}
	}
	frostui.StepDots(c, len(steps), current)
}

// footer holds Back and Next. Welcome and Done have their own buttons in
// the content (Start, Exit), as the GTK window had; Back shows when the
// flow allows it, Next is disabled until the flow allows it.
func (w *FirstRun) footer(c *ui.Context, step firstrun.Step) {
	if step == firstrun.StepWelcome || step == firstrun.StepDone {
		return
	}
	ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
		// Keyed: Next keeps its identity, and so the keyboard focus, when
		// Back comes and goes.
		if w.flow.CanBack() {
			ui.Button(c.Key("back"), i18n.T(i18n.MsgBack)).OnClick(w.goBack)
		}
		ui.Spacer(c.Key("spacer"))
		ui.PrimaryButton(c.Key("next"), i18n.T(i18n.MsgNext)).Disabled(!w.flow.CanNext()).OnClick(w.goNext)
	})
}

// welcomeStep greets the user by name, with Start and, when GNOME Settings
// is installed, Accessibility.
func (w *FirstRun) welcomeStep(c *ui.Context) {
	ui.Column(c).Grow(1).Gap(28).Justify(ui.Center).AlignItems(ui.Stretch).Children(func() {
		frostui.Hero(c, frostui.HeroIcon{Logo: w.logo}, i18n.Format(i18n.T(i18n.MsgHello), w.name), i18n.T(i18n.MsgWelcomeDesc))
		ui.Column(c).Gap(frostui.SpaceM).AlignItems(ui.Center).Children(func() {
			ui.PrimaryButton(c, i18n.T(i18n.MsgStart)).MinWidth(200).AutoFocus().OnClick(w.goNext)
			if w.settings {
				frostui.LabeledButton(c, "preferences-desktop-accessibility", i18n.T(i18n.MsgAccessibility), false).MinWidth(200).
					OnClick(func() { w.launchSettings(firstrun.PanelAccessibility) })
			}
		})
	})
}

// connectivityStep shows the connectivity status. It is status only:
// offline, the user can go on and the Apps step is skipped.
func (w *FirstRun) connectivityStep(c *ui.Context) {
	ui.Column(c).Grow(1).Gap(24).Justify(ui.Center).AlignItems(ui.Stretch).Children(func() {
		offline := false
		switch w.flow.Connectivity() {
		case firstrun.ConnChecking:
			frostui.Hero(c, frostui.HeroIcon{Busy: true}, i18n.T(i18n.MsgCheckingConnection), i18n.T(i18n.MsgCheckingDesc))
		case firstrun.ConnOnline:
			frostui.Hero(c, frostui.HeroIcon{Name: "network-transmit-receive", Tone: frostui.Success}, i18n.T(i18n.MsgConnectionAvailable), i18n.T(i18n.MsgConnectionDesc))
		case firstrun.ConnOffline:
			offline = true
			frostui.Hero(c, frostui.HeroIcon{Name: "network-offline", Tone: frostui.Warning}, i18n.T(i18n.MsgNoInternet), i18n.T(i18n.MsgOfflineDesc))
		}
		if !offline {
			return
		}
		ui.Row(c).Gap(frostui.SpaceS).Justify(ui.Center).Children(func() {
			frostui.LabeledButton(c, "view-refresh", i18n.T(i18n.MsgCheckAgain), false).OnClick(w.startConnectivityCheck)
			if w.settings {
				frostui.LabeledButton(c, "network-wireless", i18n.T(i18n.MsgOpenSettings), false).
					OnClick(func() { w.launchSettings(firstrun.PanelWifi) })
			}
		})
	})
}

// styleStep offers Light and Dark as two preview tiles. A choice applies
// at once, as in first-setup.
func (w *FirstRun) styleStep(c *ui.Context) {
	shown := w.chosen
	if shown == "" {
		// Before the first choice the step shows the desktop's scheme, as
		// the GTK window read its style manager.
		shown = firstrun.SchemeLight
		if c.Theme().Dark {
			shown = firstrun.SchemeDark
		}
	}
	ui.Row(c).Gap(frostui.SpaceL).Justify(ui.Center).Role(ui.RoleRadioGroup).Label(i18n.T(i18n.MsgColorScheme)).Children(func() {
		for _, s := range []struct {
			scheme         firstrun.ColorScheme
			label, tooltip string
		}{
			{firstrun.SchemeLight, i18n.T(i18n.MsgLight), i18n.T(i18n.MsgLightMode)},
			{firstrun.SchemeDark, i18n.T(i18n.MsgDark), i18n.T(i18n.MsgDarkMode)},
		} {
			selected := s.scheme == shown
			scheme := s.scheme
			schemeTile(c.Key(string(scheme)), scheme == firstrun.SchemeDark, s.label, s.tooltip, selected).OnClick(func() {
				// Choosing the scheme that shows already changes nothing, as
				// a toggle that is already active.
				if !selected {
					w.applyColorScheme(scheme)
				}
			})
		}
	})
}

// schemeTile is a radio tile with a small drawing of a window in the light
// or dark theme, and its label under it. The chosen tile is the selected
// row of the brand: the selected fill inside an accent hairline.
func schemeTile(c *ui.Context, dark bool, label, tooltip string, selected bool) ui.Element {
	t := c.Theme()
	preview := schemePreview(c, dark)
	border, face := t.Border, ui.Transparent
	if selected {
		border, face = t.Accent, frostui.ColorsOf(t).SelectedFill
	}
	tile := ui.ButtonBase(c).Role(ui.RoleRadio).Checked(selected).Label(label).Tooltip(tooltip).Cursor(ui.CursorPointer)
	tile.Children(func() {
		ui.Column(c).Gap(10).Padding(10).Radius(frostui.RadiusCard).Background(face).Border(1, border).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(196, 118).Draw(func(p *ui.Painter, r ui.Rect) {
				drawWindowSketch(p, r, preview)
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				icon, color := "radio", t.TextMuted
				if selected {
					icon, color = "object-select", t.Accent
				}
				frostui.Icon(c, icon).TextColor(color)
				frostui.Strong(c, label).SingleLine()
			})
		})
	})
	return tile
}

// schemePreview is the theme a scheme tile draws its window in: the kit's
// Frostyard theme of that darkness with the desktop's preferences but not
// its accent, as frostui.Adapt sets the window's own theme.
func schemePreview(c *ui.Context, dark bool) *ui.Theme {
	p := c.Preferences()
	p.Accent = ui.Color{}
	return frostui.FrostyardTheme(dark).WithPreferences(p)
}

// drawWindowSketch paints a tiny window in theme pt, as the brand draws
// one: the chrome's title bar and side bar, and a square card on the panel
// gradient over a hairline, with two lines of text and a primary button on
// the brand gradient.
func drawWindowSketch(p *ui.Painter, r ui.Rect, pt *ui.Theme) {
	const bar = 18
	k := frostui.ColorsOf(pt)
	radius := pt.Radius
	p.Fill(r, pt.Background, radius)
	p.FillRadii(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: bar}, [4]float32{radius, radius, 0, 0}, k.Chrome)
	for i := range 3 {
		p.Fill(ui.Rect{X: r.X + r.W - 14 - float32(i)*10, Y: r.Y + 6, W: 6, H: 6}, pt.TextMuted.Alpha(0.5), 3)
	}
	side := ui.Rect{X: r.X, Y: r.Y + bar, W: r.W * 0.28, H: r.H - bar}
	p.FillRadii(side, [4]float32{0, 0, 0, radius}, k.Chrome)
	for i := range 3 {
		p.Fill(ui.Rect{X: side.X + 8, Y: side.Y + 10 + float32(i)*12, W: side.W - 16, H: 5}, pt.TextMuted.Alpha(0.45), frostui.RadiusChip)
	}
	card := ui.Rect{X: side.X + side.W + 10, Y: r.Y + bar + 10, W: r.W - side.W - 20, H: r.H - bar - 20}
	p.Fill(card, k.Panel, frostui.RadiusCard)
	p.FillGradient(card, ui.LinearGradient{From: k.PanelFrom, To: k.PanelTo, Angle: 160, End: 0.62}, frostui.RadiusCard)
	p.Stroke(card, pt.Border, frostui.RadiusCard, 1)
	p.Fill(ui.Rect{X: card.X + 8, Y: card.Y + 10, W: card.W * 0.62, H: 6}, pt.Text.Alpha(0.75), frostui.RadiusChip)
	p.Fill(ui.Rect{X: card.X + 8, Y: card.Y + 22, W: card.W * 0.42, H: 5}, pt.TextMuted.Alpha(0.6), frostui.RadiusChip)
	button := ui.Rect{X: card.X + 8, Y: card.Y + card.H - 18, W: 40, H: 10}
	p.FillGradient(button, ui.LinearGradient{From: k.GradientFrom, To: k.GradientTo, Angle: 135}, frostui.RadiusChip)
	p.Stroke(r, pt.Border, radius, 1)
}

// appsStep lists the configured applications: one section per category
// with a switch that turns the whole category on or off, and one row with
// a switch per application while its category is on. It shows no icons and
// fetches nothing from the network.
func (w *FirstRun) appsStep(c *ui.Context) {
	if w.apps == nil {
		return
	}
	t := c.Theme()
	frostui.Muted(c, i18n.T(i18n.MsgSelectApps)).TextAlign(ui.Center)
	ui.Scroll(c).Grow(1).Basis(0).Children(func() {
		ui.Column(c).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Padding(0, 4, 0, 0).Children(func() {
			for _, category := range w.apps.Categories {
				key := category.Key
				title := i18n.T(category.Title)
				categoryOn := w.selection.CategoryOn(key)
				ui.Column(c.Key("category:"+key)).Radius(frostui.RadiusCard).Background(t.Background).Border(1, t.Border).
					AlignItems(ui.Stretch).Children(func() {
					ui.Row(c).Gap(frostui.SpaceM).Padding(12, 14).AlignItems(ui.Center).Children(func() {
						ui.Column(c).Grow(1).Gap(2).Children(func() {
							frostui.Strong(c, title)
							if category.Subtitle != "" {
								frostui.Muted(c, i18n.T(category.Subtitle))
							}
						})
						on := categoryOn
						if ui.Switch(c, &on).Label(title).Changed() {
							w.selection.SetCategory(key, on)
						}
					})
					if !categoryOn {
						return
					}
					for _, app := range category.Apps {
						id := app.ID
						ui.Column(c.Key("app:" + id)).AlignItems(ui.Stretch).Children(func() {
							frostui.RowDivider(c)
							ui.Row(c).Gap(frostui.SpaceM).Padding(10, 14, 10, 28).AlignItems(ui.Center).Children(func() {
								ui.Column(c).Grow(1).Gap(1).Children(func() {
									frostui.Body(c, app.Name).SingleLine()
									frostui.Caption(c, id).Font("monospace").SingleLine()
								})
								on := w.selection.AppOn(id)
								if ui.Switch(c, &on).Label(app.Name).Changed() {
									w.selection.SetApp(id, on)
								}
							})
						})
					}
				})
			}
		})
	})
}

// progressStep lists the planned actions, each with its state: waiting,
// a spinner while it runs, then a check or a warning.
func (w *FirstRun) progressStep(c *ui.Context) {
	if len(w.actions) == 0 {
		return
	}
	t := c.Theme()
	ended := 0
	for _, s := range w.states {
		if s == firstrun.ActionSucceeded || s == firstrun.ActionFailed {
			ended++
		}
	}
	ui.Progress(c, float64(ended)/float64(len(w.actions))).Label(i18n.T(i18n.MsgProgress))
	ui.Scroll(c).Grow(1).Basis(0).Children(func() {
		ui.Column(c).Radius(frostui.RadiusCard).Background(t.Background).Border(1, t.Border).AlignItems(ui.Stretch).
			Role(ui.RoleList).Label(i18n.T(i18n.MsgProgress)).Children(func() {
			for i, action := range w.actions {
				state := w.states[i]
				title := action.Title()
				ui.Column(c.Key(i)).AlignItems(ui.Stretch).Children(func() {
					if i > 0 {
						frostui.RowDivider(c)
					}
					row := ui.Row(c).Gap(frostui.SpaceM).Padding(11, 14).AlignItems(ui.Center).Role(ui.RoleListItem).Label(title)
					if state == firstrun.ActionPending {
						row.Opacity(0.6)
					}
					row.Children(func() {
						ui.Box(c).Size(18, 18).Center().Children(func() {
							switch state {
							case firstrun.ActionRunning:
								ui.Spinner(c).Size(16, 16).Label(title)
							case firstrun.ActionSucceeded:
								frostui.Icon(c, "object-select").TextColor(t.Success)
							case firstrun.ActionFailed:
								frostui.Icon(c, "dialog-warning").TextColor(t.Warning)
							default:
								frostui.Icon(c, "content-loading").TextColor(t.TextMuted)
							}
						})
						frostui.Body(c, title).Grow(1).SingleLine()
					})
				})
			}
		})
	})
}

// doneStep says the first run finished, with Exit and, when an action or
// the completion failed, the warning icon and the error log.
func (w *FirstRun) doneStep(c *ui.Context) {
	done := w.done
	icon := frostui.HeroIcon{Name: "object-select", Tone: frostui.Success}
	if done.Warning {
		icon = frostui.HeroIcon{Name: "dialog-warning", Tone: frostui.Warning}
	}
	ui.Column(c).Grow(1).Gap(24).Justify(ui.Center).AlignItems(ui.Stretch).Children(func() {
		frostui.Hero(c, icon, i18n.T(i18n.MsgFinished), i18n.T(done.Description))
		ui.Row(c).Justify(ui.Center).Children(func() {
			ui.PrimaryButton(c, i18n.T(i18n.MsgExit)).MinWidth(200).AutoFocus().OnClick(w.exit)
		})
		if len(done.Entries) == 0 {
			return
		}
		t := c.Theme()
		ui.Column(c).Padding(10, 14).Radius(frostui.RadiusCard).Background(t.Background).Border(1, t.Border).AlignItems(ui.Stretch).Children(func() {
			ui.Collapsible(c, i18n.T(i18n.MsgErrorLog), &w.errorLogOpen, func() {
				ui.Scroll(c).MaxHeight(180).Children(func() {
					ui.Column(c).Gap(frostui.SpaceS).Padding(frostui.SpaceS, 0, 0, 0).AlignItems(ui.Stretch).Children(func() {
						for i, entry := range done.Entries {
							ui.Column(c.Key(i)).Gap(2).Children(func() {
								frostui.Strong(c, entry.Title).Selectable()
								frostui.Mono(c, entry.Message).Selectable()
							})
						}
					})
				})
			})
		})
	})
}
