// Package version carries the ggterm release identity.
//
// Release builds set Version, Commit and Date through -ldflags. Development
// builds keep the defaults below.
package version

import "fmt"

var (
	// Version is the semantic version of this build.
	Version = "1.0.0"
	// Commit is the VCS revision injected by the release build.
	Commit = "dev"
	// Date is the build date injected by the release build.
	Date = "unknown"
)

// String returns a human-readable version line.
func String() string {
	if Commit == "" || Commit == "dev" {
		return Version
	}
	if Date == "" || Date == "unknown" {
		return fmt.Sprintf("%s (%s)", Version, Commit)
	}
	return fmt.Sprintf("%s (%s, %s)", Version, Commit, Date)
}
