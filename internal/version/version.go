package version

import (
	"fmt"
	"runtime"
)

// These variables are set at build time via ldflags.
var (
	Version   = "dev"
	GitCommit = "none"
	BuildDate = "unknown"
)

func Full() string {
	return fmt.Sprintf("orbit %s (commit: %s, built: %s, %s/%s)",
		Version, GitCommit, BuildDate, runtime.GOOS, runtime.GOARCH)
}
