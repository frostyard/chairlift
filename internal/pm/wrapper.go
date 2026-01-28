// Package pm provides a wrapper around the frostyard/pm library
// with ChairLift-specific features like dry-run support and progress reporter
package pm

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/frostyard/chairlift/internal/async"
	pm "github.com/frostyard/pm"
)

var (
	dryRun   = false
	dryRunMu sync.RWMutex

	// Flatpak manager and availability
	flatpakManager   pm.Manager
	flatpakMu        sync.RWMutex
	flatpakAvailable *bool
	flatpakAvailMu   sync.RWMutex

	// Snap manager and availability
	snapManager   pm.Manager
	snapMu        sync.RWMutex
	snapAvailable *bool
	snapAvailMu   sync.RWMutex
	snapTimeout   = 120 * time.Second

	// Homebrew manager and availability
	brewManager   pm.Manager
	brewMu        sync.RWMutex
	brewAvailable *bool
	brewAvailMu   sync.RWMutex
	brewTimeout   = 120 * time.Second
)

// ProgressCallback is called during long-running operations with progress updates
type ProgressCallback func(action *pm.ProgressAction, task *pm.ProgressTask, step *pm.ProgressStep, message *pm.ProgressMessage)

// SetDryRun sets the global dry-run mode
func SetDryRun(mode bool) {
	dryRunMu.Lock()
	defer dryRunMu.Unlock()
	dryRun = mode
	log.Printf("pm wrapper dry-run mode: %v", mode)
}

// IsDryRun returns whether dry-run mode is enabled
func IsDryRun() bool {
	dryRunMu.RLock()
	defer dryRunMu.RUnlock()
	return dryRun
}

// InitializeFlatpak initializes the Flatpak manager with optional progress reporting
// Also checks availability synchronously to populate the cache before UI is built
func InitializeFlatpak(progressCallback ProgressCallback) error {
	flatpakMu.Lock()
	defer flatpakMu.Unlock()

	opts := []pm.ConstructorOption{}
	if progressCallback != nil {
		log.Printf("[PM] Initializing Flatpak with progress callback")
		opts = append(opts, pm.WithProgress(newProgressReporter(progressCallback)))
	} else {
		log.Printf("[PM] Initializing Flatpak without progress callback")
	}

	mgr := pm.NewFlatpak(opts...)
	flatpakManager = mgr

	// Check availability synchronously during init (before UI is built)
	// This happens early enough to not block UI rendering
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		available, err := mgr.Available(ctx)
		if err != nil {
			available = false
		}

		flatpakAvailMu.Lock()
		flatpakAvailable = &available
		flatpakAvailMu.Unlock()
	}()

	return nil
}

// GetFlatpakManager returns the initialized Flatpak manager
func GetFlatpakManager() pm.Manager {
	flatpakMu.RLock()
	defer flatpakMu.RUnlock()
	return flatpakManager
}

// FlatpakIsInstalled checks if Flatpak is available
// Returns the cached availability status (populated during InitializeFlatpak)
func FlatpakIsInstalled() bool {
	flatpakAvailMu.RLock()
	defer flatpakAvailMu.RUnlock()

	if flatpakAvailable != nil {
		return *flatpakAvailable
	}

	// If cache not populated yet, return false
	return false
}

// FlatpakApplication represents an installed Flatpak application
type FlatpakApplication struct {
	ID      string
	Name    string
	Version string
	IsUser  bool // true for user scope, false for system scope
	Kind    string
}

