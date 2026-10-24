package thoth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPruneJournalCivilDateRetention(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		now       time.Time
		maxAge    int
		keepDates []string
		dropDates []string
	}{
		{
			name: "UTC-next-day-local-evening",
			now:  time.Date(2026, 10, 23, 20, 6, 0, 0, newYork), maxAge: 1,
			keepDates: []string{"2026-10-23", "2026-10-22"},
			dropDates: []string{"2026-10-21"},
		},
		{
			name: "exact-N-and-N-plus-one",
			now:  time.Date(2026, 10, 23, 23, 59, 0, 0, newYork), maxAge: 3,
			keepDates: []string{"2026-10-23", "2026-10-20"},
			dropDates: []string{"2026-10-19"},
		},
		{
			name: "25-hour-fall-back-day",
			now:  time.Date(2026, 11, 1, 23, 30, 0, 0, newYork), maxAge: 1,
			keepDates: []string{"2026-11-01", "2026-10-31"},
			dropDates: []string{"2026-10-30"},
		},
		{
			name: "23-hour-spring-forward-day",
			now:  time.Date(2026, 3, 8, 23, 30, 0, 0, newYork), maxAge: 1,
			keepDates: []string{"2026-03-08", "2026-03-07"},
			dropDates: []string{"2026-03-06"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, ".thoth")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			journal := "# Civil-date fixture\n\n"
			dates := append(append([]string(nil), tc.dropDates...), tc.keepDates...)
			for i, date := range dates {
				journal += fmt.Sprintf("## Entry %03d — %s 20:06 — Fixture\n\nbody-%s\n\n---\n", i+1, date, date)
			}
			path := filepath.Join(dir, "journal.md")
			if err := os.WriteFile(path, []byte(journal), 0600); err != nil {
				t.Fatal(err)
			}
			removed, err := pruneJournalAt(PruneOptions{RepoRoot: root, MaxAge: tc.maxAge}, tc.now)
			if err != nil {
				t.Fatal(err)
			}
			if removed != len(tc.dropDates) {
				t.Fatalf("removed=%d, want %d", removed, len(tc.dropDates))
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, date := range tc.keepDates {
				if !strings.Contains(string(data), "body-"+date) {
					t.Errorf("retention removed eligible civil date %s", date)
				}
			}
			for _, date := range tc.dropDates {
				if strings.Contains(string(data), "body-"+date) {
					t.Errorf("retention kept expired civil date %s", date)
				}
			}
		})
	}
}

func TestCompactRetentionPolicy(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // Isolate Stele's default path as well as journal fixtures.
	for _, tc := range []struct {
		name string
		opts CompactOptions
		want int
	}{
		{"explicit-zero-preserves", CompactOptions{NoDefaultPrune: true}, defaultMaxKeep + 6},
		{"omitted-uses-default", CompactOptions{}, defaultMaxKeep},
		{"positive-count", CompactOptions{MaxKeep: 3}, 3},
		{"positive-age", CompactOptions{MaxAge: 1}, 1},
		{"positive-count-with-intent", CompactOptions{MaxKeep: 3, NoDefaultPrune: true}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, ".thoth")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "memory.yaml"), []byte("project: fixture\n"), 0600); err != nil {
				t.Fatal(err)
			}
			original := "# Retained journal\n\n"
			for i := 1; i <= defaultMaxKeep+5; i++ {
				original += fmt.Sprintf("## Entry %03d — 2000-01-01 — Original %d\n\noriginal-body-%03d\n\n---\n\n", i, i, i)
			}
			path := filepath.Join(dir, "journal.md")
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			opts := tc.opts
			opts.RepoRoot, opts.Summary = root, "Retain explicit policy"
			if err := Compact(opts); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got := string(data)
			if n := strings.Count(got, "## Entry "); n != tc.want {
				t.Fatalf("entries=%d, want %d", n, tc.want)
			}
			if !strings.Contains(got, "Retain explicit policy") || !strings.HasPrefix(got, "# Retained journal\n") {
				t.Fatal("new entry or journal header missing")
			}
			if tc.name == "explicit-zero-preserves" {
				if !strings.HasPrefix(got, original) {
					t.Fatal("explicit zero policy changed original journal bytes")
				}
			} else if strings.Contains(got, "original-body-001") {
				t.Fatal("pruning policy did not remove oldest entry")
			}
		})
	}
}

