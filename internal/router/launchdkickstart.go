// launchdkickstart.go — the "dead label" duty (local sovereignty P1, owner
// directive 2026-07-23): every enabled ai.sirsi.* LaunchAgent plist on disk must be
// LOADED in launchd. The morning's reboot proved the failure mode — a label
// that exits at boot (or was never bootstrapped after an edit) stays dead
// until a human or a CLOUD agent notices. This duty is deterministic shell-out
// supervision: no LLM, no network, runs on the resident supervisor.
//
// Scope deliberately narrow (A32 do-no-harm): it BOOTSTRAPS plists that are
// on disk but absent from launchd. It never kills, never kickstarts a loaded
// label (a loaded label's own RunAtLoad/StartInterval/KeepAlive policy owns
// its process lifecycle), never touches labels outside ai.sirsi.* /
// actions.runner.*, and skips .bak/.quarantined/edited-suffix files. OS-disabled
// intent requires explicit reviewed repair, not passive recovery.
package router

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// launchdDeps are the OS seams (Rule A16) — tests stub these.
type launchdDeps struct {
	listLabels     func() (map[string]bool, error) // labels currently known to launchd
	disabledLabels func() (map[string]bool, error) // labels disabled in override DB
	enableLabel    func(domain, label string) error
	bootstrapPlist func(plistPath string) error
	uid            func() int
	isQuarantined  func() bool // true if the operator deliberately stopped the gemma broker
	// isFabricQuarantined, when true, blocks reviving EVERY label — the R7/G6
	// generalization of isQuarantined (which only ever covered quarantinedLabels).
	// nil is treated as "not quarantined" so existing callers/tests are unaffected.
	isFabricQuarantined func() bool
	reviewedTargets     map[string]ManagedLaunchdPreviewTarget
}

// ManagedLaunchdRecovery is the factual result of one bounded recovery pass.
// Enabled labels may already be loaded; Bootstrapped labels were absent from
// launchd and were loaded from an exact managed plist. Keeping those outcomes
// separate prevents the UI and activity journal from calling an enable-only
// repair a process restart.
type ManagedLaunchdRecovery struct {
	Enabled      []string
	Bootstrapped []string
}

const managedLaunchdPreviewSchema = "sirsi.maat.launchd-disabled-preview.v1"
const managedLaunchdPreviewMaxPlistBytes = 1 << 20

func readManagedLaunchdPlist(path string) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open managed plist: invalid file descriptor")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("managed plist is not a regular file")
	}
	contents, err := io.ReadAll(io.LimitReader(file, managedLaunchdPreviewMaxPlistBytes+1))
	if err != nil {
		return nil, err
	}
	if len(contents) == 0 || len(contents) > managedLaunchdPreviewMaxPlistBytes {
		return nil, fmt.Errorf("managed plist is empty or exceeds the 1 MiB review limit")
	}
	return contents, nil
}

var managedLaunchdLabelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)

// ManagedLaunchdPreviewTarget is one exact eligible managed plist in a
// read-only launchd repair preview. The plist digest and loaded state bind the
// reviewed operation to both its file and the enable/bootstrap disposition.
type ManagedLaunchdPreviewTarget struct {
	Label     string `json:"label"`
	Plist     string `json:"plist"`
	SHA256    string `json:"sha256"`
	WasLoaded bool   `json:"was_loaded"`
}

// ManagedLaunchdPreview is the canonical, read-only target set for one
// operator-confirmed disabled-launchd recovery.
type ManagedLaunchdPreview struct {
	Schema  string                        `json:"schema"`
	Targets []ManagedLaunchdPreviewTarget `json:"targets"`
	Binding string                        `json:"binding"`
}

