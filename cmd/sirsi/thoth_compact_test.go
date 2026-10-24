package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunThothCompactRetentionUsesGo(t *testing.T) {
	// No production repo, journal, router state, or external writer is used.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TMPDIR", t.TempDir())
	oldSummary, oldAge, oldKeep := thothCompactSummary, thothCompactMaxAge, thothCompactMaxKeep
	t.Cleanup(func() {
		thothCompactSummary, thothCompactMaxAge, thothCompactMaxKeep = oldSummary, oldAge, oldKeep
	})
	for _, tc := range []struct {
		name        string
		args        []string
		wantEntries int
	}{
		{"positive-age", []string{"--max-age=1"}, 1},
		{"explicit-zero", []string{"--max-age=0", "--max-keep=0"}, 26},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Chdir(root)
			for _, dir := range []string{".git", ".thoth", "bin"} {
				if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
					t.Fatal(err)
				}
			}
			marker := filepath.Join(root, "external-writer-invoked")
			t.Setenv("THOTH_TEST_DELEGATE_MARKER", marker)
			bin := filepath.Join(root, "bin")
			t.Setenv("PATH", bin)
			delegate := filepath.Join(bin, "thoth-compact")
			if err := os.WriteFile(delegate, []byte("#!/bin/sh\nprintf invoked > \"$THOTH_TEST_DELEGATE_MARKER\"\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nexit 97\n"), 0700); err != nil {
				t.Fatal(err)
			}
			if found, err := exec.LookPath("thoth-compact"); err != nil || found != delegate {
				t.Fatalf("fake external writer must be available: path=%q err=%v", found, err)
			}
			if err := os.WriteFile(filepath.Join(root, ".thoth", "memory.yaml"), []byte("project: fixture\n"), 0600); err != nil {
				t.Fatal(err)
			}
			original := "# Owned CLI journal\n\n"
			for i := 1; i <= 25; i++ {
				original += fmt.Sprintf("## Entry %03d — 2000-01-01 — Old fixture\n\nold-body-%03d\n\n---\n", i, i)
			}
			path := filepath.Join(root, ".thoth", "journal.md")
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := &cobra.Command{Use: "compact", RunE: runThothCompact, SilenceUsage: true, SilenceErrors: true}
			cmd.Flags().StringVarP(&thothCompactSummary, "summary", "s", "", "")
			cmd.Flags().IntVar(&thothCompactMaxAge, "max-age", 0, "")
			cmd.Flags().IntVar(&thothCompactMaxKeep, "max-keep", 0, "")
			cmd.SetArgs(append([]string{"--summary", "Owned CLI decision"}, tc.args...))
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("external writer must never run: marker stat=%v", err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got := string(data)
			if n := strings.Count(got, "## Entry "); n != tc.wantEntries {
				t.Fatalf("entries=%d, want %d", n, tc.wantEntries)
			}
			if !strings.HasPrefix(got, "# Owned CLI journal\n") || !strings.Contains(got, "Owned CLI decision") || !strings.Contains(got, "Entry 026") {
				t.Fatal("Go compact must preserve header and append today's new entry")
			}
			if tc.name == "positive-age" && strings.Contains(got, "old-body-") {
				t.Fatal("Go age retention must remove old entries")
			}
			if tc.name == "explicit-zero" && !strings.HasPrefix(got, original) {
				t.Fatal("explicit zero must preserve every original journal byte")
			}
		})
	}
}

func TestThothCompactExplicitZeroRetentionFlags(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want bool
	}{
		{"omitted", nil, false},
		{"age-zero", []string{"--max-age=0"}, true},
		{"keep-zero", []string{"--max-keep", "0"}, true},
		{"both-zero", []string{"--max-age", "0", "--max-keep", "0"}, true},
		{"positive-age", []string{"--max-age=7", "--max-keep=0"}, false},
		{"positive-keep", []string{"--max-age=0", "--max-keep=3"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "compact"}
			var age, keep int
			cmd.Flags().IntVar(&age, "max-age", 0, "")
			cmd.Flags().IntVar(&keep, "max-keep", 0, "")
			if err := cmd.ParseFlags(tc.args); err != nil {
				t.Fatal(err)
			}
			opts := thothCompactOptions(cmd, "fixture-root", "fixture-summary", age, keep)
			if opts.NoDefaultPrune != tc.want || opts.MaxAge != age || opts.MaxKeep != keep || opts.RepoRoot != "fixture-root" || opts.Summary != "fixture-summary" {
				t.Fatalf("options=%+v, want NoDefaultPrune=%v and unchanged inputs", opts, tc.want)
			}
		})
	}
}