func TestCompact_AppendsSessionDecisions(t *testing.T) {
	tmp := t.TempDir()
	thothDir := filepath.Join(tmp, ".thoth")
	os.MkdirAll(thothDir, 0755)

	// Create minimal memory.yaml
	memPath := filepath.Join(thothDir, "memory.yaml")
	os.WriteFile(memPath, []byte("project: test\nversion: 1.0\n"), 0644)

	// Create minimal journal
	journalPath := filepath.Join(thothDir, "journal.md")
	os.WriteFile(journalPath, []byte("# Journal\n---\n"), 0644)

	err := Compact(CompactOptions{
		RepoRoot: tmp,
		Summary:  "Use interface-based providers\nKeep backward compat",
	})
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}

	// Check memory.yaml has session decisions
	data, _ := os.ReadFile(memPath)
	content := string(data)
	if !strings.Contains(content, "## Session Decisions") {
		t.Error("expected Session Decisions section in memory.yaml")
	}
	if !strings.Contains(content, "Use interface-based providers") {
		t.Error("expected decision text in memory.yaml")
	}
}

func TestCompact_CreatesJournalEntry(t *testing.T) {
	tmp := t.TempDir()
	thothDir := filepath.Join(tmp, ".thoth")
	os.MkdirAll(thothDir, 0755)
	os.WriteFile(filepath.Join(thothDir, "memory.yaml"), []byte("project: test\n"), 0644)
	os.WriteFile(filepath.Join(thothDir, "journal.md"), []byte("# Journal\n---\n"), 0644)

	err := Compact(CompactOptions{RepoRoot: tmp, Summary: "Decision one"})
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(thothDir, "journal.md"))
	content := string(data)
	if !strings.Contains(content, "(COMPACT)") {
		t.Error("expected (COMPACT) marker in journal entry")
	}
	if !strings.Contains(content, "Decision one") {
		t.Error("expected decision text in journal entry")
	}
}

func TestCompact_IncludesRouterSnapshot(t *testing.T) {
	tmp := t.TempDir()
	thothDir := filepath.Join(tmp, ".thoth")
	routerDir := filepath.Join(tmp, ".agents", "idea-router")
	os.MkdirAll(thothDir, 0755)
	os.MkdirAll(routerDir, 0755)
	os.WriteFile(filepath.Join(thothDir, "memory.yaml"), []byte("project: test\n"), 0644)
	os.WriteFile(filepath.Join(thothDir, "journal.md"), []byte("# Journal\n---\n"), 0644)
	os.WriteFile(filepath.Join(routerDir, "state.json"), []byte(`{
  "active_topics": ["router-v3-multi-agent-queue"],
  "completed_topics": ["autorouter-daemon-v2"],
  "pending_for_claude": ["20260517-codex-router-v3-plan-review"],
  "last_codex_read": "2026-05-17T19:39:27-04:00",
  "last_claude_read": "2026-05-17T17:00:00Z"
}`), 0644)

	err := Compact(CompactOptions{RepoRoot: tmp, Summary: "Preserve current work"})
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(thothDir, "journal.md"))
	content := string(data)
	if !strings.Contains(content, "Router snapshot:") {
		t.Error("expected router snapshot in journal")
	}
	if !strings.Contains(content, "router-v3-multi-agent-queue") {
		t.Error("expected active router topic in journal")
	}
	if !strings.Contains(content, "20260517-codex-router-v3-plan-review") {
		t.Error("expected pending router item in journal")
	}
}

