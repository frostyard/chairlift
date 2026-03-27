package maintenance

import (
	"context"
	"fmt"
	"log"

	"github.com/frostyard/chairlift/internal/async"
	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/operations"
	"github.com/frostyard/chairlift/internal/pages"
	"github.com/frostyard/chairlift/internal/pm"
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// Page implements the Maintenance page.
type Page struct {
	toolbarView *adw.ToolbarView
	prefsPage   *adw.PreferencesPage

	config   *config.Config
	toaster  pages.Toaster
	executor ScriptExecutor

	// Goroutine lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new Maintenance page with the given dependencies.
func New(deps pages.Deps) *Page {
	ctx, cancel := context.WithCancel(context.Background())

	p := &Page{
		config:   deps.Config,
		toaster:  deps.Toaster,
		executor: &DefaultExecutor{},
		ctx:      ctx,
		cancel:   cancel,
	}

	p.buildUI()
	return p
}

// Widget returns the root widget for embedding.
func (p *Page) Widget() *adw.ToolbarView {
	return p.toolbarView
}

// Destroy cleans up resources and cancels running goroutines.
func (p *Page) Destroy() {
	if p.cancel != nil {
		p.cancel()
	}
}

func (p *Page) buildUI() {
	p.toolbarView = adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	p.toolbarView.AddTopBar(&headerBar.Widget)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	scrolled.SetVexpand(true)

	p.prefsPage = adw.NewPreferencesPage()
	scrolled.SetChild(&p.prefsPage.Widget)
	p.toolbarView.SetContent(&scrolled.Widget)

	// Build groups based on config
	p.buildCleanupGroup()
	p.buildHomebrewCleanupGroup()
	p.buildFlatpakCleanupGroup()
	p.buildOptimizationGroup()
}

func (p *Page) buildCleanupGroup() {
	if !p.config.IsGroupEnabled("maintenance_page", "maintenance_cleanup_group") {
		return
	}

	group := adw.NewPreferencesGroup()
	group.SetTitle("System Cleanup")
	group.SetDescription("Clean up system files and free disk space")

	groupCfg := p.config.GetGroupConfig("maintenance_page", "maintenance_cleanup_group")
	actions := ParseActions(groupCfg)

	for _, action := range actions {
		action := action // Capture in local variable for closure

		row := adw.NewActionRow()
		row.SetTitle(action.Title)
		row.SetSubtitle(action.Script)

		if action.Sudo {
			sudoIcon := gtk.NewImageFromIconName("dialog-password-symbolic")
			row.AddPrefix(&sudoIcon.Widget)
		}

		button := gtk.NewButtonWithLabel("Run")
		button.SetValign(gtk.AlignCenterValue)
		button.AddCssClass("suggested-action")

		clickedCb := func(btn gtk.Button) {
			p.onActionClicked(button, &action)
		}
		button.ConnectClicked(&clickedCb)

		row.AddSuffix(&button.Widget)
		group.Add(&row.Widget)
	}

	p.prefsPage.Add(group)
}

func (p *Page) onActionClicked(button *gtk.Button, action *Action) {
	button.SetSensitive(false)
	button.SetLabel("Running...")

	op := operations.Start(action.Title, operations.CategoryMaintenance, false)
	op.RetryFunc = func() {
		p.onActionClicked(button, action)
	}

	go func() {
		err := p.executor.Execute(p.ctx, action.Script, action.Sudo)

		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return // Page destroyed
			default:
			}

			button.SetSensitive(true)
			button.SetLabel("Run")
			op.Complete(err)

			if err != nil {
				p.toaster.ShowErrorToast(async.NewUserError(
					fmt.Sprintf("Couldn't run %s", action.Title), err).FormatForUser())
				log.Printf("Maintenance action error details: %v", err)
			} else {
				p.toaster.ShowToast(fmt.Sprintf("%s completed", action.Title))
			}
		})
	}()
}