// ListFlatpakApplications returns all installed Flatpak applications (user and system)
func ListFlatpakApplications() ([]FlatpakApplication, error) {
	if IsDryRun() {
		return []FlatpakApplication{}, nil
	}

	mgr := GetFlatpakManager()
	if mgr == nil {
		return nil, fmt.Errorf("flatpak manager not initialized")
	}

	// Cast to Lister interface
	lister, ok := mgr.(pm.Lister)
	if !ok {
		return nil, fmt.Errorf("flatpak manager does not support list operation")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := pm.ListOptions{}
	installed, err := lister.ListInstalled(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list Flatpak applications: %w", err)
	}

	var apps []FlatpakApplication
	for _, pkg := range installed {
		// Namespace is "user" or "system" from pm library's flatpak backend.
		// The pm library parses `flatpak list --columns=installation` to populate this.
		isUser := pkg.Ref.Namespace == "user"
		apps = append(apps, FlatpakApplication{
			ID:      pkg.Ref.Name,
			Name:    pkg.Ref.Name, // Use Name from Ref since there's no separate Name field
			Version: pkg.Version,
			IsUser:  isUser,
			Kind:    string(pkg.Ref.Kind),
		})
	}

	return apps, nil
}

// ListFlatpakApplicationsByScope returns Flatpak applications filtered by scope (user or system)
func ListFlatpakApplicationsByScope(userScope bool) ([]FlatpakApplication, error) {
	if IsDryRun() {
		return []FlatpakApplication{}, nil
	}

	all, err := ListFlatpakApplications()
	if err != nil {
		return nil, err
	}

	var filtered []FlatpakApplication
	for _, app := range all {
		if app.IsUser == userScope {
			filtered = append(filtered, app)
		}
	}

	return filtered, nil
}

// FlatpakUpdateInfo represents available Flatpak updates
type FlatpakUpdateInfo struct {
	ID           string
	Name         string
	CurrentVer   string
	AvailableVer string
	IsUser       bool
}

// ListFlatpakUpdates returns available Flatpak updates
// For now, this is a placeholder that returns the installed apps as a workaround
func ListFlatpakUpdates() ([]FlatpakUpdateInfo, error) {
	if IsDryRun() {
		return []FlatpakUpdateInfo{}, nil
	}

	// TODO: Implement when pm library has update detection
	// For now, return empty list
	return []FlatpakUpdateInfo{}, nil
}

// FlatpakInstall installs a Flatpak application
func FlatpakInstall(appID string, userScope bool) error {
	if IsDryRun() {
		log.Printf("[DRY-RUN] Would install Flatpak: %s (user=%v)", appID, userScope)
		return nil
	}

	mgr := GetFlatpakManager()
	if mgr == nil {
		return fmt.Errorf("flatpak manager not initialized")
	}

	// Cast to Installer interface
	installer, ok := mgr.(pm.Installer)
	if !ok {
		return fmt.Errorf("flatpak manager does not support install operation")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scope := "system"
	if userScope {
		scope = "user"
	}

	ref := pm.PackageRef{
		Name:      appID,
		Namespace: scope,
		Kind:      "app",
	}

	opts := pm.InstallOptions{}
	_, err := installer.Install(ctx, []pm.PackageRef{ref}, opts)
	if err != nil {
		return async.NewUserErrorWithHint(
			fmt.Sprintf("Couldn't install %s", appID),
			"Check your internet connection or try again later",
			err,
		)
	}

	return nil
}

// FlatpakUninstall uninstalls a Flatpak application
func FlatpakUninstall(appID string, userScope bool) error {
	if IsDryRun() {
		log.Printf("[DRY-RUN] Would uninstall Flatpak: %s (user=%v)", appID, userScope)
		return nil
	}

	mgr := GetFlatpakManager()
	if mgr == nil {
		return fmt.Errorf("flatpak manager not initialized")
	}

	// Cast to Uninstaller interface
	uninstaller, ok := mgr.(pm.Uninstaller)
	if !ok {
		return fmt.Errorf("flatpak manager does not support uninstall operation")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scope := "system"
	if userScope {
		scope = "user"
	}

	ref := pm.PackageRef{
		Name:      appID,
		Namespace: scope,
		Kind:      "app",
	}

	opts := pm.UninstallOptions{}
	_, err := uninstaller.Uninstall(ctx, []pm.PackageRef{ref}, opts)
	if err != nil {
		return async.NewUserErrorWithHint(
			fmt.Sprintf("Couldn't remove %s", appID),
			"The app may be in use or protected",
			err,
		)
	}

	return nil
}

// FlatpakUpdate updates a Flatpak application
func FlatpakUpdate(appID string, userScope bool) error {
	if IsDryRun() {
		log.Printf("[DRY-RUN] Would update Flatpak: %s (user=%v)", appID, userScope)
		return nil
	}

	mgr := GetFlatpakManager()
	if mgr == nil {
		return fmt.Errorf("flatpak manager not initialized")
	}

	// Cast to Upgrader interface
	upgrader, ok := mgr.(pm.Upgrader)
	if !ok {
		return fmt.Errorf("flatpak manager does not support upgrade operation")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Note: Upgrader.Upgrade() upgrades all packages. For single package upgrade,
	// we'd need additional filtering which the current pm API doesn't support
	opts := pm.UpgradeOptions{}
	_, err := upgrader.Upgrade(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to update Flatpak %s: %w", appID, err)
	}

	return nil
}

// FlatpakUninstallUnused removes unused Flatpak runtimes
func FlatpakUninstallUnused() (string, error) {
	if IsDryRun() {
		return "[DRY-RUN] Would uninstall unused Flatpak runtimes", nil
	}

	// This operation may not be directly supported by pm library
	// For now, return a placeholder
	return "Unused Flatpak runtimes cleanup (not yet implemented)", nil
}

// progressReporter implements pm.ProgressReporter interface
type progressReporter struct {
	callback ProgressCallback
	mu       sync.Mutex
}

// newProgressReporter creates a new progress reporter
func newProgressReporter(callback ProgressCallback) pm.ProgressReporter {
	return &progressReporter{
		callback: callback,
	}
}

// OnAction is called at the start of a top-level operation
func (pr *progressReporter) OnAction(action pm.ProgressAction) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	fmt.Println("[ProgressReporter] OnAction called:", action)
	log.Printf("[ProgressReporter] OnAction called: %s (Started: %v, Ended: %v)", action.Name, action.StartedAt, action.EndedAt)
	if pr.callback != nil {
		// Marshal to main thread for UI update
		actionCopy := action
		async.RunOnMain(func() {
			pr.callback(&actionCopy, nil, nil, nil)
		})
	}
}

// OnTask is called when a major step begins
func (pr *progressReporter) OnTask(task pm.ProgressTask) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	fmt.Println("[ProgressReporter] OnTask called:", task)
	log.Printf("[ProgressReporter] OnTask called: %s (ActionID: %s, Started: %v, Ended: %v)", task.Name, task.ActionID, task.StartedAt, task.EndedAt)
	if pr.callback != nil {
		taskCopy := task
		async.RunOnMain(func() {
			pr.callback(nil, &taskCopy, nil, nil)
		})
	}
}

// OnStep is called for fine-grained progress updates
func (pr *progressReporter) OnStep(step pm.ProgressStep) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	fmt.Println("[ProgressReporter] OnStep called:", step)
	log.Printf("[ProgressReporter] OnStep called: %s (TaskID: %s, Started: %v, Ended: %v)", step.Name, step.TaskID, step.StartedAt, step.EndedAt)
	if pr.callback != nil {
		stepCopy := step
		async.RunOnMain(func() {
			pr.callback(nil, nil, &stepCopy, nil)
		})
	}
}

// OnMessage is called for informational, warning, or error messages
func (pr *progressReporter) OnMessage(msg pm.ProgressMessage) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	fmt.Println("[ProgressReporter] OnMessage called:", msg)
	log.Printf("[ProgressReporter] OnMessage called: %s (StepID: %s, TaskID: %s, ActionID: %s, Severity: %v)", msg.Text, msg.StepID, msg.TaskID, msg.ActionID, msg.Severity)
	if pr.callback != nil {
		msgCopy := msg
		async.RunOnMain(func() {
			pr.callback(nil, nil, nil, &msgCopy)
		})
	}
}

