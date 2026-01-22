// Package snap provides an interface to the Snap package manager via the snapd REST API
package snap

import (
	"context"
	"log"
	"time"

	"github.com/snapcore/snapd/client"
)

var (
	dryRun  = false
	timeout = 60 * time.Second
)

// SetDryRun sets the dry-run mode
func SetDryRun(mode bool) {
	dryRun = mode
	log.Printf("Snap dry-run mode: %v", mode)
}

// IsDryRun returns whether dry-run mode is enabled
func IsDryRun() bool {
	return dryRun
}

// Application represents an installed Snap application
type Application struct {
	Name        string
	ID          string
	Version     string
	Channel     string
	Confinement string
	Developer   string
	Status      string
}

// getClient creates a new snapd client with interactive polkit authentication
func getClient() *client.Client {
	return client.New(&client.Config{
		Interactive: true,
	})
}

// IsInstalled checks if Snap is installed and accessible by checking if snapd socket is available
func IsInstalled() bool {
	cli := getClient()
	_, err := cli.SysInfo()
	return err == nil
}

// ListInstalledSnaps returns all installed Snap applications
func ListInstalledSnaps() ([]Application, error) {
	cli := getClient()

	snaps, err := cli.List(nil, nil)
	if err != nil {
		// ErrNoSnapsInstalled is not actually an error for our purposes
		if err == client.ErrNoSnapsInstalled {
			return []Application{}, nil
		}
		return nil, err
	}

	apps := make([]Application, 0, len(snaps))
	for _, s := range snaps {
		developer := ""
		if s.Publisher != nil {
			developer = s.Publisher.Username
		}
		apps = append(apps, Application{
			Name:        s.Name,
			ID:          s.ID,
			Version:     s.Version,
			Channel:     s.TrackingChannel,
			Confinement: string(s.Confinement),
			Developer:   developer,
			Status:      s.Status,
		})
	}

	return apps, nil
}

// IsSnapInstalled checks if a specific snap is installed
func IsSnapInstalled(name string) (bool, error) {
	cli := getClient()

	_, _, err := cli.Snap(name)
	if err != nil {
		// Check if it's a "not found" error
		if e, ok := err.(*client.Error); ok && e.Kind == client.ErrorKindSnapNotFound {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// Install installs a snap by name
func Install(ctx context.Context, name string) (string, error) {
	if dryRun {
		log.Printf("[DRY-RUN] Would install snap: %s", name)
		return "", nil
	}

	cli := getClient()
	changeID, err := cli.Install(name, nil, nil)
	if err != nil {
		return "", err
	}

	return changeID, nil
}

// WaitForChange waits for a change to complete and returns the final status
func WaitForChange(ctx context.Context, changeID string) error {
	if changeID == "" {
		return nil
	}

	cli := getClient()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		change, err := cli.Change(changeID)
		if err != nil {
			return err
		}

		if change.Ready {
			if change.Err != "" {
				return &Error{Message: change.Err}
			}
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}
}

// Error represents a Snap-related error
type Error struct {
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

// DefaultContext returns a context with the default timeout
func DefaultContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}