func TestCompact_IdempotentSection(t *testing.T) {
	tmp := t.TempDir()
	thothDir := filepath.Join(tmp, ".thoth")
	os.MkdirAll(thothDir, 0755)
	os.WriteFile(filepath.Join(thothDir, "memory.yaml"), []byte("project: test\n"), 0644)
	os.WriteFile(filepath.Join(thothDir, "journal.md"), []byte("# Journal\n---\n"), 0644)

	// Compact twice
	Compact(CompactOptions{RepoRoot: tmp, Summary: "First decision"})
	Compact(CompactOptions{RepoRoot: tmp, Summary: "Second decision"})

	data, _ := os.ReadFile(filepath.Join(thothDir, "memory.yaml"))
	content := string(data)
	count := strings.Count(content, "## Session Decisions")
	if count != 1 {
		t.Errorf("expected exactly 1 Session Decisions header, got %d", count)
	}
	if !strings.Contains(content, "First decision") {
		t.Error("expected first decision")
	}
	if !strings.Contains(content, "Second decision") {
		t.Error("expected second decision")
	}
}

func TestCompact_EmptySummary(t *testing.T) {
	tmp := t.TempDir()
	thothDir := filepath.Join(tmp, ".thoth")
	os.MkdirAll(thothDir, 0755)
	os.WriteFile(filepath.Join(thothDir, "memory.yaml"), []byte("project: test\n"), 0644)

	err := Compact(CompactOptions{RepoRoot: tmp, Summary: ""})
	if err == nil {
		t.Error("expected error for empty summary")
	}
}

func TestCompact_MissingThothDir(t *testing.T) {
	err := Compact(CompactOptions{RepoRoot: t.TempDir(), Summary: "test"})
	if err == nil {
		t.Error("expected error for missing .thoth dir")
	}
}

func TestPruneJournal_MaxKeep(t *testing.T) {
	tmp := t.TempDir()
	thothDir := filepath.Join(tmp, ".thoth")
	os.MkdirAll(thothDir, 0755)

	journal := `# Journal
---

## Entry 001 — 2026-01-01 — Old entry

Content 1

---

## Entry 002 — 2026-02-01 — Middle entry

Content 2

---

## Entry 003 — 2026-03-01 — Recent entry

Content 3

---
`
	os.WriteFile(filepath.Join(thothDir, "journal.md"), []byte(journal), 0644)

	removed, err := PruneJournal(PruneOptions{RepoRoot: tmp, MaxKeep: 2})
	if err != nil {
		t.Fatalf("PruneJournal: %v", err)
	}
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	data, _ := os.ReadFile(filepath.Join(thothDir, "journal.md"))
	content := string(data)
	if strings.Contains(content, "Entry 001") {
		t.Error("entry 001 should have been pruned")
	}
	if !strings.Contains(content, "Entry 002") {
		t.Error("entry 002 should be kept")
	}
	if !strings.Contains(content, "Entry 003") {
		t.Error("entry 003 should be kept")
	}
}

func TestPruneJournal_PreservesHeader(t *testing.T) {
	tmp := t.TempDir()
	thothDir := filepath.Join(tmp, ".thoth")
	os.MkdirAll(thothDir, 0755)

	journal := `# Engineering Journal
# Description line
---

## Entry 001 — 2026-01-01 — Only entry

Content

---
`
	os.WriteFile(filepath.Join(thothDir, "journal.md"), []byte(journal), 0644)

	removed, err := PruneJournal(PruneOptions{RepoRoot: tmp, MaxKeep: 1})
	if err != nil {
		t.Fatalf("PruneJournal: %v", err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed (only 1 entry, keep 1), got %d", removed)
	}

	data, _ := os.ReadFile(filepath.Join(thothDir, "journal.md"))
	if !strings.Contains(string(data), "# Engineering Journal") {
		t.Error("header should be preserved")
	}
}

func TestPruneJournal_NoEntries(t *testing.T) {
	tmp := t.TempDir()
	thothDir := filepath.Join(tmp, ".thoth")
	os.MkdirAll(thothDir, 0755)
	os.WriteFile(filepath.Join(thothDir, "journal.md"), []byte("# Journal\n---\n"), 0644)

	removed, err := PruneJournal(PruneOptions{RepoRoot: tmp, MaxKeep: 5})
	if err != nil {
		t.Fatalf("PruneJournal: %v", err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed, got %d", removed)
	}
}
