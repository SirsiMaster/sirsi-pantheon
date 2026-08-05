package cleaner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeJob writes a launchd plist with the given label and ProgramArguments.
func writeJob(t *testing.T, dir, label string, args ...string) {
	t.Helper()
	body := ""
	for _, a := range args {
		body += fmt.Sprintf("\t\t<string>%s</string>\n", a)
	}
	doc := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
%s	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`, label, body)
	if err := os.WriteFile(filepath.Join(dir, label+".plist"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
}

// allLoaded/noneLoaded install a launchctl stand-in. Tests must never shell out
// to the real launchctl — the result would depend on the developer's machine.
func allLoaded(t *testing.T) {
	t.Helper()
	old := getLoadedCheck()
	setLoadedCheck(func(string) bool { return true })
	t.Cleanup(func() { setLoadedCheck(old) })
}

func loadedOnly(t *testing.T, labels ...string) {
	t.Helper()
	set := map[string]bool{}
	for _, l := range labels {
		set[l] = true
	}
	old := getLoadedCheck()
	setLoadedCheck(func(l string) bool { return set[l] })
	t.Cleanup(func() { setLoadedCheck(old) })
}

type fixture struct {
	home, hub, servedSnap, servedRepo, coldRepo, agents string
}

// liveSNEFixture builds a HuggingFace hub with one served and one cold model,
// plus an installed gemma-broker job following the real serve contract.
func liveSNEFixture(t *testing.T) fixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	f := fixture{home: home}
	f.hub = filepath.Join(home, ".cache", "huggingface", "hub")
	f.servedRepo = filepath.Join(f.hub, "models--mlx-community--gemma-4-12B-it-8bit")
	f.servedSnap = filepath.Join(f.servedRepo, "snapshots", "200bb6db075e137a4deb08838865ac4ddb86292e")
	f.coldRepo = filepath.Join(f.hub, "models--someone--abandoned-7b")
	f.agents = filepath.Join(home, "Library", "LaunchAgents")
	for _, d := range []string{f.servedSnap, filepath.Join(f.coldRepo, "snapshots", "aaaa"), f.agents} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	sne := filepath.Join(home, ".sirsi", "sne", "current", "sne-server-macos-arm64")
	if err := os.MkdirAll(filepath.Dir(sne), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sne, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeJob(t, f.agents, "ai.sirsi.gemma-broker",
		sne, "serve", f.servedSnap, "--profile", "interactive", "127.0.0.1:8477")
	return f
}

func TestLiveModelPaths_ReadsServedSnapshotFromServeContract(t *testing.T) {
	f := liveSNEFixture(t)
	allLoaded(t)

	got := LiveModelPaths()
	if len(got) != 1 || got[0] != f.servedSnap {
		t.Fatalf("LiveModelPaths() = %v, want exactly [%s]", got, f.servedSnap)
	}
}

// An installed plist is a file on disk, not a running service. A job the
// operator unloaded must protect nothing, or every stale plist ever left in
// LaunchAgents permanently freezes a tree.
func TestLiveModelPaths_StoppedJobProtectsNothing(t *testing.T) {
	f := liveSNEFixture(t)
	loadedOnly(t /* nothing is loaded */)

	if got := LiveModelPaths(); len(got) != 0 {
		t.Errorf("LiveModelPaths() = %v with the job unloaded, want none", got)
	}
	if err := ValidatePath(f.hub); err != nil {
		t.Errorf("ValidatePath(hub) = %v with the job unloaded, want nil", err)
	}
}

// The original detector took ANY absolute directory argument of ANY ai.sirsi
// job. That silently enrolls unrelated services — a job passing a working
// directory, a data root, a queue path — and makes their trees undeletable
// with no way for the operator to see why.
func TestLiveModelPaths_IgnoresNonServeDirectoryArguments(t *testing.T) {
	f := liveSNEFixture(t)
	allLoaded(t)

	workDir := filepath.Join(f.home, "Development", "sirsi-pantheon", ".agents", "idea-router", "items")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A real-shaped sibling job: absolute directory argument, no serve verb.
	writeJob(t, f.agents, "ai.sirsi.router-conduit",
		"/bin/zsh", "-c", "conduit", "--items", workDir)

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
// listen address are ProgramArguments too; if either leaked in, the blast
// radius would be wrong.
func TestLiveModelPaths_IgnoresBinaryAndAddressArguments(t *testing.T) {
	f := liveSNEFixture(t)
	allLoaded(t)

	for _, p := range LiveModelPaths() {
		if p != f.servedSnap {
			t.Errorf("LiveModelPaths() included non-substrate argument %q", p)
		}
	}
}

// Regex parsing compares the escaped text; a path containing "&" is written
// "&amp;" in the plist and would never match the real path on disk — the guard
// would silently aim at nothing.
func TestLiveModelPaths_DecodesXMLEntitiesInPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	allLoaded(t)

	modelDir := filepath.Join(home, "models", "r&d-checkpoint")
	agents := filepath.Join(home, "Library", "LaunchAgents")
	for _, d := range []string{modelDir, agents} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeJob(t, agents, "ai.sirsi.gemma-broker",
		"/opt/sne", "serve", "/PLACEHOLDER", "127.0.0.1:8477")
	// Write the entity form the way launchd actually stores it.
	plist := filepath.Join(agents, "ai.sirsi.gemma-broker.plist")
	b, err := os.ReadFile(plist)
	if err != nil {
		t.Fatal(err)
	}
	escaped := filepath.Join(home, "models", "r&amp;d-checkpoint")
	if err := os.WriteFile(plist, []byte(strings.ReplaceAll(string(b), "/PLACEHOLDER", escaped)), 0o644); err != nil {
		t.Fatal(err)
	}

	got := LiveModelPaths()
	if len(got) != 1 || got[0] != modelDir {
		t.Fatalf("LiveModelPaths() = %v, want [%s] — the entity must decode to the real path", got, modelDir)
	}
}

func TestValidatePath_BlocksLiveModelSubstrate(t *testing.T) {
	f := liveSNEFixture(t)
	allLoaded(t)

	blocked := []struct{ name, path string }{
		{"the served snapshot itself", f.servedSnap},
		{"a file inside the served snapshot", filepath.Join(f.servedSnap, "model-00001.safetensors")},
		// The ancestor cases are the ones that actually bite: the scan rule's
		// finding is a directory ABOVE the snapshot, so a same-or-below check
		// would wave both of these straight through to os.RemoveAll.
		{"the served model's repo directory", f.servedRepo},
		{"the whole HuggingFace hub", f.hub},
	}
	for _, tc := range blocked {
		if err := ValidatePath(tc.path); err == nil {
			t.Errorf("ValidatePath(%s) = nil, want BLOCKED — deleting it destroys the live model", tc.name)
		}
	}

	// Protection must not spread: a cold sibling model is still reclaimable.
	if err := ValidatePath(f.coldRepo); err != nil {
		t.Errorf("ValidatePath(cold model) = %v, want nil — protection over-reached to an unserved model", err)
	}
}

func TestValidatePath_NoLiveJobLeavesHubDeletable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	allLoaded(t)

	hub := filepath.Join(home, ".cache", "huggingface", "hub")
	if err := os.MkdirAll(hub, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ValidatePath(hub); err != nil {
		t.Errorf("ValidatePath(hub) = %v with no SNE job installed, want nil", err)
	}
}

// A dry run must refuse too. Reporting "would free 24 GB" for the live model is
// the failure the owner sees — the deletion never has to happen for the number
// to be a lie.
func TestDeleteFile_DryRunRefusesLiveModelSubstrate(t *testing.T) {
	f := liveSNEFixture(t)
	allLoaded(t)

	if _, err := DeleteFile(f.hub, true /*dryRun*/, true /*useTrash*/); err == nil {
		t.Error("DeleteFile(hub, dryRun=true) = nil, want BLOCKED")
	}
}

// Tree roots are never a cleanup target, and none of the existing prefix,
// basename, or $HOME-relative rules rejected them.
func TestValidatePath_RejectsTreeRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	allLoaded(t)

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