func (p ManagedLaunchdPreview) Validate() error {
	if p.Schema != managedLaunchdPreviewSchema || len(p.Targets) == 0 || len(p.Targets) > 64 || !validLaunchdDigest(p.Binding) {
		return fmt.Errorf("invalid managed launchd preview envelope")
	}
	previous := ""
	for _, target := range p.Targets {
		if target.Label <= previous || !managedLaunchdLabel(target.Label) || target.Plist != target.Label+".plist" || !managedPlist(target.Plist) || !validLaunchdDigest(target.SHA256) {
			return fmt.Errorf("invalid or non-canonical managed launchd preview target")
		}
		previous = target.Label
	}
	binding, err := managedLaunchdPreviewBinding(p.Targets)
	if err != nil || binding != p.Binding {
		return fmt.Errorf("managed launchd preview binding does not match its targets")
	}
	return nil
}

func managedLaunchdLabel(label string) bool {
	return (strings.HasPrefix(label, "ai.sirsi.") || strings.HasPrefix(label, "actions.runner.")) && managedLaunchdLabelPattern.MatchString(label)
}

func validLaunchdDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func managedLaunchdPreviewBinding(targets []ManagedLaunchdPreviewTarget) (string, error) {
	canonical := struct {
		Schema  string                        `json:"schema"`
		Targets []ManagedLaunchdPreviewTarget `json:"targets"`
	}{Schema: managedLaunchdPreviewSchema, Targets: targets}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// PreviewDisabledManagedLaunchAgents reads only current local launchd and
// managed-plist state. It never enables, bootstraps, or changes a service.
func PreviewDisabledManagedLaunchAgents() (ManagedLaunchdPreview, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return ManagedLaunchdPreview{}, fmt.Errorf("managed launchd preview: resolve user home: %w", err)
	}
	return previewManagedLaunchd(filepath.Join(home, "Library", "LaunchAgents"), launchdOS)
}

func previewManagedLaunchd(agentsDir string, deps launchdDeps) (ManagedLaunchdPreview, error) {
	if deps.isFabricQuarantined != nil && deps.isFabricQuarantined() {
		return ManagedLaunchdPreview{}, fmt.Errorf("managed launchd preview refused: the local fabric is quarantined")
	}
	disabled, err := managedDisabledSnapshot(deps.disabledLabels)
	if err != nil {
		return ManagedLaunchdPreview{}, fmt.Errorf("managed launchd preview: %w", err)
	}
	if deps.listLabels == nil {
		return ManagedLaunchdPreview{}, fmt.Errorf("managed launchd preview: loaded-label probe unavailable")
	}
	loaded, err := deps.listLabels()
	if err != nil || loaded == nil {
		if err == nil {
			err = fmt.Errorf("unknown loaded-label state")
		}
		return ManagedLaunchdPreview{}, fmt.Errorf("managed launchd preview: read loaded labels: %w", err)
	}
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return ManagedLaunchdPreview{}, fmt.Errorf("managed launchd preview: read managed LaunchAgents: %w", err)
	}
	byLabel := make(map[string]os.DirEntry, len(entries))
	for _, entry := range entries {
		if managedPlist(entry.Name()) {
			byLabel[labelForPlist(entry.Name())] = entry
		}
	}
	quarantined := deps.isQuarantined != nil && deps.isQuarantined()
	labels := make([]string, 0, len(disabled))
	for label, isDisabled := range disabled {
		if !isDisabled || (!strings.HasPrefix(label, "ai.sirsi.") && !strings.HasPrefix(label, "actions.runner.")) {
			continue
		}
		if !managedLaunchdLabel(label) {
			return ManagedLaunchdPreview{}, fmt.Errorf("managed launchd preview refused: malformed disabled managed label")
		}
		if !(quarantined && quarantinedLabels[label]) {
			labels = append(labels, label)
		}
	}
	sort.Strings(labels)
	if len(labels) == 0 || len(labels) > 64 {
		return ManagedLaunchdPreview{}, fmt.Errorf("managed launchd preview refused: eligible target set is empty or exceeds 64 labels")
	}
	preview := ManagedLaunchdPreview{Schema: managedLaunchdPreviewSchema}
	for _, label := range labels {
		entry, ok := byLabel[label]
		if !ok || entry.IsDir() {
			return ManagedLaunchdPreview{}, fmt.Errorf("managed launchd preview refused: disabled managed label %q has no regular plist", label)
		}
		path := filepath.Join(agentsDir, label+".plist")
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
			if err == nil {
				err = fmt.Errorf("target is not a regular non-symlink file")
			}
			return ManagedLaunchdPreview{}, fmt.Errorf("managed launchd preview refused: inspect target %q: %w", label, err)
		}
		if info.Size() <= 0 || info.Size() > managedLaunchdPreviewMaxPlistBytes {
			return ManagedLaunchdPreview{}, fmt.Errorf("managed launchd preview refused: target %q is empty or exceeds the 1 MiB review limit", label)
		}
		contents, err := readManagedLaunchdPlist(path)
		if err != nil {
			return ManagedLaunchdPreview{}, fmt.Errorf("managed launchd preview: read target %q: %w", label, err)
		}
		sum := sha256.Sum256(contents)
		preview.Targets = append(preview.Targets, ManagedLaunchdPreviewTarget{
			Label: label, Plist: label + ".plist", SHA256: hex.EncodeToString(sum[:]), WasLoaded: loaded[label],
		})
	}
	preview.Binding, err = managedLaunchdPreviewBinding(preview.Targets)
	if err != nil {
		return ManagedLaunchdPreview{}, fmt.Errorf("managed launchd preview: bind exact target set: %w", err)
	}
	if err := preview.Validate(); err != nil {
		return ManagedLaunchdPreview{}, fmt.Errorf("managed launchd preview: %w", err)
	}
	return preview, nil
}

