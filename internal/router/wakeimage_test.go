package router

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWakeExecutableReplacementWaitsForConsumerBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sirsi")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	w, err := watchWakeExecutable(path)
	if err != nil {
		t.Fatal(err)
	}
	if w.replaced() {
		t.Fatal("unchanged image reported replaced")
	}
	newPath := path + ".new"
	if err := os.WriteFile(newPath, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(newPath, path); err != nil {
		t.Fatal(err)
	}
	if !w.replaced() {
		t.Fatal("atomic installer replacement not detected")
	}
	if wakeUpgradeReady(w.replaced(), true) {
		t.Fatal("upgrade interrupted active consumer")
	}
	if !wakeUpgradeReady(w.replaced(), false) {
		t.Fatal("idle old generation did not yield")
	}
}

func TestWakeExecutableIncompleteReplacementPreservesRunningImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sirsi")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	w, err := watchWakeExecutable(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".retained"); err != nil {
		t.Fatal(err)
	}
	if w.replaced() {
		t.Fatal("missing target evicted current image")
	}
	if err := os.Symlink(path+".retained", path); err != nil {
		t.Fatal(err)
	}
	if w.replaced() {
		t.Fatal("symlink replacement accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if w.replaced() {
		t.Fatal("non-executable replacement accepted")
	}
}
