package cleaner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Live model substrate protection (Rule A1).
//
// The Sirsi Native Engine (SNE) serves a model straight out of a HuggingFace
// snapshot directory named on the launchd job's command line. Deleting it, or
// any directory above it, takes local inference down and costs a
// multi-gigabyte re-download, so it is protected exactly like a system path.
//
// Three deliberate narrowings, each one closing a way this could protect too
// much (codex-pantheon review of PR #493):
//
//   - Detection follows the SNE *serve contract* — the positional argument
//     after the literal `serve` verb — not "any absolute directory argument".
//     The loose form would enroll any future job that happens to pass a working
//     directory, silently making unrelated trees undeletable.
//   - Authority is the LOADED launchd job's own argv, read back from
//     `launchctl print`. The plist on disk is *desired* configuration and is
//     not consulted at all: editing it does not mutate an already-bootstrapped
//     job, so a plist-derived guard would protect model B while the loaded
//     process still served model A — and would protect nothing at all if the
//     file were deleted out from under a running job. Both are deletion-
//     boundary false negatives.
//
// LOADED, not RUNNING, is the liveness bar on purpose (ruled by codex-pantheon
// on PR #493). A KeepAlive job being kickstarted is momentarily process-dead
// while still owning its model directory; binding to process liveness would
// open a window where a scan during a restart offers live weights as
// reclaimable. An operator who means to release the model unloads the job.

// loadedJobs returns the argv of every loaded ai.sirsi.* launchd job, keyed by
// label. Injectable per Rule A16, guarded per Rule A21 — scans run
// concurrently, and an unguarded package-level pointer is the data race that
// rule exists to prevent.
var (
	jobsMu    sync.RWMutex
	jobsProbe = defaultLoadedJobs
)

func getJobsProbe() func() map[string][]string {
	jobsMu.RLock()
	defer jobsMu.RUnlock()
	return jobsProbe
}

func setJobsProbe(fn func() map[string][]string) {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	jobsProbe = fn
}

// SetLoadedJobsProbe replaces the launchd job reader and returns a restore
// function. Exported because protection spans packages: the jackal scan-rule
// tests must assert that a live substrate is suppressed at report time, and
// they cannot reach an unexported seam.
//
// Production code never calls this. Tests that do MUST restore, or a later
// test inherits a stubbed launchd and passes for the wrong reason.
func SetLoadedJobsProbe(fn func() map[string][]string) (restore func()) {
	old := getJobsProbe()
	setJobsProbe(fn)
	return func() { setJobsProbe(old) }
}

// defaultLoadedJobs asks launchd what it is actually running.
func defaultLoadedJobs() map[string][]string {
	out, err := exec.Command("launchctl", "list").Output()
	if err != nil {
		return nil
	}
	jobs := map[string][]string{}
	uid := strconv.Itoa(os.Getuid())
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		label := fields[len(fields)-1]
		if !strings.HasPrefix(label, "ai.sirsi.") {
			continue
		}
		printed, err := exec.Command("launchctl", "print", "gui/"+uid+"/"+label).Output()
		if err != nil {
			continue // not bootstrapped in this domain
		}
		if args := parseLaunchctlArguments(string(printed)); len(args) > 0 {
			jobs[label] = args
		}
	}
	return jobs
}

// parseLaunchctlArguments extracts the argv from `launchctl print` output.
//
//	arguments = {
//		/path/to/binary
//		serve
//		/path/to/model
//	}
//
// This is plain text, not XML — the entity-decoding concern that applied to
// reading the plist directly does not exist here.
func parseLaunchctlArguments(printed string) []string {
	var args []string
	inBlock := false
	for _, line := range strings.Split(printed, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case !inBlock && strings.HasPrefix(t, "arguments") && strings.HasSuffix(t, "{"):
			inBlock = true
		case inBlock && t == "}":
			return args
		case inBlock && t != "":
			args = append(args, t)
		}
	}
	return args
}

// sneModelDir returns the model directory named by an SNE serve invocation.
//
// The contract is positional: `<engine> serve <modelDir> [flags…] <addr>`.
// Only the argument immediately after the literal `serve` counts. Anything
// looser — "any absolute directory in the argument list" — would quietly
// enroll unrelated jobs that pass a working directory or data root, and turn
// their trees undeletable with no way for the operator to tell why.
func sneModelDir(args []string) (string, bool) {
	for i, a := range args {
		if a != "serve" || i+1 >= len(args) {
			continue
		}
		cand := args[i+1]
		if !filepath.IsAbs(cand) {
			return "", false
		}
		info, err := os.Stat(cand)
		if err != nil || !info.IsDir() {
			return "", false
		}
		return filepath.Clean(cand), true
	}
	return "", false
}

// LiveModelPaths returns model directories held open by a LOADED Sirsi engine
// job, derived from that job's own argv as launchd reports it. Empty when
// nothing is serving.
func LiveModelPaths() []string {
	var live []string
	seen := map[string]bool{}
	for _, args := range getJobsProbe()() {
		modelDir, ok := sneModelDir(args)
		if !ok || seen[modelDir] {
			continue
		}
		seen[modelDir] = true
		live = append(live, modelDir)
	}
	return live
}

// ConflictsWithLiveModel reports whether removing path would destroy any part
// of a live model substrate, and which substrate it hit.
//
// Both directions matter. Path-inside-substrate is the obvious case; the one
// that actually bites is path-ABOVE-substrate — a scan rule's finding is a
// cache directory several levels above the snapshot the engine has open, so a
// same-or-below check alone would wave the whole cache through.
func ConflictsWithLiveModel(path string) (string, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	abs = filepath.Clean(abs)
	for _, live := range LiveModelPaths() {
		if withinOrEqual(live, abs) || withinOrEqual(abs, live) {
			return live, true
		}
	}
	return "", false
}

func withinOrEqual(parent, child string) bool {
	if parent == child {
		return true
	}
	return strings.HasPrefix(child, strings.TrimSuffix(parent, string(os.PathSeparator))+string(os.PathSeparator))
}

// isCatastrophicRoot reports whether a path is a tree root whose removal is
// never a legitimate cleanup: the filesystem root, a mounted volume's top
// level, or the user's home directory itself.
//
// ValidatePath allowed "/" — its protected prefixes are all deeper (/System/,
// /usr/, /Library/Extensions/…), and none of them is a prefix of "/" — and a
// test pinned that as intended. Found while making that test hermetic in
// PR #493; codex-pantheon ruled it a blocking defect rather than a curiosity.
func isCatastrophicRoot(abs string) bool {
	if abs == string(os.PathSeparator) {
		return true
	}
	// A volume's top level: /Volumes/<name>, no deeper.
	if rest, ok := strings.CutPrefix(abs, "/Volumes/"); ok && rest != "" && !strings.Contains(rest, "/") {
		return true
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if filepath.Clean(home) == abs {
			return true
		}
	}
	return false
}