// ============================================================================
// SNAP
// ============================================================================

// SnapApplication represents an installed Snap application
type SnapApplication struct {
	Name        string
	ID          string
	Version     string
	Channel     string
	Confinement string
	Developer   string
	Status      string
}

// InitializeSnap initializes the Snap manager
func InitializeSnap() error {
	snapMu.Lock()
	defer snapMu.Unlock()

	mgr := pm.NewSnap()
	snapManager = mgr

	// Check availability asynchronously during init (before UI is built)
	// This happens early enough to not block UI rendering
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		available, err := mgr.Available(ctx)
		if err != nil {
			available = false
		}

		snapAvailMu.Lock()
		snapAvailable = &available
		snapAvailMu.Unlock()
	}()

	return nil
}

// SnapIsInstalled checks if Snap is installed and accessible
// Returns the cached availability status (populated during InitializeSnap)
func SnapIsInstalled() bool {
	snapAvailMu.RLock()
	defer snapAvailMu.RUnlock()

	if snapAvailable != nil {
		return *snapAvailable
	}

	// If cache not populated yet, return false
	return false
}

// ListInstalledSnaps returns all installed Snap applications
func ListInstalledSnaps() ([]SnapApplication, error) {
	if IsDryRun() {
		return []SnapApplication{}, nil
	}

	snapMu.RLock()
	mgr := snapManager
	snapMu.RUnlock()

	if mgr == nil {
		return nil, fmt.Errorf("snap manager not initialized")
	}

	lister, ok := mgr.(pm.Lister)
	if !ok {
		return nil, fmt.Errorf("snap manager does not support list operation")
	}

	ctx, cancel := context.WithTimeout(context.Background(), snapTimeout)
	defer cancel()

	opts := pm.ListOptions{}
	installed, err := lister.ListInstalled(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list Snap applications: %w", err)
	}

	apps := make([]SnapApplication, 0, len(installed))
	for _, pkg := range installed {
		apps = append(apps, SnapApplication{
			Name:    pkg.Ref.Name,
			ID:      pkg.Ref.Name,
			Version: pkg.Version,
			Channel: pkg.Ref.Channel,
			Status:  pkg.Status,
		})
	}

	return apps, nil
}

