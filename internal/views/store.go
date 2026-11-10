package views

import (
	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/navigation"
)

// Store is ChairLift's state: one part per domain, each owned by its file,
// and the pages that draw them. It belongs to the UI thread.
type Store struct {
	Env *Env

	System   *System      // system.go: deployments, the image source, host facts
	Live     *Live        // live.go: processor, memory, disk, sampled each second
	Features *Features    // features.go: updex features and their source
	Flatpak  *Flatpak     // flatpak.go: apps and the Flatpak source
	Brew     *Brew        // brew.go: packages, taps, bundles and the Homebrew source
	Firmware *Firmware    // firmware.go: fwupd devices and the firmware source
	Problems *Problems    // problems.go: problem records and reporting
	Maint    *Maintenance // maintenance.go: cleanups

	pages map[string]Page
}

// NewStore builds every domain and page. Nothing loads until Start.
func NewStore(env *Env) *Store {
	s := &Store{Env: env}
	s.System = NewSystem(env)
	s.Live = NewLive(env)
	s.Features = NewFeatures(env)
	s.Flatpak = NewFlatpak(env)
	s.Brew = NewBrew(env)
	s.Firmware = NewFirmware(env)
	s.Problems = NewProblems(env)
	s.Maint = NewMaintenance(env, s)
	s.pages = map[string]Page{
		navigation.Overview:    NewOverviewPage(s),
		navigation.Updates:     NewUpdatesPage(s),
		navigation.Apps:        NewAppsPage(s),
		navigation.Features:    NewFeaturesPage(s),
		navigation.Problems:    NewProblemsPage(s),
		navigation.Maintenance: NewMaintenancePage(s),
	}
	return s
}

// Start begins every domain's first load. Each domain loads only what its
// enabled groups need.
func (s *Store) Start() {
	s.System.Start()
	s.Live.Start()
	s.Features.Start()
	s.Flatpak.Start()
	s.Brew.Start()
	s.Firmware.Start()
	s.Problems.Start()
	s.Maint.Start()
}

// Page returns the page with a navigation ID, nil for an unknown one.
func (s *Store) Page(id string) Page { return s.pages[id] }

// Sources are the update sources that apply here, in run order: the
// system image, features, Flatpak, Homebrew, firmware.
func (s *Store) Sources() []UpdateSource {
	all := []UpdateSource{s.System.Source(), s.Features.Source(), s.Flatpak.Source(), s.Brew.Source(), s.Firmware.Source()}
	out := all[:0]
	for _, src := range all {
		if src != nil && src.Status().Available {
			out = append(out, src)
		}
	}
	return out
}

// UpdateCount is the number of items all sources can update, held ones
// included, and how many sources have any: the Updates badge and figure.
func (s *Store) UpdateCount() (items, sources int) {
	for _, src := range s.Sources() {
		if n := len(src.Items()); n > 0 {
			items += n
			sources++
		}
	}
	return items, sources
}

// EverythingJobs returns the jobs "Update everything" runs: each source's
// default set, in source order. Firmware gives none: it asks first, device
// by device.
func (s *Store) EverythingJobs() []*frostui.Job {
	var jobs []*frostui.Job
	for _, src := range s.Sources() {
		jobs = append(jobs, src.Jobs()...)
	}
	return jobs
}

// UpdateEverything queues EverythingJobs and opens the console. It does
// nothing while jobs run.
func (s *Store) UpdateEverything() {
	if s.Env.Jobs.Busy() {
		s.Env.Toast("Wait for the running job to finish")
		return
	}
	jobs := s.EverythingJobs()
	if len(jobs) == 0 {
		s.Env.Toast("Nothing to update")
		return
	}
	s.Env.Jobs.Enqueue(jobs...)
}

// CheckAll starts a check of every source.
func (s *Store) CheckAll() {
	for _, src := range s.Sources() {
		src.Refresh()
	}
}

// Checking reports whether any source checks now.
func (s *Store) Checking() bool {
	for _, src := range s.Sources() {
		if src.Status().Checking {
			return true
		}
	}
	return false
}
