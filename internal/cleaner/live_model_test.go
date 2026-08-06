package cleaner

import (
	"os"
	"path/filepath"
	"testing"
)

// stubJobs installs a launchd stand-in. Tests must never shell out to the real
// launchctl — the result would depend on the developer's own machine.
func stubJobs(t *testing.T, jobs map[string][]string) {
	t.Helper()
	m := make(map[string]JobArgs, len(jobs))
	for label, args := range jobs {
		m[label] = JobArgs{Args: args}
	}
	stubJobResults(t, m)
}

// stubJobResults installs discovery results verbatim, including failures.
func stubJobResults(t *testing.T, jobs map[string]JobArgs) {
	t.Helper()
	restore := SetLoadedJobsProbe(func() map[string]JobArgs { return jobs })
	t.Cleanup(restore)
}

type fixture struct {
	home, hub, servedSnap, servedRepo, coldRepo, otherSnap string
}

// liveSNEFixture builds a HuggingFace hub with one served model, one cold
// model, and a second (unserved) snapshot used by the edited-config tests.
func liveSNEFixture(t *testing.T) fixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	f := fixture{home: home}
	f.hub = filepath.Join(home, ".cache", "huggingface", "hub")
	f.servedRepo = filepath.Join(f.hub, "models--mlx-community--gemma-4-12B-it-8bit")
	f.servedSnap = filepath.Join(f.servedRepo, "snapshots", "200bb6db")
	f.otherSnap = filepath.Join(f.hub, "models--mlx-community--gemma-4-12B-it-5bit", "snapshots", "bbbb")
	f.coldRepo = filepath.Join(f.hub, "models--someone--abandoned-7b")
	for _, d := range []string{f.servedSnap, f.otherSnap, filepath.Join(f.coldRepo, "snapshots", "aaaa")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func serveArgs(modelDir string) []string {
	return []string{"/opt/sne/sne-server-macos-arm64", "serve", modelDir, "--profile", "interactive", "127.0.0.1:8477"}
}

func TestLiveModelPaths_ReadsServedSnapshotFromLoadedArgv(t *testing.T) {
	f := liveSNEFixture(t)
	stubJobs(t, map[string][]string{"ai.sirsi.gemma-broker": serveArgs(f.servedSnap)})

	got := LiveModelPaths()
	if len(got) != 1 || got[0] != f.servedSnap {
		t.Fatalf("LiveModelPaths() = %v, want exactly [%s]", got, f.servedSnap)
	}
}

// Nothing loaded means nothing protected. An operator who means to release the
// model unloads the job.
func TestLiveModelPaths_UnloadedJobProtectsNothing(t *testing.T) {
	f := liveSNEFixture(t)
	stubJobs(t, map[string][]string{})

	if got := LiveModelPaths(); len(got) != 0 {
		t.Errorf("LiveModelPaths() = %v with nothing loaded, want none", got)
	}
	if err := ValidatePath(f.hub); err != nil {
		t.Errorf("ValidatePath(hub) = %v with nothing loaded, want nil", err)
	}
}

// THE deletion-boundary false negative: the plist on disk is desired
// configuration, and editing it does not mutate an already-bootstrapped job.
// A plist-derived guard would protect model B while the loaded process still
// served model A — and would then happily delete A.
func TestLiveModelPaths_EditedPlistDoesNotMoveProtection(t *testing.T) {
	f := liveSNEFixture(t)

	// Operator edits ~/Library/LaunchAgents to point at the 5bit model but
	// never reloads. launchd still reports the 8bit argv.
	agents := filepath.Join(f.home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agents, "ai.sirsi.gemma-broker.plist"),
		[]byte("<plist><dict><key>ProgramArguments</key><array><string>serve</string><string>"+f.otherSnap+"</string></array></dict></plist>"), 0o644); err != nil {
		t.Fatal(err)
	}
	stubJobs(t, map[string][]string{"ai.sirsi.gemma-broker": serveArgs(f.servedSnap)})

	if err := ValidatePath(f.servedSnap); err == nil {
		t.Error("ValidatePath(model the loaded job is actually serving) = nil — protection followed the edited file instead of the running job")
	}
	if err := ValidatePath(f.otherSnap); err != nil {
		t.Errorf("ValidatePath(model only the edited file names) = %v, want nil — nothing is serving it", err)
	}
}