// IsSnapInstalled checks if a specific snap is installed
func IsSnapInstalled(name string) (bool, error) {
	snaps, err := ListInstalledSnaps()
	if err != nil {
		return false, err
	}

	for _, s := range snaps {
		if s.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// SnapInstall installs a snap by name
func SnapInstall(ctx context.Context, name string) (string, error) {
	if IsDryRun() {
		log.Printf("[DRY-RUN] Would install snap: %s", name)
		return "", nil
	}

	snapMu.RLock()
	mgr := snapManager
	snapMu.RUnlock()

	if mgr == nil {
		return "", fmt.Errorf("snap manager not initialized")
	}

	installer, ok := mgr.(pm.Installer)
	if !ok {
		return "", fmt.Errorf("snap manager does not support install operation")
	}

	ref := pm.PackageRef{
		Name: name,
		Kind: "snap",
	}

	opts := pm.InstallOptions{}
	result, err := installer.Install(ctx, []pm.PackageRef{ref}, opts)
	if err != nil {
		return "", async.NewUserErrorWithHint(
			fmt.Sprintf("Couldn't install %s", name),
			"The Snap store may be unavailable",
			err,
		)
	}

	if result.Changed && len(result.PackagesInstalled) > 0 {
		return "installed", nil
	}
	return "", nil
}

// SnapWaitForChange is a no-op for compatibility (pm operations are synchronous)
func SnapWaitForChange(ctx context.Context, changeID string) error {
	return nil
}

// SnapDefaultContext returns a context with the default timeout
func SnapDefaultContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), snapTimeout)
}

// ============================================================================
// HOMEBREW
// ============================================================================

// HomebrewPackage represents an installed Homebrew package
type HomebrewPackage struct {
	Name               string
	Version            string
	InstalledOnRequest bool
	Pinned             bool
	Outdated           bool
	Dependencies       []string
}

// HomebrewSearchResult represents a search result
type HomebrewSearchResult struct {
	Name        string
	Description string
	Homepage    string
}

// InitializeHomebrew initializes the Homebrew manager with optional progress reporting
func InitializeHomebrew(progressCallback ProgressCallback) error {
	brewMu.Lock()
	defer brewMu.Unlock()

	opts := []pm.ConstructorOption{}
	if progressCallback != nil {
		log.Printf("[PM] Initializing Homebrew with progress callback")
		opts = append(opts, pm.WithProgress(newProgressReporter(progressCallback)))
	} else {
		log.Printf("[PM] Initializing Homebrew without progress callback")
	}

	mgr := pm.NewBrew(opts...)
	brewManager = mgr

	// Check availability asynchronously during init (before UI is built)
	// This happens early enough to not block UI rendering
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		available, err := mgr.Available(ctx)
		if err != nil {
			available = false
		}

		brewAvailMu.Lock()
		brewAvailable = &available
		brewAvailMu.Unlock()
	}()

	return nil
}

// HomebrewIsInstalled checks if Homebrew is installed and accessible
// Returns the cached availability status (populated during InitializeHomebrew)
func HomebrewIsInstalled() bool {
	brewAvailMu.RLock()
	defer brewAvailMu.RUnlock()

	if brewAvailable != nil {
		return *brewAvailable
	}

	// If cache not populated yet, return false
	return false
}

// HomebrewIsDryRun returns whether dry-run mode is enabled
func HomebrewIsDryRun() bool {
	return IsDryRun()
}