// RestoreReviewedDisabledManagedLaunchAgents re-reads and validates the full
// target snapshot before entering the existing managed recovery path. Only the
// exact labels represented by the matching preview are passed as its allowlist.
func RestoreReviewedDisabledManagedLaunchAgents(binding string) (ManagedLaunchdRecovery, error) {
	if !validLaunchdDigest(binding) {
		return ManagedLaunchdRecovery{}, fmt.Errorf("managed launchd recovery requires a valid preview binding")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ManagedLaunchdRecovery{}, fmt.Errorf("managed launchd recovery: resolve user home: %w", err)
	}
	return restoreReviewedManagedLaunchd(filepath.Join(home, "Library", "LaunchAgents"), launchdOS, binding)
}

func restoreReviewedManagedLaunchd(agentsDir string, deps launchdDeps, binding string) (ManagedLaunchdRecovery, error) {
	preview, err := previewManagedLaunchd(agentsDir, deps)
	if err != nil {
		return ManagedLaunchdRecovery{}, err
	}
	if preview.Binding != binding {
		return ManagedLaunchdRecovery{}, fmt.Errorf("managed launchd preview changed; no service state was changed")
	}
	targets := make(map[string]bool, len(preview.Targets))
	reviewed := make(map[string]ManagedLaunchdPreviewTarget, len(preview.Targets))
	loaded := make(map[string]bool, len(preview.Targets))
	for _, target := range preview.Targets {
		targets[target.Label] = true
		reviewed[target.Label] = target
		loaded[target.Label] = target.WasLoaded
	}
	deps.disabledLabels = func() (map[string]bool, error) { return targets, nil }
	// Keep the reviewed enable/bootstrap disposition fixed through actuation;
	// do not widen it with another launchd list after the binding check.
	deps.listLabels = func() (map[string]bool, error) { return loaded, nil }
	deps.reviewedTargets = reviewed
	return recoverManagedLaunchd(agentsDir, deps, targets)
}

// quarantinedLabels are launchd labels this duty must never revive while the
// broker is quarantined. A deliberate operator stop (`sirsi gemma quarantine`)
// only guards RunGemmaLivenessDuty (PR #611) — it does not remove these plists
// from ~/Library/LaunchAgents, so without this check the very next kickstart
// tick silently undoes the stop, exactly the failure class PR #611 exists to
// close (codex-inference finding, 2026-08-06).
var quarantinedLabels = map[string]bool{
	"ai.sirsi.gemma-broker": true,
	"ai.sirsi.gemma-worker": true,
}

