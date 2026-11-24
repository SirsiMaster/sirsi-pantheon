package routerstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Compare against the previous glob behavior on ordinary directories,
// symlinked lanes, hidden names, and entries whose names end in .json.
func TestRelaySingleScanMatchesPreviousGlob(t *testing.T) {
	root := t.TempDir()
	for _, lane := range []string{"lane-a", ".hidden", "lane-empty"} {
		if err := os.MkdirAll(filepath.Join(root, lane, "req"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "lane-a", "req", "a.json"), []byte("inert"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".hidden", "req", "nested.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "lane-a"), filepath.Join(root, "lane-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "lane-req-link"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "lane-a", "req"), filepath.Join(root, "lane-req-link", "req")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "root-file"), []byte("inert"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(root, "*", "req", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	for _, f := range files {
		set[filepath.Base(filepath.Dir(filepath.Dir(f)))] = true
	}
	want := make([]string, 0, len(set))
	for n := range set {
		want = append(want, n)
	}
	sort.Strings(want)
	rl := &Relay{Spool: root, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rl.discoverTracked(ctx, nil)
	rl.workers.Wait()
	got := make([]string, 0, len(rl.lanes))
	for n := range rl.lanes {
		got = append(got, n)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lane discovery changed: got %v want %v", got, want)
	}
}

func TestRelaySingleScanPartialReadsAndPermissionWarningOnce(t *testing.T) {
	root := t.TempDir()
	req := filepath.Join(root, "lane", "req")
	if err := os.MkdirAll(req, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(req, "fixture.json"), []byte("inert"), 0600); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	var reqReads int
	rl := &Relay{Spool: root, Log: slog.New(slog.NewTextHandler(&logs, nil))}
	rl.readDirFn = func(path string) ([]os.DirEntry, error) {
		entries, err := os.ReadDir(path)
		if path == req {
			reqReads++
			if err != nil {
				return entries, err
			}
			return entries, fs.ErrPermission
		}
		return entries, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rl.discoverTracked(ctx, nil)
	rl.discoverTracked(ctx, nil)
	rl.workers.Wait()
	if reqReads != 2 {
		t.Fatalf("request directory read %d times for two passes", reqReads)
	}
	if _, ok := rl.lanes["lane"]; !ok {
		t.Fatal("partial entries were discarded")
	}
	if n := strings.Count(logs.String(), "lane directory not readable"); n != 1 {
		t.Fatalf("permission warning count %d, want one", n)
	}
	if !errors.Is(fs.ErrPermission, os.ErrPermission) {
		t.Fatal("fixture requires standard permission sentinel")
	}
}

func TestRelaySingleScanUnreadableDirectoryWarnsOnce(t *testing.T) {
	root := t.TempDir()
	req := filepath.Join(root, "lane", "req")
	if err := os.MkdirAll(req, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(req, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(req, 0700) })
	if _, err := os.ReadDir(req); !errors.Is(err, fs.ErrPermission) {
		t.Skip("this identity can read the mode-000 fixture")
	}
	var logs bytes.Buffer
	rl := &Relay{Spool: root, Log: slog.New(slog.NewTextHandler(&logs, nil))}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rl.discoverTracked(ctx, nil)
	rl.discoverTracked(ctx, nil)
	rl.workers.Wait()
	if n := strings.Count(logs.String(), "lane directory not readable"); n != 1 {
		t.Fatalf("permission warning count %d, want one", n)
	}
	if len(rl.lanes) != 0 {
		t.Fatal("unreadable fixture was discovered")
	}
}

func TestRelaySingleScanTreatsSpoolPathLiterally(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool[1]")
	if err := os.MkdirAll(filepath.Join(root, "lane", "req"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lane", "req", "fixture.json"), []byte("inert"), 0600); err != nil {
		t.Fatal(err)
	}
	rl := &Relay{Spool: root, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rl.discoverTracked(ctx, nil)
	rl.workers.Wait()
	if _, ok := rl.lanes["lane"]; !ok {
		t.Fatal("literal spool path was treated as glob syntax")
	}
}