// ListHomebrewFormulae returns all installed formulae
func ListHomebrewFormulae() ([]HomebrewPackage, error) {
	if IsDryRun() {
		return []HomebrewPackage{}, nil
	}

	brewMu.RLock()
	mgr := brewManager
	brewMu.RUnlock()

	if mgr == nil {
		return nil, fmt.Errorf("homebrew manager not initialized")
	}

	lister, ok := mgr.(pm.Lister)
	if !ok {
		return nil, fmt.Errorf("homebrew manager does not support list operation")
	}

	ctx, cancel := context.WithTimeout(context.Background(), brewTimeout)
	defer cancel()

	opts := pm.ListOptions{}
	installed, err := lister.ListInstalled(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list Homebrew formulae: %w", err)
	}

	var packages []HomebrewPackage
	for _, pkg := range installed {
		if pkg.Ref.Kind == "cask" {
			continue
		}
		packages = append(packages, HomebrewPackage{
			Name:    pkg.Ref.Name,
			Version: pkg.Version,
		})
	}

	return packages, nil
}

// ListHomebrewCasks returns all installed casks
func ListHomebrewCasks() ([]HomebrewPackage, error) {
	if IsDryRun() {
		return []HomebrewPackage{}, nil
	}

	brewMu.RLock()
	mgr := brewManager
	brewMu.RUnlock()

	if mgr == nil {
		return nil, fmt.Errorf("homebrew manager not initialized")
	}

	lister, ok := mgr.(pm.Lister)
	if !ok {
		return nil, fmt.Errorf("homebrew manager does not support list operation")
	}

	ctx, cancel := context.WithTimeout(context.Background(), brewTimeout)
	defer cancel()

	opts := pm.ListOptions{}
	installed, err := lister.ListInstalled(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list Homebrew casks: %w", err)
	}

	var packages []HomebrewPackage
	for _, pkg := range installed {
		if pkg.Ref.Kind != "cask" {
			continue
		}
		packages = append(packages, HomebrewPackage{
			Name:    pkg.Ref.Name,
			Version: pkg.Version,
		})
	}

	return packages, nil
}

// ListHomebrewOutdated returns all outdated packages
func ListHomebrewOutdated() ([]HomebrewPackage, error) {
	if IsDryRun() {
		return []HomebrewPackage{}, nil
	}

	// pm library doesn't have dedicated outdated interface yet
	// Fall back to executing brew command directly
	ctx, cancel := context.WithTimeout(context.Background(), brewTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "brew", "outdated", "--quiet")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	err := cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return nil, err
		}
	}

	var packages []HomebrewPackage
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			packages = append(packages, HomebrewPackage{
				Name:     line,
				Outdated: true,
			})
		}
	}

	return packages, nil
}

// HomebrewSearch searches for formulae matching the query
func HomebrewSearch(query string) ([]HomebrewSearchResult, error) {
	if IsDryRun() {
		return []HomebrewSearchResult{}, nil
	}

	brewMu.RLock()
	mgr := brewManager
	brewMu.RUnlock()

	if mgr == nil {
		return nil, fmt.Errorf("homebrew manager not initialized")
	}

	searcher, ok := mgr.(pm.Searcher)
	if !ok {
		return nil, fmt.Errorf("homebrew manager does not support search operation")
	}

	ctx, cancel := context.WithTimeout(context.Background(), brewTimeout)
	defer cancel()

	opts := pm.SearchOptions{}
	results, err := searcher.Search(ctx, query, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to search Homebrew: %w", err)
	}

	var searchResults []HomebrewSearchResult
	for _, pkg := range results {
		searchResults = append(searchResults, HomebrewSearchResult{
			Name: pkg.Name,
		})
	}

	return searchResults, nil
}

// HomebrewInstall installs a package
func HomebrewInstall(name string, isCask bool) error {
	log.Printf("HomebrewInstall called: name=%s, isCask=%v", name, isCask)

	if IsDryRun() {
		log.Printf("[DRY-RUN] Would install homebrew package: %s (cask=%v)", name, isCask)
		return nil
	}

	brewMu.RLock()
	mgr := brewManager
	brewMu.RUnlock()

	if mgr == nil {
		err := fmt.Errorf("homebrew manager not initialized")
		log.Printf("HomebrewInstall error: %v", err)
		return err
	}

	installer, ok := mgr.(pm.Installer)
	if !ok {
		err := fmt.Errorf("homebrew manager does not support install operation")
		log.Printf("HomebrewInstall error: %v", err)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), brewTimeout)
	defer cancel()

	kind := "formula"
	if isCask {
		kind = "cask"
	}

	ref := pm.PackageRef{
		Name: name,
		Kind: kind,
	}

	log.Printf("HomebrewInstall: Installing package ref: %+v", ref)
	opts := pm.InstallOptions{}
	_, err := installer.Install(ctx, []pm.PackageRef{ref}, opts)
	if err != nil {
		log.Printf("HomebrewInstall error: failed to install %s: %v", name, err)
		return async.NewUserErrorWithHint(
			fmt.Sprintf("Couldn't install %s", name),
			"Check your internet connection or run 'brew update'",
			err,
		)
	}

	log.Printf("HomebrewInstall: Successfully installed %s", name)
	return nil
}