var launchdOS = launchdDeps{
	listLabels: func() (map[string]bool, error) {
		out, err := exec.Command("launchctl", "list").Output()
		if err != nil {
			return nil, err
		}
		labels := map[string]bool{}
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 3 {
				labels[fields[2]] = true
			}
		}
		return labels, nil
	},
	disabledLabels: func() (map[string]bool, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		uid := os.Getuid()
		out, err := exec.CommandContext(ctx, "/bin/launchctl", "print-disabled", fmt.Sprintf("gui/%d", uid)).Output()
		if err != nil {
			return nil, fmt.Errorf("launchctl print-disabled: %w", err)
		}
		return parseManagedDisabledLabels(string(out))
	},
	enableLabel: func(domain, label string) error {
		out, err := exec.Command("launchctl", "enable", domain+"/"+label).CombinedOutput()
		if err != nil {
			return fmt.Errorf("enable %s: %v (%s)", label, err, strings.TrimSpace(string(out)))
		}
		return nil
	},
	bootstrapPlist: func(plistPath string) error {
		uid := os.Getuid()
		out, err := exec.Command("launchctl", "bootstrap", fmt.Sprintf("gui/%d", uid), plistPath).CombinedOutput()
		if err != nil {
			return fmt.Errorf("bootstrap %s: %v (%s)", filepath.Base(plistPath), err, strings.TrimSpace(string(out)))
		}
		return nil
	},
	uid: os.Getuid,
	isQuarantined: func() bool {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		_, err = os.Stat(QuarantineMarkerPath(home))
		return err == nil
	},
	isFabricQuarantined: func() bool {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		return IsFabricQuarantined(home)
	},
}

// Unlike a display-only parser, recovery must distinguish an empty override
// map from missing/malformed output. Both launchd boolean and word forms are
// accepted; space-joined keys retain their established per-label meaning.
func parseManagedDisabledLabels(output string) (map[string]bool, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "disabled services = {" || strings.TrimSpace(lines[len(lines)-1]) != "}" {
		return nil, fmt.Errorf("launchctl print-disabled: unknown output")
	}
	entry := regexp.MustCompile(`^"([^"\\]+)"\s*=>\s*(disabled|enabled|true|false)$`)
	disabled := map[string]bool{}
	seen := map[string]bool{}
	for _, line := range lines[1 : len(lines)-1] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		match := entry.FindStringSubmatch(line)
		if len(match) != 3 || len(strings.Fields(match[1])) == 0 {
			return nil, fmt.Errorf("launchctl print-disabled: malformed override entry")
		}
		value := match[2] == "disabled" || match[2] == "true"
		for _, label := range strings.Fields(match[1]) {
			if previous, ok := seen[label]; ok && previous != value {
				return nil, fmt.Errorf("launchctl print-disabled: contradictory override entries")
			}
			seen[label] = value
			if value {
				disabled[label] = true
			}
		}
	}
	return disabled, nil
}

func managedDisabledSnapshot(probe func() (map[string]bool, error)) (map[string]bool, error) {
	if probe == nil {
		return nil, fmt.Errorf("read disabled launchd overrides: probe unavailable")
	}
	disabled, err := probe()
	if err != nil {
		return nil, fmt.Errorf("read disabled launchd overrides: %w", err)
	}
	if disabled == nil {
		return nil, fmt.Errorf("read disabled launchd overrides: unknown state")
	}
	return disabled, nil
}

// managedPlist reports whether a LaunchAgents entry is one of ours and a real
// plist (not a backup/quarantine artifact).
func managedPlist(name string) bool {
	if !strings.HasSuffix(name, ".plist") {
		return false // .bak-*, .quarantined, editor droppings
	}
	return strings.HasPrefix(name, "ai.sirsi.") || strings.HasPrefix(name, "actions.runner.")
}

// labelForPlist derives the launchd label from the plist filename. Sirsi and
// actions-runner plists are named exactly <label>.plist by convention.
func labelForPlist(name string) string {
	return strings.TrimSuffix(name, ".plist")
}