func (p *Page) buildHomebrewCleanupGroup() {
	if !p.config.IsGroupEnabled("maintenance_page", "maintenance_brew_group") {
		return
	}
	if !pm.HomebrewIsInstalled() {
		return
	}

	group := adw.NewPreferencesGroup()
	group.SetTitle("Homebrew Cleanup")
	group.SetDescription("Remove old versions and clear Homebrew cache")

	row := adw.NewActionRow()
	row.SetTitle("Clean Up Homebrew")
	row.SetSubtitle("Remove outdated downloads and old package versions")

	icon := gtk.NewImageFromIconName("user-trash-symbolic")
	row.AddPrefix(&icon.Widget)

	button := gtk.NewButtonWithLabel("Clean Up")
	button.SetValign(gtk.AlignCenterValue)
	button.AddCssClass("suggested-action")

	clickedCb := func(btn gtk.Button) {
		p.onBrewCleanupClicked(button)
	}
	button.ConnectClicked(&clickedCb)

	row.AddSuffix(&button.Widget)
	group.Add(&row.Widget)

	p.prefsPage.Add(group)
}

func (p *Page) onBrewCleanupClicked(button *gtk.Button) {
	button.SetSensitive(false)
	button.SetLabel("Cleaning...")

	op := operations.Start("Homebrew Cleanup", operations.CategoryMaintenance, false)
	op.RetryFunc = func() {
		p.onBrewCleanupClicked(button)
	}

	go func() {
		output, err := pm.HomebrewCleanup()

		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return // Page destroyed
			default:
			}

			button.SetSensitive(true)
			button.SetLabel("Clean Up")
			op.Complete(err)

			if err != nil {
				p.toaster.ShowErrorToast(async.NewUserError(
					"Couldn't clean up Homebrew", err).FormatForUser())
				log.Printf("Homebrew cleanup error details: %v", err)
				return
			}

			if pm.HomebrewIsDryRun() {
				p.toaster.ShowToast(output)
			} else {
				p.toaster.ShowToast("Homebrew cleanup completed")
			}
		})
	}()
}

func (p *Page) buildFlatpakCleanupGroup() {
	if !p.config.IsGroupEnabled("maintenance_page", "maintenance_flatpak_group") {
		return
	}
	if !pm.FlatpakIsInstalled() {
		return
	}

	group := adw.NewPreferencesGroup()
	group.SetTitle("Flatpak Cleanup")
	group.SetDescription("Remove unused Flatpak runtimes and extensions")

	row := adw.NewActionRow()
	row.SetTitle("Remove Unused Runtimes")
	row.SetSubtitle("Uninstall unused Flatpak runtimes and extensions")

	icon := gtk.NewImageFromIconName("user-trash-symbolic")
	row.AddPrefix(&icon.Widget)

	button := gtk.NewButtonWithLabel("Clean Up")
	button.SetValign(gtk.AlignCenterValue)
	button.AddCssClass("suggested-action")

	clickedCb := func(btn gtk.Button) {
		p.onFlatpakCleanupClicked(button)
	}
	button.ConnectClicked(&clickedCb)

	row.AddSuffix(&button.Widget)
	group.Add(&row.Widget)

	p.prefsPage.Add(group)
}

func (p *Page) onFlatpakCleanupClicked(button *gtk.Button) {
	button.SetSensitive(false)
	button.SetLabel("Cleaning...")

	op := operations.Start("Flatpak Cleanup", operations.CategoryMaintenance, false)
	op.RetryFunc = func() {
		p.onFlatpakCleanupClicked(button)
	}

	go func() {
		output, err := pm.FlatpakUninstallUnused()

		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return // Page destroyed
			default:
			}

			button.SetSensitive(true)
			button.SetLabel("Clean Up")
			op.Complete(err)

			if err != nil {
				p.toaster.ShowErrorToast(async.NewUserError(
					"Couldn't remove unused Flatpak runtimes", err).FormatForUser())
				log.Printf("Flatpak cleanup error details: %v", err)
				return
			}

			if pm.IsDryRun() {
				p.toaster.ShowToast(output)
			} else {
				p.toaster.ShowToast("Flatpak cleanup completed")
			}
		})
	}()
}

func (p *Page) buildOptimizationGroup() {
	if !p.config.IsGroupEnabled("maintenance_page", "maintenance_optimization_group") {
		return
	}

	group := adw.NewPreferencesGroup()
	group.SetTitle("System Optimization")
	group.SetDescription("Optimize system performance")

	// Placeholder for optimization features
	row := adw.NewActionRow()
	row.SetTitle("Optimization tools")
	row.SetSubtitle("Coming soon")
	group.Add(&row.Widget)

	p.prefsPage.Add(group)
}
