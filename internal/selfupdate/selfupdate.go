// Package selfupdate — local binary-drift detection for the sirsi family.
//
// This is the detection half of ADR-023 (the deferred half — verified atomic
// self-update — lives behind `sirsi self-update` in a later phase). Everything
// here is LOCAL and read-only: it discovers sibling sirsi binaries on the host
// and asks each one its version via `<binary> version --json`. No network.
//
// Three drift modes (see ADR-023):
//   - D1 self-vs-upstream: running binary older than the latest release (network; deferred).
//   - D2 sibling drift:    `sirsi` fresh but `sirsi-menubar` stale — they share internal/router.
//   - D3 path drift:       the binary on $PATH != the running binary.
//
// This package is deliberately self-contained (no internal/router import) so
// the health surface in internal/router can consume it without an import cycle.
package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

// Method classifies how a binary was installed (governs how it may be updated).
type Method int

const (
	MethodUnknown Method = iota
	MethodRaw
	MethodHomebrew
	MethodGoRun
)

func (m Method) String() string {
	switch m {
	case MethodRaw:
		return "raw"
	case MethodHomebrew:
		return "homebrew"
	case MethodGoRun:
		return "go-run"
	default:
		return "unknown"
	}
}

// family is the set of sirsi binaries tracked for sibling drift. sirsi and
// sirsi-menubar are the pair that caused the CTR deploy-drift incident: both
// compile internal/router, so a stale menubar runs an old reaper silently.
var family = []string{"sirsi", "sirsi-menubar"}

// probeTimeout bounds each `version --json` subprocess so a hung binary can
// never stall a health tick.
const probeTimeout = 200 * time.Millisecond

// Sibling is a discovered sirsi-family binary on the host.
type Sibling struct {
	version.Info
	Method Method `json:"method"`
	Err    string `json:"error,omitempty"`
}

// DriftReport summarizes binary drift across the host. LOCAL, no network.
type DriftReport struct {
	Self       version.Info `json:"self"`
	Siblings   []Sibling    `json:"siblings"`
	D2Mismatch []Sibling    `json:"d2_mismatch,omitempty"`
	D3PathBin  string       `json:"d3_path_bin,omitempty"`
	Healthy    bool         `json:"healthy"`
}

// Summary renders a one-line, human-readable drift status — the token the
// SessionStart/menubar health surface displays.
func (r *DriftReport) Summary() string {
	if r == nil {
		return "unknown"
	}
	if r.Healthy {
		return fmt.Sprintf("%s in sync", r.Self.Version)
	}
	var parts []string
	for _, s := range r.D2Mismatch {
		parts = append(parts, fmt.Sprintf("%s %s stale (self %s)", s.Binary, s.Version, r.Self.Version))
	}
	if r.D3PathBin != "" {
		parts = append(parts, "PATH binary differs: "+r.D3PathBin)
	}
	if len(parts) == 0 {
		return "drift detected"
	}
	return strings.Join(parts, "; ")
}

// DetectMethod classifies how a sirsi-family binary at execPath was installed.
// Pure path inspection — no process exec.
func DetectMethod(execPath string) Method {
	if execPath == "" {
		return MethodUnknown
	}
	clean := filepath.Clean(execPath)
	tmp := filepath.Clean(os.TempDir())
	if (tmp != "" && strings.HasPrefix(clean, tmp)) || strings.Contains(clean, "/go-build") {
		return MethodGoRun
	}
	for _, p := range homebrewPrefixes() {
		if p == "" {
			continue
		}
		if strings.HasPrefix(clean, filepath.Clean(p)+string(os.PathSeparator)) {
			return MethodHomebrew
		}
	}
	return MethodRaw
}

func homebrewPrefixes() []string {
	prefixes := []string{"/opt/homebrew", "/usr/local"}
	if hp := os.Getenv("HOMEBREW_PREFIX"); hp != "" {
		prefixes = append(prefixes, hp)
	}
	return prefixes
}

// probeFn queries a binary's version contract. Injectable for tests.
var probeFn = probeBinary

func probeBinary(path string) (version.Info, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "version", "--json").Output()
	if err != nil {
		return version.Info{}, err
	}
	var info version.Info
	if err := json.Unmarshal(out, &info); err != nil {
		return version.Info{}, err
	}
	return info, nil
}

// ScanHost discovers sibling sirsi binaries and queries each one's version.
// LOCAL, no network. Safe to call on every health tick.
func ScanHost() (*DriftReport, error) {
	self := version.Current("sirsi")
	siblings := discover(self.Path)

	pathBin := ""
	if p, err := exec.LookPath("sirsi"); err == nil {
		pathBin = p
	}
	return BuildReport(self, siblings, pathBin), nil
}

// discover walks candidate install dirs, probing each sirsi-family binary it
// finds (skipping the running executable itself).
func discover(selfPath string) []Sibling {
	var siblings []Sibling
	seen := map[string]bool{}
	selfClean := filepath.Clean(selfPath)
	for _, dir := range candidateDirs(selfPath) {
		for _, name := range family {
			clean := filepath.Clean(filepath.Join(dir, name))
			if seen[clean] {
				continue
			}
			fi, err := os.Stat(clean)
			if err != nil || fi.IsDir() {
				continue
			}
			seen[clean] = true
			if clean == selfClean {
				continue // don't re-probe ourselves
			}
			sib := Sibling{Method: DetectMethod(clean)}
			info, perr := probeFn(clean)
			info.Path = clean
			if info.Binary == "" {
				info.Binary = name
			}
			sib.Info = info
			if perr != nil {
				sib.Err = perr.Error()
			}
			siblings = append(siblings, sib)
		}
	}
	return siblings
}

func candidateDirs(selfPath string) []string {
	var dirs []string
	add := func(d string) {
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	if selfPath != "" {
		add(filepath.Dir(selfPath))
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, ".local", "bin"))
	}
	add("/opt/homebrew/bin")
	add("/usr/local/bin")
	if hp := os.Getenv("HOMEBREW_PREFIX"); hp != "" {
		add(filepath.Join(hp, "bin"))
	}
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		add(p)
	}
	return dirs
}

// BuildReport applies the drift rules to a discovered set. Pure and
// deterministic — the unit the tests exercise.
func BuildReport(self version.Info, siblings []Sibling, pathBin string) *DriftReport {
	r := &DriftReport{Self: self, Siblings: siblings}
	for _, s := range siblings {
		// Only compare when the sibling reported a real, stamped version.
		if s.Err != "" || s.Version == "" || s.Version == "dev" {
			continue
		}
		if s.Version != self.Version {
			r.D2Mismatch = append(r.D2Mismatch, s)
		}
	}
	if pathBin != "" && self.Path != "" {
		pathClean := filepath.Clean(pathBin)
		selfClean := filepath.Clean(self.Path)
		if pathClean != selfClean {
			// Different install locations are not drift when the PATH binary is a
			// discovered, healthy copy of this exact stamped release. Pantheon.app
			// intentionally exposes its CLI through ~/.local/bin; path inequality
			// alone previously produced a permanent false warning after install.
			pathMatchesRelease := false
			for _, s := range siblings {
				if filepath.Clean(s.Path) == pathClean && s.Err == "" && s.Version == self.Version && s.Commit == self.Commit {
					pathMatchesRelease = true
					break
				}
			}
			if !pathMatchesRelease {
				r.D3PathBin = pathClean
			}
		}
	}
	r.Healthy = len(r.D2Mismatch) == 0 && r.D3PathBin == ""
	return r
}
