package cleaner

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// plistWithArgs writes a launchd job whose ProgramArguments carry the SNE
// launch contract shape: binary, verb, absolute model directory, flags, addr.
func plistWithArgs(t *testing.T, dir, label string, args ...string) {
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

// liveSNEFixture builds a HuggingFace hub containing one model served by a
// running SNE job and one cold model, and returns hub, servedSnapshot,
// servedRepo, coldRepo.
func liveSNEFixture(t *testing.T) (string, string, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	hub := filepath.Join(home, ".cache", "huggingface", "hub")
	servedRepo := filepath.Join(hub, "models--mlx-community--gemma-4-12B-it-8bit")
	servedSnap := filepath.Join(servedRepo, "snapshots", "200bb6db075e137a4deb08838865ac4ddb86292e")
	coldRepo := filepath.Join(hub, "models--someone--abandoned-7b")
	for _, d := range []string{servedSnap, filepath.Join(coldRepo, "snapshots", "aaaa")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	agents := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	sne := filepath.Join(home, ".sirsi", "sne", "current", "sne-server-macos-arm64")
	if err := os.MkdirAll(filepath.Dir(sne), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sne, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	plistWithArgs(t, agents, "ai.sirsi.gemma-broker",
		sne, "serve", servedSnap, "--profile", "interactive", "127.0.0.1:8477")

	return hub, servedSnap, servedRepo, coldRepo
}

func TestLiveModelPaths_ReadsServedSnapshotFromLaunchContract(t *testing.T) {
	_, servedSnap, _, _ := liveSNEFixture(t)

	got := LiveModelPaths()
	if len(got) != 1 || got[0] != servedSnap {
		t.Fatalf("LiveModelPaths() = %v, want exactly [%s]", got, servedSnap)
	}
}

// The binary and the listen address are also ProgramArguments. Only the
// directory is substrate — if either of the others leaked in, protection would
// spread to unrelated paths and the blast radius would be wrong.
func TestLiveModelPaths_IgnoresNonDirectoryArguments(t *testing.T) {
	_, servedSnap, _, _ := liveSNEFixture(t)

	for _, p := range LiveModelPaths() {
		if p != servedSnap {
			t.Errorf("LiveModelPaths() included non-substrate argument %q", p)
		}
	}
}

func TestValidatePath_BlocksLiveModelSubstrate(t *testing.T) {
	hub, servedSnap, servedRepo, coldRepo := liveSNEFixture(t)

	blocked := []struct{ name, path string }{
		{"the served snapshot itself", servedSnap},
		{"a file inside the served snapshot", filepath.Join(servedSnap, "model-00001.safetensors")},
		// The ancestor cases are the ones that actually bite: the scan rule's
		// finding is a directory ABOVE the snapshot, so a same-or-below check
		// would wave both of these straight through to os.RemoveAll.
		{"the served model's repo directory", servedRepo},
		{"the whole HuggingFace hub", hub},
	}
	for _, tc := range blocked {
		if err := ValidatePath(tc.path); err == nil {
			t.Errorf("ValidatePath(%s) = nil, want BLOCKED — deleting it destroys the live model", tc.name)
		}
	}

	// Protection must not spread: a cold sibling model is still reclaimable.
	if err := ValidatePath(coldRepo); err != nil {
		t.Errorf("ValidatePath(cold model) = %v, want nil — protection over-reached to an unserved model", err)
	}
}

func TestValidatePath_NoLiveJobLeavesHubDeletable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	hub, _, _, _ := liveSNEFixture(t)

	if _, err := DeleteFile(hub, true /*dryRun*/, true /*useTrash*/); err == nil {
		t.Error("DeleteFile(hub, dryRun=true) = nil, want BLOCKED")
	}
}
