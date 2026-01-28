package applications

import (
	"context"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/pages"
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// Page implements the Applications page with sidebar navigation.
type Page struct {
	toolbarView *adw.ToolbarView
	splitView   *adw.NavigationSplitView
	sidebarList *gtk.ListBox
	contentStack *gtk.Stack

	currentCategory PMCategory

	config    *config.Config
	toaster   pages.Toaster
	launchApp func(string)
	openURL   func(string)

	// Goroutine lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new Applications page with the given dependencies.
func New(deps pages.Deps, launchApp, openURL func(string)) *Page {
	ctx, cancel := context.WithCancel(context.Background())

	p := &Page{
		config:    deps.Config,
		toaster:   deps.Toaster,
		launchApp: launchApp,
		openURL:   openURL,
		ctx:       ctx,
		cancel:    cancel,
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

	// Create NavigationSplitView for sidebar layout
	p.splitView = adw.NewNavigationSplitView()
	p.splitView.SetSidebarWidthFraction(0.25)
	p.splitView.SetMinSidebarWidth(180)
	p.splitView.SetMaxSidebarWidth(280)

	// Build sidebar
	sidebarPage := p.buildSidebar()
	p.splitView.SetSidebar(sidebarPage)

	// Build content area
	contentPage := p.buildContent()
	p.splitView.SetContent(contentPage)

	p.toolbarView.SetContent(&p.splitView.Widget)
}

func (p *Page) buildSidebar() *adw.NavigationPage {
	toolbarView := adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)
	toolbarView.AddTopBar(&headerBar.Widget)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	scrolled.SetVexpand(true)

	p.sidebarList = gtk.NewListBox()
	p.sidebarList.SetSelectionMode(gtk.SelectionSingleValue)
	p.sidebarList.AddCssClass("navigation-sidebar")

	// Add sidebar items
	items := GetSidebarItems()
	for _, item := range items {
		p.addSidebarRow(item)
	}

	// Connect row-activated signal
	rowActivatedCb := func(listbox gtk.ListBox, rowPtr uintptr) {
		row := gtk.ListBoxRowNewFromInternalPtr(rowPtr)
		p.onSidebarRowActivated(*row)
	}
	p.sidebarList.ConnectRowActivated(&rowActivatedCb)

	// Select first row by default
	if first := p.sidebarList.GetRowAtIndex(0); first != nil {
		p.sidebarList.SelectRow(first)
	}

	scrolled.SetChild(&p.sidebarList.Widget)
	toolbarView.SetContent(&scrolled.Widget)

	return adw.NewNavigationPage(&toolbarView.Widget, "Categories")
}

func (p *Page) addSidebarRow(item SidebarItem) {
	row := gtk.NewListBoxRow()

	box := gtk.NewBox(gtk.OrientationHorizontalValue, 12)
	box.SetMarginTop(8)
	box.SetMarginBottom(8)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	icon := gtk.NewImageFromIconName(item.IconName)
	box.Append(&icon.Widget)

	label := gtk.NewLabel(item.Label)
	label.SetHexpand(true)
	label.SetXalign(0)
	box.Append(&label.Widget)

	// Show "Not installed" indicator if PM unavailable
	if !item.IsInstalled && item.Category != CategoryAll {
		dimLabel := gtk.NewLabel("Not installed")
		dimLabel.AddCssClass("dim-label")
		box.Append(&dimLabel.Widget)
		row.SetSensitive(false)
	}

	row.SetChild(&box.Widget)
	// Store category in row name for lookup
	row.SetName(string(item.Category))

	p.sidebarList.Append(&row.Widget)
}

func (p *Page) buildContent() *adw.NavigationPage {
	toolbarView := adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	toolbarView.AddTopBar(&headerBar.Widget)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	scrolled.SetVexpand(true)

	// Stack to hold different category views
	p.contentStack = gtk.NewStack()
	p.contentStack.SetTransitionType(gtk.StackTransitionTypeCrossfadeValue)

	// Add placeholder page for each category
	// (Plan 03 will replace these with real content)
	for _, cat := range []PMCategory{CategoryAll, CategoryFlatpak, CategoryHomebrew, CategorySnap} {
		page := adw.NewPreferencesPage()
		placeholder := adw.NewPreferencesGroup()
		placeholder.SetTitle(string(cat) + " content")
		placeholder.SetDescription("Loading...")
		page.Add(placeholder)
		p.contentStack.AddNamed(&page.Widget, string(cat))
	}

	// Show "all" by default
	p.contentStack.SetVisibleChildName(string(CategoryAll))
	p.currentCategory = CategoryAll

	scrolled.SetChild(&p.contentStack.Widget)
	toolbarView.SetContent(&scrolled.Widget)

	return adw.NewNavigationPage(&toolbarView.Widget, "Applications")
}

func (p *Page) onSidebarRowActivated(row gtk.ListBoxRow) {
	category := PMCategory(row.GetName())
	if category == p.currentCategory {
		return
	}
	p.currentCategory = category
	p.contentStack.SetVisibleChildName(string(category))
}
