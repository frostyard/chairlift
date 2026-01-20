// Package version provides build version information
package version

// Build information set via ldflags
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
	BuiltBy = "unknown"
)

// Info returns formatted version information
func Info() string {
	return Version
}

// Full returns detailed version information
func Full() string {
	return Version + " (" + Commit + ")"
}
