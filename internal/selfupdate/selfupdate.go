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
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
const maxProbeJSONBytes = 64 << 10

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

// probeContextFn preserves the bounded local version probe while allowing a
// caller to cancel the complete inventory operation.
var probeContextFn = probeBinaryContext

func probeBinary(path string) (version.Info, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	return probeBinaryContext(ctx, path)
}

func probeBinaryContext(parent context.Context, path string) (version.Info, error) {
	if parent == nil {
		return version.Info{}, fmt.Errorf("binary version probe requires a context")
	}
	ctx, cancel := context.WithTimeout(parent, probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "version", "--json")
	cmd.WaitDelay = probeTimeout
	var out boundedProbeOutput
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return version.Info{}, err
	}
	if out.overflow {
		return version.Info{}, errors.New("binary version probe output exceeds its bounded JSON envelope")
	}
	var info version.Info
	if err := json.Unmarshal(out.bytes, &info); err != nil {
		return version.Info{}, err
	}
	if strings.TrimSpace(info.Binary) == "" || strings.TrimSpace(info.Version) == "" {
		return version.Info{}, errors.New("binary version probe returned an incomplete identity envelope")
	}
	return info, nil
}

type boundedProbeOutput struct {
	bytes    []byte
	overflow bool
}

func (out *boundedProbeOutput) Write(p []byte) (int, error) {
	remaining := maxProbeJSONBytes - len(out.bytes)
	if remaining > 0 {
		n := len(p)
		if n > remaining {
			n = remaining
		}
		out.bytes = append(out.bytes, p[:n]...)
	}
	if len(p) > remaining {
		out.overflow = true
	}
	// Continue draining excess bytes so a noisy child cannot block on its
	// stdout pipe; the caller rejects the incomplete bounded envelope.
	return len(p), nil
}

// ScanHost discovers sibling sirsi binaries and queries each one's version.
// LOCAL, no network. Safe to call on every health tick.
func ScanHost() (*DriftReport, error) {
	return ScanHostContext(context.Background())
}

// ScanHostContext performs the existing local-only inventory with a caller
// bound. Each executable probe remains individually bounded by probeTimeout.
func ScanHostContext(ctx context.Context) (*DriftReport, error) {
	if ctx == nil {
		return nil, fmt.Errorf("binary drift scan requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	self := version.Current("sirsi")
	siblings, err := discoverContext(ctx, self.Path)
	if err != nil {
		return nil, err
	}

	pathBin := ""
	if p, err := exec.LookPath("sirsi"); err == nil {
		pathBin = p
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return BuildReport(self, siblings, pathBin), nil
}

// discoverContext walks candidate install dirs, probing each sirsi-family binary it
// finds (skipping the running executable itself).
func discoverContext(ctx context.Context, selfPath string) ([]Sibling, error) {
	return discoverWithProbe(ctx, selfPath, probeContextFn)
}

func discoverWithProbe(ctx context.Context, selfPath string, probe func(context.Context, string) (version.Info, error)) ([]Sibling, error) {
	var siblings []Sibling
	seen := map[string]bool{}
	selfClean := filepath.Clean(selfPath)
	for _, dir := range candidateDirs(selfPath) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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
			info, perr := probe(ctx, clean)
			if err := ctx.Err(); err != nil {
				return nil, err
			}
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
	return siblings, nil
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
		// Homebrew deliberately links /opt/homebrew/bin/sirsi to the binary
		// inside Pantheon.app. Comparing those two *names* made every healthy
		// Cask installation look drifted and sent the native app to a needless
		// self-repair screen. Prefer filesystem identity when both paths exist;
		// retain the conservative textual comparison for unavailable paths.
		if !sameExecutable(pathBin, self.Path) {
			r.D3PathBin = filepath.Clean(pathBin)
		}
	}
	r.Healthy = len(r.D2Mismatch) == 0 && r.D3PathBin == ""
	return r
}

// sameExecutable recognizes two names for the same executable object or exact
// bytes. It follows intentional PATH symlinks, but fails closed if either
// target cannot be opened, is not regular, changes while read, or no longer
// matches its retained descriptor. Byte equality describes this scan only; it
// does not promise that either name will keep resolving to those bytes later.
func sameExecutable(left, right string) bool {
	return sameExecutableWith(left, right, os.Open, os.Stat, digestExecutable)
}

type executableOpen func(string) (*os.File, error)
type executableStat func(string) (os.FileInfo, error)
type executableDigest func(*os.File) ([sha256.Size]byte, error)

func sameExecutableWith(left, right string, open executableOpen, stat executableStat, digest executableDigest) (same bool) {
	if filepath.Clean(left) == filepath.Clean(right) {
		return true
	}
	if open == nil || stat == nil || digest == nil {
		return false
	}
	leftFile, err := open(left)
	if err != nil || leftFile == nil {
		return false
	}
	defer func() {
		if err := leftFile.Close(); err != nil {
			same = false
		}
	}()
	rightFile, err := open(right)
	if err != nil || rightFile == nil {
		return false
	}
	defer func() {
		if err := rightFile.Close(); err != nil {
			same = false
		}
	}()

	leftBefore, err := leftFile.Stat()
	if err != nil || leftBefore == nil || !leftBefore.Mode().IsRegular() {
		return false
	}
	rightBefore, err := rightFile.Stat()
	if err != nil || rightBefore == nil || !rightBefore.Mode().IsRegular() {
		return false
	}
	if !executablePathStillNamesFile(left, leftBefore, stat) || !executablePathStillNamesFile(right, rightBefore, stat) {
		return false
	}
	if os.SameFile(leftBefore, rightBefore) {
		return executablePathStillNamesFile(left, leftBefore, stat) &&
			executablePathStillNamesFile(right, rightBefore, stat)
	}
	if leftBefore.Size() != rightBefore.Size() {
		return false
	}

	leftDigest, err := digest(leftFile)
	if err != nil {
		return false
	}
	rightDigest, err := digest(rightFile)
	if err != nil {
		return false
	}
	leftAfter, leftErr := leftFile.Stat()
	rightAfter, rightErr := rightFile.Stat()
	if leftErr != nil || rightErr != nil || leftAfter == nil || rightAfter == nil ||
		!stableExecutableFile(leftBefore, leftAfter) ||
		!stableExecutableFile(rightBefore, rightAfter) {
		return false
	}
	if !executablePathStillNamesFile(left, leftAfter, stat) || !executablePathStillNamesFile(right, rightAfter, stat) {
		return false
	}
	return leftDigest == rightDigest
}

func executablePathStillNamesFile(path string, opened os.FileInfo, stat executableStat) bool {
	current, err := stat(path)
	return err == nil && current != nil && current.Mode().IsRegular() && os.SameFile(opened, current)
}

func stableExecutableFile(before, after os.FileInfo) bool {
	return os.SameFile(before, after) && before.Mode() == after.Mode() &&
		before.Size() == after.Size() && before.ModTime().Equal(after.ModTime())
}

func digestExecutable(file *os.File) ([sha256.Size]byte, error) {
	var empty [sha256.Size]byte
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return empty, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return empty, err
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}
