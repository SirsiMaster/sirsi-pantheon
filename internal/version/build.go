// Package version — build.go
//
// The binary build-version contract. Every sirsi-family binary reports its
// identity through the SAME stamped variables, so there is one source of
// truth instead of seven hand-edited `var version` literals.
//
// Stamped at build time via:
//
//	-ldflags "-X github.com/SirsiMaster/sirsi-pantheon/internal/version.Version=v0.22.0 \
//	          -X github.com/SirsiMaster/sirsi-pantheon/internal/version.Commit=<sha> \
//	          -X github.com/SirsiMaster/sirsi-pantheon/internal/version.Date=<iso8601>"
//
// When built with a plain `go build` (no ldflags), Current() falls back to
// debug.ReadBuildInfo() so the binary still self-reports honestly (Rule A23).
package version

import (
	"os"
	"runtime"
	"runtime/debug"
)

// Build identity. Overridden at link time; defaults are honest placeholders.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Info is the uniform version contract reported by every sirsi-family binary
// (via `<binary> version --json`). It is the unit of comparison for binary
// drift detection in internal/selfupdate.
type Info struct {
	Binary          string `json:"binary"`
	Version         string `json:"version"`
	Commit          string `json:"commit"`
	Date            string `json:"date"`
	Path            string `json:"path"`
	GoVer           string `json:"go"`
	Dirty           bool   `json:"dirty"`
	RouterSchemaMax int    `json:"router_schema_max"`
}

// Current returns the running binary's build identity. binary is the logical
// name (e.g. "sirsi", "sirsi-menubar"). Path is the resolved executable path.
// Commit and Dirty fall back to VCS build info when ldflags were not supplied,
// so a plain `go build` never lies about what it is.
func Current(binary string) Info {
	info := Info{
		Binary:  binary,
		Version: Version,
		Commit:  Commit,
		Date:    Date,
		GoVer:   runtime.Version(),
	}
	if exe, err := os.Executable(); err == nil {
		info.Path = exe
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if info.Commit == "none" && s.Value != "" {
					info.Commit = s.Value
				}
			case "vcs.modified":
				if s.Value == "true" {
					info.Dirty = true
				}
			}
		}
	}
	return info
}
