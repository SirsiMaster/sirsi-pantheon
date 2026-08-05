package cleaner

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Live model substrate protection (Rule A1).
//
// The Sirsi Native Engine (SNE) serves a model straight out of a HuggingFace
// snapshot directory — the launchd job passes that directory as an absolute
// positional argument (see ai.sirsi.gemma-broker). Deleting it, or ANY
// directory above it, takes local inference down and costs a multi-gigabyte
// re-download, so it is protected exactly like a system path.
//
// argv is the source of truth here, deliberately: it is what the service was
// ACTUALLY started with. A config file records intent and goes stale the moment
// someone repoints the job by hand — argv cannot.

// launchAgentsDir resolves off $HOME rather than a package-level injectable
// (Rule A16), because $HOME already parameterizes it — tests point the whole
// lookup somewhere else with t.Setenv and need no mock, and there is no shared
// function pointer for concurrent scans to race on (Rule A21).
func launchAgentsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "LaunchAgents")
}

var (
	progArgsRe = regexp.MustCompile(`(?s)<key>ProgramArguments</key>\s*<array>(.*?)</array>`)
	plistStrRe = regexp.MustCompile(`<string>(.*?)</string>`)
)

// LiveModelPaths returns absolute directories held open as a model substrate by
// an installed Sirsi launchd service.
//
// Detection is by SHAPE, not by an allow-list of job names: any ai.sirsi.*
// job whose ProgramArguments contain an absolute path that resolves to an
// existing directory is treated as live substrate. A future SNE service is
// covered the day it ships, with no canon edit. Binaries and log files are
// paths too but are not directories, so they fall out naturally.
//
// ponytail: re-parses the plists on every call. They are a handful of small
// files and this runs per-deletion, not per-file-walked; add a cache only if a
// profile shows it.
func LiveModelPaths() []string {
	dir := launchAgentsDir()
	if dir == "" {
		return nil
	}
	plists, err := filepath.Glob(filepath.Join(dir, "ai.sirsi.*.plist"))
	if err != nil || len(plists) == 0 {
		return nil
	}

	var live []string
	seen := map[string]bool{}
	for _, p := range plists {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		block := progArgsRe.FindSubmatch(data)
		if block == nil {
			continue
		}
		for _, m := range plistStrRe.FindAllSubmatch(block[1], -1) {
			arg := strings.TrimSpace(string(m[1]))
			if !filepath.IsAbs(arg) || seen[arg] {
				continue
			}
			if info, err := os.Stat(arg); err == nil && info.IsDir() {
				seen[arg] = true
				live = append(live, filepath.Clean(arg))
			}
		}
	}
	return live
}

// ConflictsWithLiveModel reports whether removing path would destroy any part of
// a live model substrate, and which substrate it hit.
//
// Both directions matter. Path-inside-substrate is the obvious case; the one
// that actually bites is path-ABOVE-substrate — the HuggingFace scan rule's
// finding is a cache directory several levels above the snapshot the engine has
// open, so a same-or-below check alone would wave the whole cache through.
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
