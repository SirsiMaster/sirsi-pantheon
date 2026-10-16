package router

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodMachine = `{"id":"M1","model":"MacBookPro18,1","monitor_port":8099,"registered_at":"2026-10-16T12:00:00Z","admitted_by":"ra"}`

func TestParseMachinesRejectsIncompleteAndDuplicate(t *testing.T) {
	if _, err := parseMachines([]byte(`{"machines":[` + goodMachine + `]}`)); err != nil {
		t.Fatalf("good record refused: %v", err)
	}
	for name, body := range map[string]string{
		"no admitter": `{"machines":[{"id":"M1","model":"x","registered_at":"t"}]}`,
		"no model":    `{"machines":[{"id":"M1","registered_at":"t","admitted_by":"ra"}]}`,
		"duplicate":   `{"machines":[` + goodMachine + `,` + goodMachine + `]}`,
		"empty":       `{"machines":[]}`,
	} {
		if _, err := parseMachines([]byte(body)); err == nil {
			t.Errorf("%s: accepted, want refusal", name)
		}
	}
}

// A pinned host reads the origin snapshot; a working-tree edit must not add a machine.
func TestLoadMachinesPinnedIgnoresWorkingTree(t *testing.T) {
	snap, root := t.TempDir(), t.TempDir()
	setSnapshotDirForTest(snap)
	t.Cleanup(func() { setSnapshotDirForTest("") })
	must := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(snap, "agents.json"), `{"agents":{}}`)
	must(filepath.Join(snap, "machines.json"), `{"machines":[`+goodMachine+`]}`)
	must(filepath.Join(root, "machines.json"), `{"machines":[`+strings.Replace(goodMachine, `"M1"`, `"ROGUE"`, 1)+`]}`)
	ms, err := LoadMachines(root)
	if err != nil || len(ms) != 1 || ms[0].ID != "M1" {
		t.Fatalf("pinned read = %v, %v; want only M1 from the snapshot", ms, err)
	}
	if err := os.Remove(filepath.Join(snap, "agents.json")); err != nil { // unpinned: working tree
		t.Fatal(err)
	}
	if ms, err = LoadMachines(root); err != nil || ms[0].ID != "ROGUE" {
		t.Fatalf("unpinned read = %v, %v; want the working tree", ms, err)
	}
}

// A bad machines.json on origin must not block the agent registry sync, nor replace a good snapshot.
func TestSyncKeepsAgentsAndPriorMachinesWhenMachinesFileIsBad(t *testing.T) {
	snap, root := t.TempDir(), filepath.Join(t.TempDir(), ".agents", "idea-router")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	setSnapshotDirForTest(snap)
	t.Cleanup(func() { setSnapshotDirForTest("") })
	prior := `{"machines":[` + goodMachine + `]}`
	if err := os.WriteFile(filepath.Join(snap, "machines.json"), []byte(prior), 0o644); err != nil {
		t.Fatal(err)
	}
	git := func(_ string, args ...string) (string, error) {
		switch {
		case args[0] == "rev-parse":
			return "abc\n", nil
		case args[0] == "show" && strings.HasSuffix(args[1], "machines.json"):
			return `{"machines":[{"id":"X"}]}`, nil
		case args[0] == "show":
			return `{"agents":{"a":{"id":"a"}}}`, nil
		}
		return "", nil
	}
	if _, err := SyncRegistrySnapshot(root, git); err != nil {
		t.Fatalf("sync blocked by a bad machines.json: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(snap, "machines.json")); string(b) != prior {
		t.Fatalf("good machines snapshot was replaced by %q", b)
	}
}
