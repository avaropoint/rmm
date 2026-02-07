// Package version provides build-time version information
// injected via ldflags during compilation.
package version

// These variables are set at build time via -ldflags.
var (
	Version   = "dev"
	BuildTime = "unknown"
)