// recoverManagedLaunchd restores managed, on-disk plists in two explicit
// stages. Passive callers (nil targets) never clear disabled intent or
// bootstrap disabled labels. A non-nil target set belongs to explicit confirmed
// repair, which may clear disabled overrides and bootstrap absent targets only.
// It is an exact label allowlist, so a user-confirmed repair cannot incidentally
// revive another managed service. It never revives quarantined Gemma labels or
// anything outside the managed filename allowlist.
func recoverManagedLaunchd(agentsDir string, deps launchdDeps, targets map[string]bool) (ManagedLaunchdRecovery, error) {
	var names []string
	if targets != nil && deps.reviewedTargets != nil {
		if len(targets) == 0 || len(deps.reviewedTargets) != len(targets) {
			return ManagedLaunchdRecovery{}, fmt.Errorf("reviewed managed launchd target set is empty or inconsistent")
		}
		for label, selected := range targets {
			if !selected || !managedLaunchdLabel(label) {
				return ManagedLaunchdRecovery{}, fmt.Errorf("reviewed managed launchd target %q is invalid", label)
			}
			names = append(names, label+".plist")
		}
		sort.Strings(names)
		for _, name := range names {
			label := labelForPlist(name)
			path := filepath.Join(agentsDir, name)
			info, statErr := os.Lstat(path)
			if statErr != nil || info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
				if statErr == nil {
					statErr = fmt.Errorf("target is not a regular non-symlink file")
				}
				return ManagedLaunchdRecovery{}, fmt.Errorf("reviewed managed launchd target %q is missing or malformed: %w", label, statErr)
			}
			expected, ok := deps.reviewedTargets[label]
			if !ok || info.Size() <= 0 || info.Size() > managedLaunchdPreviewMaxPlistBytes {
				return ManagedLaunchdRecovery{}, fmt.Errorf("reviewed managed launchd target %q is missing or exceeds the review limit", label)
			}
			contents, readErr := readManagedLaunchdPlist(path)
			if readErr != nil {
				return ManagedLaunchdRecovery{}, fmt.Errorf("read reviewed managed launchd target %q: %w", label, readErr)
			}
			sum := sha256.Sum256(contents)
			if hex.EncodeToString(sum[:]) != expected.SHA256 {
				return ManagedLaunchdRecovery{}, fmt.Errorf("reviewed managed launchd target %q changed before recovery", label)
			}
		}
	} else {
		entries, err := os.ReadDir(agentsDir)
		if err != nil {
			if os.IsNotExist(err) {
				return ManagedLaunchdRecovery{}, nil
			}
			return ManagedLaunchdRecovery{}, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				names = append(names, entry.Name())
			}
		}
	}
	// Fabric-wide stand-down (R7/G6): honored BEFORE touching launchd at all,
	// so a quarantined fabric reads as zero revivals rather than "everything
	// except the labels this duty happens to know about."
	if deps.isFabricQuarantined != nil && deps.isFabricQuarantined() {
		return ManagedLaunchdRecovery{}, nil
	}

	loaded, err := deps.listLabels()
	if err != nil {
		return ManagedLaunchdRecovery{}, fmt.Errorf("launchctl list: %w", err)
	}
	// Unknown operator intent is not permission to revive a service.
	disabled, err := managedDisabledSnapshot(deps.disabledLabels)
	if err != nil {
		return ManagedLaunchdRecovery{}, err
	}

	// Recovery is also used by descriptor/symlink safety checks whose test seam
	// does not need a synthetic uid. Keep that path safe instead of panicking
	// before it can reject an unsafe managed plist.
	uid := os.Getuid()
	if deps.uid != nil {
		uid = deps.uid()
	}
	domain := fmt.Sprintf("gui/%d", uid)

	quarantined := deps.isQuarantined != nil && deps.isQuarantined()

	var recovery ManagedLaunchdRecovery
	var firstErr error
	for _, name := range names {
		if !managedPlist(name) {
			continue
		}
		plistPath := filepath.Join(agentsDir, name)
		info, statErr := os.Lstat(plistPath)
		if statErr != nil || info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
			if firstErr == nil {
				if statErr != nil {
					firstErr = fmt.Errorf("inspect managed plist %s: %w", name, statErr)
				} else {
					firstErr = fmt.Errorf("managed plist %s is not a regular non-symlink file", name)
				}
			}
			continue
		}
		label := labelForPlist(name)
		if targets != nil && !targets[label] {
			continue
		}
		if quarantined && quarantinedLabels[label] {
			continue // deliberate operator stop — never silently revive
		}
		if targets == nil && disabled[label] {
			continue // OS-disabled intent is preserved by passive supervision
		}
		// Enable before bootstrap when the override DB has disabled this label.
		// Without this step, bootstrap silently fails (Operation not permitted)
		// and the duty appears to succeed — the exact failure mode from 2026-07-31.
		if disabled[label] && deps.enableLabel != nil {
			if err := deps.enableLabel(domain, label); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			recovery.Enabled = append(recovery.Enabled, label)
		}
		if loaded[label] {
			continue // enabled above if needed; its own launchd policy owns its process
		}
		if err := deps.bootstrapPlist(plistPath); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		recovery.Bootstrapped = append(recovery.Bootstrapped, label)
	}
	return recovery, firstErr
}