// Deleting the plist does not unload the job. A file-derived guard would see
// nothing and protect nothing while the engine kept serving.
func TestLiveModelPaths_RemovedPlistDoesNotDropProtection(t *testing.T) {
	f := liveSNEFixture(t)
	// No plist written at all — the job is loaded regardless.
	stubJobs(t, map[string][]string{"ai.sirsi.gemma-broker": serveArgs(f.servedSnap)})

	if err := ValidatePath(f.servedSnap); err == nil {
		t.Error("ValidatePath(served model) = nil with no plist on disk — protection must come from the loaded job")
	}
}

// The original detector took ANY absolute directory argument. That silently
// enrolls unrelated services and makes their trees undeletable.
func TestLiveModelPaths_IgnoresNonServeDirectoryArguments(t *testing.T) {
	f := liveSNEFixture(t)
	workDir := filepath.Join(f.home, "Development", "sirsi-pantheon", ".agents", "idea-router", "items")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stubJobs(t, map[string][]string{
		"ai.sirsi.gemma-broker":   serveArgs(f.servedSnap),
		"ai.sirsi.router-conduit": {"/bin/zsh", "-c", "conduit", "--items", workDir},
	})

	for _, p := range LiveModelPaths() {
		if p == workDir {
			t.Fatalf("LiveModelPaths() protected %q — a non-serve directory argument", workDir)
		}
	}
	if err := ValidatePath(workDir); err != nil {
		t.Errorf("ValidatePath(unrelated job's directory) = %v, want nil", err)
	}
}

// Only the argument immediately after `serve` is substrate. The binary and the
// listen address are argv too; if either leaked in the blast radius is wrong.
func TestLiveModelPaths_IgnoresBinaryAndAddressArguments(t *testing.T) {
	f := liveSNEFixture(t)
	stubJobs(t, map[string][]string{"ai.sirsi.gemma-broker": serveArgs(f.servedSnap)})

	for _, p := range LiveModelPaths() {
		if p != f.servedSnap {
			t.Errorf("LiveModelPaths() included non-substrate argument %q", p)
		}
	}
}

func TestParseLaunchctlArguments(t *testing.T) {
	printed := `ai.sirsi.gemma-broker = {
	active count = 1
	arguments = {
		/Users/x/.sirsi/sne/current/sne-server-macos-arm64
		serve
		/Users/x/.cache/huggingface/hub/models--m--g/snapshots/abc
		--profile
		interactive
		127.0.0.1:8477
	}

	stdout path = /Users/x/.sirsi/sne-server.log
}`
	got := parseLaunchctlArguments(printed)
	want := []string{
		"/Users/x/.sirsi/sne/current/sne-server-macos-arm64",
		"serve",
		"/Users/x/.cache/huggingface/hub/models--m--g/snapshots/abc",
		"--profile",
		"interactive",
		"127.0.0.1:8477",
	}
	if len(got) != len(want) {
		t.Fatalf("parseLaunchctlArguments = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("arg %d = %q, want %q", i, got[i], want[i])
		}
	}
	// Must stop at the closing brace, not run into stdout path.
	for _, a := range got {
		if a == "stdout path = /Users/x/.sirsi/sne-server.log" {
			t.Error("parser ran past the end of the arguments block")
		}
	}
}

func TestValidatePath_BlocksLiveModelSubstrate(t *testing.T) {
	f := liveSNEFixture(t)
	stubJobs(t, map[string][]string{"ai.sirsi.gemma-broker": serveArgs(f.servedSnap)})

	blocked := []struct{ name, path string }{
		{"the served snapshot itself", f.servedSnap},
		{"a file inside the served snapshot", filepath.Join(f.servedSnap, "model-00001.safetensors")},
		// The ancestor cases are the ones that bite: the scan rule's finding
		// is a directory ABOVE the snapshot, so a same-or-below check would
		// wave both straight through to os.RemoveAll.
		{"the served model's repo directory", f.servedRepo},
		{"the whole HuggingFace hub", f.hub},
	}
	for _, tc := range blocked {
		if err := ValidatePath(tc.path); err == nil {
			t.Errorf("ValidatePath(%s) = nil, want BLOCKED — deleting it destroys the live model", tc.name)
		}
	}

	if err := ValidatePath(f.coldRepo); err != nil {
		t.Errorf("ValidatePath(cold model) = %v, want nil — protection over-reached", err)
	}
}

