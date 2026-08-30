// Package buildinfo exposes metadata injected into Ship binaries at build time.
package buildinfo

import "fmt"

var (
	// SHA is overridden in release builds with -ldflags -X.
	SHA = "development"
	// Version is overridden for tagged releases.
	Version = "dev"
)

func Summary(component string) string {
	return fmt.Sprintf("%s %s (%s)", component, Version, SHA)
}