// KickstartDeadLabels is the supervisor-compatible wrapper. Its return value
// remains limited to labels that were absent and bootstrapped, so historical
// callers cannot mistake an explicit repair's enable-only result for a reload.
// This passive path never enables or bootstraps an OS-disabled label.
func KickstartDeadLabels(agentsDir string, deps launchdDeps) ([]string, error) {
	recovery, err := recoverManagedLaunchd(agentsDir, deps, nil)
	return recovery.Bootstrapped, err
}

// RestoreDisabledManagedLaunchAgents is the explicit, operator-confirmed
// repair used by `sirsi liveness-watch restore-disabled`. It snapshots the
// disabled override labels and applies the shared managed-plist recovery only
// to that allowlist. Unlike the resident supervisor sweep, it never uses this
// UI action as authority to revive a different unloaded service.
func RestoreDisabledManagedLaunchAgents() (ManagedLaunchdRecovery, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return ManagedLaunchdRecovery{}, err
	}
	disabled, err := managedDisabledSnapshot(launchdOS.disabledLabels)
	if err != nil {
		return ManagedLaunchdRecovery{}, err
	}
	// Preserve the snapshot through the recovery pass. The live state can only
	// become less actionable after a concurrent enable; the fixed target set
	// prevents a newly-disabled unrelated label from expanding this repair.
	deps := launchdOS
	deps.disabledLabels = func() (map[string]bool, error) { return disabled, nil }
	return recoverManagedLaunchd(filepath.Join(home, "Library", "LaunchAgents"), deps, disabled)
}

// Kickstart wiring follows the gemma-liveness seam pattern (Rule A16/A21):
// INERT by default so library consumers and tests never shell real launchctl;
// cmd/sirsi wires RunLaunchdKickstartDuty at init.
var (
	kickstartMu sync.RWMutex
	kickstartFn = func(routerRoot, repoRoot string) error { return nil }
)

// SetLaunchdKickstartFn installs the real kickstart pass.
func SetLaunchdKickstartFn(fn func(routerRoot, repoRoot string) error) {
	kickstartMu.Lock()
	defer kickstartMu.Unlock()
	if fn != nil {
		kickstartFn = fn
	}
}

func getLaunchdKickstartFn() func(string, string) error {
	kickstartMu.RLock()
	defer kickstartMu.RUnlock()
	return kickstartFn
}

// RunLaunchdKickstartDuty is the real duty pass: revive dead labels and
// record each revival as an owner-visible heal.
func RunLaunchdKickstartDuty(routerRoot, repoRoot string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	revived, err := KickstartDeadLabels(filepath.Join(home, "Library", "LaunchAgents"), launchdOS)
	for _, label := range revived {
		RecordHeal(fmt.Sprintf("background service %s was not running — reloaded", label))
	}
	return err
}
