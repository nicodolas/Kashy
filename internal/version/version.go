// Package version is the single source of truth for Kashy's version number.
// To release: update Major/Minor/Patch here, run `make release`.
// Build tooling injects GitCommit and BuildDate via -ldflags.
package version

import "fmt"

const (
	Major = 1
	Minor = 2
	Patch = 0
)

// Injected at build time via -ldflags "-X github.com/nicodolas/kashy/internal/version.GitCommit=..."
var (
	GitCommit = "dev"
	BuildDate = "unknown"
)

// String returns the full semver string: v0.2.0
func String() string {
	return fmt.Sprintf("v%d.%d.%d", Major, Minor, Patch)
}

// Full returns version + git commit + build date: v0.2.0 (abc1234, 2026-07-01)
func Full() string {
	return fmt.Sprintf("%s (%s, %s)", String(), GitCommit, BuildDate)
}