// A dry run must refuse too. Reporting "would free 24 GB" for the live model is
// the failure the operator sees — the deletion never has to happen for the
// number to be a lie.
func TestDeleteFile_DryRunRefusesLiveModelSubstrate(t *testing.T) {
	f := liveSNEFixture(t)
	stubJobs(t, map[string][]string{"ai.sirsi.gemma-broker": serveArgs(f.servedSnap)})

	if _, err := DeleteFile(f.hub, true /*dryRun*/, true /*useTrash*/); err == nil {
		t.Error("DeleteFile(hub, dryRun=true) = nil, want BLOCKED")
	}
}

// Tree roots are never a cleanup target, and none of the existing prefix,
// basename, or $HOME-relative rules rejected them.
func TestValidatePath_RejectsTreeRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stubJobs(t, map[string][]string{})

	for _, p := range []string{"/", home, "/Volumes/ExternalDisk"} {
		if err := ValidatePath(p); err == nil {
			t.Errorf("ValidatePath(%q) = nil, want BLOCKED — it is a tree root", p)
		}
	}
	// …but only the root itself. Everything beneath must stay deletable.
	for _, p := range []string{
		filepath.Join(home, "Library", "Caches", "stale"),
		"/Volumes/ExternalDisk/some/cache",
	} {
		if err := ValidatePath(p); err != nil {
			t.Errorf("ValidatePath(%q) = %v, want nil — the guard must stop at the root", p, err)
		}
	}
}

// A loaded SNE job whose argv cannot be read is UNKNOWN authority, not absence
// of a live model. Collapsing that into an empty live set turns a broken probe
// into permission to delete the running engine's weights.
func TestValidatePath_FailsClosedWhenSNEDiscoveryFails(t *testing.T) {
	cases := []struct {
		name string
		job  JobArgs
	}{
		{"launchctl print failed", JobArgs{Err: "launchctl print failed: exit 1"}},
		{"unparseable argv", JobArgs{Err: "launchctl print returned no parseable arguments block"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := liveSNEFixture(t)
			stubJobResults(t, map[string]JobArgs{"ai.sirsi.gemma-broker": tc.job})

			if got := UnknownSubstrate(); len(got) != 1 {
				t.Fatalf("UnknownSubstrate() = %v, want one entry naming the label", got)
			}
			if err := ValidatePath(f.coldRepo); err == nil {
				t.Error("ValidatePath = nil while the live substrate is unknown — a broken probe must never read as permission to delete")
			}
			if _, err := DeleteFile(f.hub, true /*dryRun*/, true /*useTrash*/); err == nil {
				t.Error("DeleteFile dry-run = nil while the live substrate is unknown")
			}
		})
	}
}

// Keep the failure NARROW: a malformed UNRELATED ai.sirsi.* job must not freeze
// all cleanup, or one broken sibling service makes the machine un-cleanable.
func TestValidatePath_UnrelatedBrokenJobDoesNotFreezeCleanup(t *testing.T) {
	f := liveSNEFixture(t)
	stubJobResults(t, map[string]JobArgs{
		"ai.sirsi.gemma-broker":   {Args: serveArgs(f.servedSnap)},
		"ai.sirsi.router-conduit": {Err: "launchctl print failed: exit 1"},
	})

	if got := UnknownSubstrate(); len(got) != 0 {
		t.Errorf("UnknownSubstrate() = %v, want empty — the broken job is not SNE-owned", got)
	}
	if err := ValidatePath(f.coldRepo); err != nil {
		t.Errorf("ValidatePath(cold model) = %v, want nil — an unrelated broken job must not freeze cleanup", err)
	}
	// The real substrate is still protected.
	if err := ValidatePath(f.servedSnap); err == nil {
		t.Error("ValidatePath(served snapshot) = nil, want BLOCKED")
	}
}

// A total launchctl list failure must engage the same fail-closed path rather
// than presenting as "no jobs loaded".
func TestUnknownSubstrate_ListFailureIsNotEmptiness(t *testing.T) {
	liveSNEFixture(t)
	stubJobResults(t, map[string]JobArgs{
		canonicalSNELabel: {Err: "launchctl list failed: exec error"},
	})
	if got := UnknownSubstrate(); len(got) != 1 {
		t.Fatalf("UnknownSubstrate() = %v, want the list failure surfaced", got)
	}
}
