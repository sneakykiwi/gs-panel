package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the current version of GS Panel
	// This is set during build via ldflags
	Version = "0.0.1"

	// GitCommit is the git commit hash
	GitCommit = "unknown"

	// BuildTime is when the binary was built
	BuildTime = "unknown"

	// GoVersion is the Go version used to build
	GoVersion = runtime.Version()
)

// Info returns a formatted version string
func Info() string {
	return fmt.Sprintf("GS Panel v%s (commit: %s, built: %s, go: %s)",
		Version, GitCommit, BuildTime, GoVersion)
}

// Short returns just the version number
func Short() string {
	return Version
}
