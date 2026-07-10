package router

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPruneArtifacts(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "items"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Now().AddDate(0, 0, -90)

	// A dated quarantine dir older than the window.
	qOld := filepath.Join(root, "quarantine-20250101-flood")
	if err := os.MkdirAll(qOld, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(qOld, "dump.log"), []byte("xxxxx"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A recent dated quarantine dir — must be kept.
	qNew := filepath.Join(root, "quarantine-"+time.Now().Format("20060102")+"-recent")
	if err := os.MkdirAll(qNew, 0o755); err != nil {
		t.Fatal(err)
	}

	// A stale log (old mtime) → deleted; a big recent log → tail-capped.
	staleLog := filepath.Join(root, "logs", "autorouter.out.log")
	if err := os.WriteFile(staleLog, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().AddDate(0, 0, -120)
	if err := os.Chtimes(staleLog, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	bigLog := filepath.Join(root, "logs", "wake.log")
	big := make([]byte, maxLogTailBytes+1024)
	for i := range big {
		big[i] = 'a'
	}
	big[len(big)-600] = '\n' // ensure a newline near the tail for clean alignment
	if err := os.WriteFile(bigLog, big, 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := PruneArtifacts(root, cutoff, false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Reclaimed() == 0 {
		t.Fatalf("expected non-zero reclaim")
	}
	if _, err := os.Stat(qOld); !os.IsNotExist(err) {
		t.Errorf("old quarantine should be removed")
	}
	if _, err := os.Stat(qNew); err != nil {
		t.Errorf("recent quarantine must be kept: %v", err)
	}
	if _, err := os.Stat(staleLog); !os.IsNotExist(err) {
		t.Errorf("stale log should be deleted")
	}
	if fi, err := os.Stat(bigLog); err != nil {
		t.Errorf("capped log must still exist: %v", err)
	} else if fi.Size() > maxLogTailBytes {
		t.Errorf("log not capped: size=%d", fi.Size())
	}
}

func TestPruneArtifactsDryRunNoMutation(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "logs"), 0o755)
	qOld := filepath.Join(root, "quarantine-20250101-flood")
	_ = os.MkdirAll(qOld, 0o755)
	_ = os.WriteFile(filepath.Join(qOld, "d"), []byte("data"), 0o644)

	rep, err := PruneArtifacts(root, time.Now().AddDate(0, 0, -90), true)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun {
		t.Errorf("report should be marked dry-run")
	}
	if _, err := os.Stat(qOld); err != nil {
		t.Errorf("dry-run must not delete quarantine dir: %v", err)
	}
}

func TestPruneWorkQueue(t *testing.T) {
	root := t.TempDir()
	oldT := time.Now().AddDate(0, 0, -120)
	newT := time.Now().AddDate(0, 0, -5)
	wq := &WorkQueue{Items: []WorkItem{
		{ID: "a", Status: StatusCompleted, CompletedAt: oldT}, // drop
		{ID: "b", Status: StatusFailed, CompletedAt: oldT},    // drop
		{ID: "c", Status: StatusCompleted, CompletedAt: newT}, // keep (recent)
		{ID: "d", Status: StatusPending},                      // keep (active)
		{ID: "e", Status: StatusWorking},                      // keep (active)
	}}
	data, _ := json.MarshalIndent(wq, "", "  ")
	if err := os.WriteFile(filepath.Join(root, "work-queue.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	rep := PruneReport{}
	if err := pruneWorkQueue(root, time.Now().AddDate(0, 0, -90), false, &rep); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadWorkQueue(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Items) != 3 {
		t.Fatalf("want 3 kept, got %d", len(reloaded.Items))
	}
	kept := map[string]bool{}
	for _, it := range reloaded.Items {
		kept[it.ID] = true
	}
	for _, id := range []string{"c", "d", "e"} {
		if !kept[id] {
			t.Errorf("item %s should be kept", id)
		}
	}
}

func TestPruneSnapshot(t *testing.T) {
	root := t.TempDir()
	snap := filepath.Join(root, "processes.json")
	big := make([]byte, snapshotCapBytes+1)
	if err := os.WriteFile(snap, big, 0o644); err != nil {
		t.Fatal(err)
	}
	rep := PruneReport{}
	pruneSnapshot(snap, false, &rep)
	if _, err := os.Stat(snap); !os.IsNotExist(err) {
		t.Errorf("oversized snapshot should be removed")
	}
	if len(rep.Actions) != 1 || rep.Actions[0].Kind != "snapshot" {
		t.Errorf("expected one snapshot action, got %+v", rep.Actions)
	}

	// A small snapshot is left alone.
	small := filepath.Join(root, "small.json")
	_ = os.WriteFile(small, []byte("{}"), 0o644)
	rep2 := PruneReport{}
	pruneSnapshot(small, false, &rep2)
	if len(rep2.Actions) != 0 {
		t.Errorf("small snapshot should be untouched")
	}
}

func TestTailCapFileAlignsToLine(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.log")
	var sb strings.Builder
	for i := 0; i < 100000; i++ {
		sb.WriteString("line-of-log-data\n")
	}
	if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tailCapFile(p, 1024); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	if int64(len(data)) > 1024 {
		t.Errorf("tail cap exceeded: %d", len(data))
	}
	if len(data) > 0 && !strings.HasPrefix(string(data), "line-of-log-data") {
		t.Errorf("tail should start on a clean line boundary, got %q", string(data[:20]))
	}
}