// HomebrewUninstall removes a package
func HomebrewUninstall(name string, isCask bool) error {
	if IsDryRun() {
		log.Printf("[DRY-RUN] Would uninstall homebrew package: %s (cask=%v)", name, isCask)
		return nil
	}

	brewMu.RLock()
	mgr := brewManager
	brewMu.RUnlock()

	if mgr == nil {
		return fmt.Errorf("homebrew manager not initialized")
	}

	uninstaller, ok := mgr.(pm.Uninstaller)
	if !ok {
		return fmt.Errorf("homebrew manager does not support uninstall operation")
	}

	ctx, cancel := context.WithTimeout(context.Background(), brewTimeout)
	defer cancel()

	kind := "formula"
	if isCask {
		kind = "cask"
	}

	ref := pm.PackageRef{
		Name: name,
		Kind: kind,
	}

	opts := pm.UninstallOptions{}
	_, err := uninstaller.Uninstall(ctx, []pm.PackageRef{ref}, opts)
	if err != nil {
		return async.NewUserErrorWithHint(
			fmt.Sprintf("Couldn't remove %s", name),
			"The package may have dependents or be in use",
			err,
		)
	}

	return nil
}

// HomebrewUpgrade upgrades a package or all packages
func HomebrewUpgrade(name string) error {
	if IsDryRun() {
		if name != "" {
			log.Printf("[DRY-RUN] Would upgrade homebrew package: %s", name)
		} else {
			log.Printf("[DRY-RUN] Would upgrade all homebrew packages")
		}
		return nil
	}

	brewMu.RLock()
	mgr := brewManager
	brewMu.RUnlock()

	if mgr == nil {
		return fmt.Errorf("homebrew manager not initialized")
	}

	upgrader, ok := mgr.(pm.Upgrader)
	if !ok {
		return fmt.Errorf("homebrew manager does not support upgrade operation")
	}

	ctx, cancel := context.WithTimeout(context.Background(), brewTimeout)
	defer cancel()

	opts := pm.UpgradeOptions{}
	_, err := upgrader.Upgrade(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to upgrade: %w", err)
	}

	return nil
}

// HomebrewUpdate updates Homebrew itself
func HomebrewUpdate() error {
	if IsDryRun() {
		log.Printf("[DRY-RUN] Would update Homebrew")
		return nil
	}

	brewMu.RLock()
	mgr := brewManager
	brewMu.RUnlock()

	if mgr == nil {
		return fmt.Errorf("homebrew manager not initialized")
	}

	updater, ok := mgr.(pm.Updater)
	if !ok {
		return fmt.Errorf("homebrew manager does not support update operation")
	}

	ctx, cancel := context.WithTimeout(context.Background(), brewTimeout)
	defer cancel()

	opts := pm.UpdateOptions{}
	_, err := updater.Update(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to update Homebrew: %w", err)
	}

	return nil
}

// HomebrewBundleDump dumps installed packages to a Brewfile
func HomebrewBundleDump(path string, force bool) error {
	if IsDryRun() {
		log.Printf("[DRY-RUN] Would dump Brewfile to: %s", path)
		return nil
	}

	args := []string{"bundle", "dump"}
	if path != "" {
		args = append(args, "--file="+path)
	}
	if force {
		args = append(args, "--force")
	}

	return runBrewCommand(args...)
}

// HomebrewBundleInstall installs packages from a Brewfile
func HomebrewBundleInstall(path string) error {
	if IsDryRun() {
		log.Printf("[DRY-RUN] Would install from Brewfile: %s", path)
		return nil
	}

	args := []string{"bundle", "install"}
	if path != "" {
		args = append(args, "--file="+path)
	}

	return runBrewCommand(args...)
}

// HomebrewCleanup removes old versions, outdated downloads, and clears cache
func HomebrewCleanup() (string, error) {
	if IsDryRun() {
		msg := "[DRY-RUN] Would run brew cleanup"
		log.Println(msg)
		return msg, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), brewTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "brew", "cleanup")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("cleanup failed: %s", stderr.String())
	}

	return stdout.String(), nil
}

// runBrewCommand executes a brew command directly (for operations not yet in pm library)
func runBrewCommand(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), brewTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "brew", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("brew command failed: %s", stderr.String())
	}

	return nil
}
