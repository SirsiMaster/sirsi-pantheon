package cleaner

import (
	"encoding/xml"
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
//   - The job must be LOADED in launchd. An installed plist is a file on disk,
//     not a running service; a job the operator unloaded protects nothing.
//   - Parsing is structured XML, so `&amp;` in a path decodes to `&`. At a
//     deletion boundary a mis-parsed path is a mis-aimed guard.
//
// LOADED, not RUNNING, is the liveness bar on purpose. A KeepAlive job being
// kickstarted is momentarily process-dead while still very much owning its
// model directory — binding to process liveness would open a window where a
// scan during a restart offers 24 GB of live weights as reclaimable.

// launchctlLoaded reports whether a launchd label is bootstrapped in the user
// domain. Injectable per Rule A16, guarded per Rule A21 — scans run
// concurrently, and an unguarded package-level pointer is the data race that
// rule exists to prevent.
var (
	loadedMu    sync.RWMutex
	loadedCheck = defaultLaunchctlLoaded
)

func getLoadedCheck() func(string) bool {
	loadedMu.RLock()
	defer loadedMu.RUnlock()
	return loadedCheck
}

func setLoadedCheck(fn func(string) bool) {
	loadedMu.Lock()
	defer loadedMu.Unlock()
	loadedCheck = fn
}

func defaultLaunchctlLoaded(label string) bool {
	// `launchctl print` exits non-zero for a label that is not bootstrapped.
	return exec.Command("launchctl", "print", "gui/"+currentUID()+"/"+label).Run() == nil
}

func currentUID() string {
	return strconv.Itoa(os.Getuid())
}

// launchAgentsDir resolves off $HOME rather than a package-level injectable —
// $HOME already parameterizes it, so tests point the whole lookup elsewhere
// with t.Setenv and no mock is needed.
func launchAgentsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "LaunchAgents")
}

// plistJob is the slice of a launchd plist this package cares about.
type plistJob struct {
	Label string
	Args  []string
}

// parsePlistJob reads Label and ProgramArguments with a real XML decoder.
//
// A plist dict pairs a <key> with the element that FOLLOWS it, which no struct
// tag expresses, so this walks tokens. Using encoding/xml rather than a regex
// is the point: CharData arrives entity-decoded, so a path containing `&`
// (written `&amp;`) is compared as the path it actually is.
func parsePlistJob(path string) (plistJob, error) {
	f, err := os.Open(path) //nolint:gosec // path comes from a LaunchAgents glob
	if err != nil {
		return plistJob{}, err
	}
	defer func() { _ = f.Close() }()

	var job plistJob
	dec := xml.NewDecoder(f)
	var pendingKey string   // the most recent <key> value
	var currentKey string   // the key whose value we are inside
	var elem string         // current leaf element name
	var inArray bool        // inside the <array> belonging to currentKey
	var buf strings.Builder // accumulates CharData for the current leaf

	for {
		tok, err := dec.Token()
		if err != nil {
			break // EOF or malformed tail — return whatever parsed cleanly
		}
		switch t := tok.(type) {
		case xml.StartElement:
			elem = t.Name.Local
			buf.Reset()
			if elem == "array" {
				currentKey = pendingKey
				inArray = true
			}
		case xml.CharData:
			if elem == "key" || elem == "string" {
				buf.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "key":
				pendingKey = strings.TrimSpace(buf.String())
			case "string":
				val := strings.TrimSpace(buf.String())
				if inArray && currentKey == "ProgramArguments" {
					job.Args = append(job.Args, val)
				} else if pendingKey == "Label" {
					job.Label = val
				}
			case "array":
				inArray = false
				currentKey = ""
			}
			elem = ""
			buf.Reset()
		}
	}
	return job, nil
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
// job. Empty when nothing is serving — this protects a running service, not a
// file that happens to sit in LaunchAgents.
func LiveModelPaths() []string {
	dir := launchAgentsDir()
	if dir == "" {
		return nil
	}
	plists, err := filepath.Glob(filepath.Join(dir, "ai.sirsi.*.plist"))
	if err != nil || len(plists) == 0 {
		return nil
	}

	isLoaded := getLoadedCheck()
	var live []string
	seen := map[string]bool{}
	for _, p := range plists {
		job, err := parsePlistJob(p)
		if err != nil || job.Label == "" {
			continue
		}
		modelDir, ok := sneModelDir(job.Args)
		if !ok || seen[modelDir] {
			continue
		}
		// launchctl last: it is the only expensive step, and the argument
		// contract has already reduced the candidate set to ~1.
		if !isLoaded(job.Label) {
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

// SetLaunchdLoadedProbe replaces the launchd liveness probe and returns a
// restore function. Exported because protection spans packages: the jackal
// scan-rule tests must assert that a live substrate is suppressed at report
// time, and they cannot reach an unexported seam. Per Rule A16 the real
// side effect stays behind an injection point; per Rule A21 the swap goes
// through the mutex rather than a bare assignment.
//
// Production code never calls this. Tests that do MUST restore, or a later
// test inherits a stubbed launchd and passes for the wrong reason.
func SetLaunchdLoadedProbe(fn func(label string) bool) (restore func()) {
	old := getLoadedCheck()
	setLoadedCheck(fn)
	return func() { setLoadedCheck(old) }
}
