package cmdutil

import "runtime/debug"

// BuildInfo holds version-control metadata embedded by the Go toolchain at
// build time. The zero value represents an unknown build — all string fields
// are empty and Dirty is false.
type BuildInfo struct {
	// VCS is the version control system name (e.g., "git").
	VCS string

	// Revision is the full commit identifier at which the binary was built.
	Revision string

	// Time is the commit timestamp in RFC 3339 format.
	Time string

	// Dirty is true when the working tree had uncommitted changes at build time.
	Dirty bool
}

// ReadBuildInfo extracts VCS metadata from the running binary's embedded build
// information. Returns a zero-value BuildInfo when the information is
// unavailable (e.g., when running via `go run` or in tests).
func ReadBuildInfo() BuildInfo {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return BuildInfo{}
	}

	var info BuildInfo
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs":
			info.VCS = s.Value
		case "vcs.revision":
			info.Revision = s.Value
		case "vcs.time":
			info.Time = s.Value
		case "vcs.modified":
			info.Dirty = s.Value == "true"
		}
	}
	return info
}
