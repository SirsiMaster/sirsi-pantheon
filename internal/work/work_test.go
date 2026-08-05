package work

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSendCreatesOpenItem(t *testing.T) {
	root := t.TempDir()
	id, err := Send(root, "claude-pantheon", "codex-pantheon", "review canon-sync", "please review the diff")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.Contains(id, "claude-pantheon-codex-pantheon-review-canon-sync") {
		t.Fatalf("id lacks slugged participants/title: %s", id)
	}
	it, err := Get(root, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if it.From != "claude-pantheon" || it.To != "codex-pantheon" {
		t.Fatalf("from/to mismatch: %+v", it)
	}
	if it.Title != "review canon-sync" {
		t.Fatalf("title round-trip lost: %q", it.Title)
	}
	if it.Status != "open" {
		t.Fatalf("expected open, got %q", it.Status)
	}
	if it.Opened == "" {
		t.Fatal("opened timestamp missing")
	}
	if !strings.Contains(it.Instructions, "please review the diff") {
		t.Fatalf("instructions missing: %q", it.Instructions)
	}
}

func TestSendRequiresFromAndTo(t *testing.T) {
	root := t.TempDir()
	if _, err := Send(root, "", "codex", "t", "x"); err == nil {
		t.Fatal("expected error for empty from")
	}
	if _, err := Send(root, "claude", "", "t", "x"); err == nil {
		t.Fatal("expected error for empty to")
	}
}

func TestListInboxFiltersAndSorts(t *testing.T) {
	root := t.TempDir()
	a, _ := Send(root, "claude", "codex", "first", "a")
	b, _ := Send(root, "claude", "codex", "second", "b")
	_, _ = Send(root, "claude", "gemini", "for gemini", "g")

	got, err := ListInbox(root, "codex")
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 items for codex, got %d", len(got))
	}
	if got[0].ID != a || got[1].ID != b {
		t.Fatalf("expected sort by ID asc: %s,%s got %s,%s", a, b, got[0].ID, got[1].ID)
	}

	all, _ := ListInbox(root, "")
	if len(all) != 3 {
		t.Fatalf("ListInbox(\"\") should return all open: got %d", len(all))
	}
}

func TestListInboxMissingDirIsEmpty(t *testing.T) {
	root := t.TempDir() // no items/ subdir created
	got, err := ListInbox(root, "anyone")
	if err != nil {
		t.Fatalf("ListInbox on missing items/ should not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

func TestCloseAttachesResultAndHidesFromInbox(t *testing.T) {
	root := t.TempDir()
	id, _ := Send(root, "claude", "codex", "topic", "do work")
	if err := Close(root, id, "all green — see report"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	it, err := Get(root, id)
	if err != nil {
		t.Fatalf("Get after close: %v", err)
	}
	if it.Status != "closed" {
		t.Fatalf("status not closed: %q", it.Status)
	}
	if it.Closed == "" {
		t.Fatal("closed timestamp missing")
	}
	if !strings.Contains(it.Result, "all green") {
		t.Fatalf("result body not parsed: %q", it.Result)
	}
	if it.Instructions == "" || strings.Contains(it.Instructions, "Result") {
		t.Fatalf("instructions leaked into result section: %q", it.Instructions)
	}

	inbox, _ := ListInbox(root, "codex")
	if len(inbox) != 0 {
		t.Fatalf("closed item should not appear in inbox: got %d", len(inbox))
	}
}

func TestCloseTwiceFails(t *testing.T) {
	root := t.TempDir()
	id, _ := Send(root, "a", "b", "t", "")
	if err := Close(root, id, ""); err != nil {
		t.Fatal(err)
	}
	if err := Close(root, id, ""); err == nil {
		t.Fatal("expected error on second close")
	}
}

func TestCloseWithoutResultLeavesPlaceholder(t *testing.T) {
	root := t.TempDir()
	id, _ := Send(root, "a", "b", "t", "")
	if err := Close(root, id, "   "); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "items", id+".md")
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "(closed without result)") {
		t.Fatalf("missing placeholder: %s", data)
	}
}

func TestFrontmatterEscapesYAMLSensitiveTitles(t *testing.T) {
	// Titles can legitimately contain colons, quotes, or YAML indicators.
	tricky := []string{
		`router-refactor: phase 2`,
		`title with "quotes" and: colon`,
		`- leading dash`,
		`& anchor-like`,
		`* alias-like`,
		`| pipe`,
	}
	root := t.TempDir()
	for _, title := range tricky {
		id, err := Send(root, "claude", "codex", title, "x")
		if err != nil {
			t.Fatalf("Send(%q): %v", title, err)
		}
		it, err := Get(root, id)
		if err != nil {
			t.Fatalf("Get(%q): %v", title, err)
		}
		if it.Title != title {
			t.Fatalf("title round-trip broke for %q → got %q", title, it.Title)
		}
		if it.Status != "open" {
			t.Fatalf("status broken by tricky title %q: %q", title, it.Status)
		}
	}
}

func TestListAllIncludesClosed(t *testing.T) {
	root := t.TempDir()
	openID, _ := Send(root, "a", "b", "open one", "")
	closedID, _ := Send(root, "a", "b", "closed one", "")
	if err := Close(root, closedID, "done"); err != nil {
		t.Fatal(err)
	}
	all, err := ListAll(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
	var sawOpen, sawClosed bool
	for _, it := range all {
		switch it.ID {
		case openID:
			sawOpen = it.Status == "open"
		case closedID:
			sawClosed = it.Status == "closed"
		}
	}
	if !sawOpen || !sawClosed {
		t.Fatalf("ListAll missed an item: openSeen=%v closedSeen=%v", sawOpen, sawClosed)
	}
}

func TestUnquoteYAMLBackcompat(t *testing.T) {
	// Items written by older versions (no quotes) must still parse.
	root := t.TempDir()
	if err := EnsureRoot(root); err != nil {
		t.Fatal(err)
	}
	legacy := `---
from: claude
to: codex
title: bare title
status: open
opened: 2026-05-21T17:39:20Z
---

## Instructions

legacy body
`
	path := filepath.Join(root, "items", "legacy.md")
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	it, err := Get(root, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if it.From != "claude" || it.To != "codex" || it.Title != "bare title" {
		t.Fatalf("legacy parse broke: %+v", it)
	}
}

func TestItemIsBuildable(t *testing.T) {
	cases := []struct {
		typ  string
		want bool
	}{
		{"", true},          // plain item — no type means build-shaped
		{"build", true},     // explicit build
		{"task", true},      // task is build-shaped
		{"decision", false}, // decision has no build artifact
		{"review", false},   // review has no build artifact
		{"proposal", false}, // proposal has no build artifact
		{"DECISION", false}, // case-insensitive
		{"Review", false},   // case-insensitive
	}
	for _, c := range cases {
		it := Item{Type: c.typ}
		if got := it.IsBuildable(); got != c.want {
			t.Errorf("IsBuildable() for type=%q = %v, want %v", c.typ, got, c.want)
		}
	}
}
