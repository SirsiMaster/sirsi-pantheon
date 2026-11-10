package routerstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type pager []Item

func (p pager) ListPage(_ context.Context, after string, limit int) ([]Item, error) {
	var out []Item
	for _, it := range p {
		if it.ID > after && len(out) < limit {
			out = append(out, it)
		}
	}
	return out, nil
}

func TestExportMarkdownPagedWritesEveryItemLocally(t *testing.T) {
	items := pager{{ID: "a", From: "x", To: "y", Title: "1", Status: "open"}, {ID: "b", From: "x", To: "y", Title: "2", Status: "open"}, {ID: "c", From: "x", To: "y", Title: "3", Status: "closed"}}
	dir := t.TempDir()
	r, err := ExportMarkdownPaged(context.Background(), items, dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	if r.Items != 3 || r.Pages != 2 || r.LastID != "c" || r.Snapshot {
		t.Fatalf("report %+v", r)
	}
	for _, id := range []string{"a", "b", "c"} {
		b, err := os.ReadFile(filepath.Join(dir, id+".md"))
		if err != nil || string(b) != renderMarkdown(items[map[string]int{"a": 0, "b": 1, "c": 2}[id]]) {
			t.Fatalf("%s: %v", id, err)
		}
	}
}

type stuck struct{}

func (stuck) ListPage(context.Context, string, int) ([]Item, error) {
	return []Item{{ID: "a"}, {ID: "a"}}, nil
}

type failing struct{}

func (failing) ListPage(context.Context, string, int) ([]Item, error) {
	return nil, errors.New("refused")
}

func TestExportMarkdownPagedStopsOnStuckCursorAndErrors(t *testing.T) {
	if _, err := ExportMarkdownPaged(context.Background(), stuck{}, t.TempDir(), 2); err == nil {
		t.Fatal("a page that does not advance the cursor was accepted")
	}
	if r, err := ExportMarkdownPaged(context.Background(), failing{}, t.TempDir(), 2); err == nil || r.Items != 0 {
		t.Fatalf("server refusal: %+v %v", r, err)
	}
}
