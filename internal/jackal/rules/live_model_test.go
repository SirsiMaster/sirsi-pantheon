package rules

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/cleaner"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
)

// TestHuggingFaceRule_SkipsLiveSNEModel exercises the rule the registry
// actually ships — not a hand-built baseScanRule — because the defect being
// guarded lives in the rule's declared path granularity as much as in the
// filter. A rule scanning the hub as ONE finding cannot exclude a served model
// without erasing every cold model alongside it.
func TestHuggingFaceRule_SkipsLiveSNEModel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, v := range []string{"HF_HOME", "HUGGINGFACE_HUB_CACHE", "TRANSFORMERS_CACHE"} {
		t.Setenv(v, "")
	}

	hub := filepath.Join(home, ".cache", "huggingface", "hub")
	servedRepo := filepath.Join(hub, "models--mlx-community--gemma-4-12B-it-8bit")
	servedSnap := filepath.Join(servedRepo, "snapshots", "200bb6db")
	coldRepo := filepath.Join(hub, "models--someone--abandoned-7b")
	coldSnap := filepath.Join(coldRepo, "snapshots", "aaaa")

	cold := time.Now().AddDate(0, 0, -90)
	for _, snap := range []string{servedSnap, coldSnap} {
		if err := os.MkdirAll(snap, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(snap, "model.safetensors"), make([]byte, 4096), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// minAgeDays=30 — both models must look cold, so the live one is excluded
	// by the substrate check and not incidentally by its own mtime.
	for _, d := range []string{servedRepo, coldRepo} {
		if err := os.Chtimes(d, cold, cold); err != nil {
			t.Fatal(err)
		}
	}

	// Stub launchd. Protection now requires the job to be LOADED, and shelling
	// out to the real launchctl would make this pass or fail based on whether
	// the developer's own machine happens to be serving a model.
	restore := cleaner.SetLoadedJobsProbe(func() map[string]cleaner.JobArgs {
		return map[string]cleaner.JobArgs{"ai.sirsi.gemma-broker": {Args: []string{"/opt/sne/sne-server", "serve", servedSnap, "127.0.0.1:8477"}}}
	})
	defer restore()

	findings, err := NewHuggingFaceCacheRule().Scan(context.Background(), jackal.ScanOptions{HomeDir: home})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var sawCold bool
	for _, f := range findings {
		if strings.HasPrefix(servedRepo, f.Path) || strings.HasPrefix(f.Path, servedRepo) {
			t.Errorf("scan reported %q as reclaimable — it holds the live SNE model at %q", f.Path, servedSnap)
		}
		if f.Path == coldRepo {
			sawCold = true
		}
	}
	if !sawCold {
		t.Errorf("scan reported no cold models; protection erased the rule instead of narrowing it (findings=%v)", findings)
	}
}
