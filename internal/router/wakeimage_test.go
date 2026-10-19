package router

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func fixtureWakeIdentity(t *testing.T, path string) wakeImageIdentity {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := wakeFileIdentity(info)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func TestWakeExecutableReplacementWaitsForConsumerBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sirsi")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	w, err := watchWakeExecutable(path, fixtureWakeIdentity(t, path))
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
	w, err := watchWakeExecutable(path, fixtureWakeIdentity(t, path))
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

func TestWakeReplacementBeforeWatcherCaptureUsesLoadedIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sirsi")
	if err := os.WriteFile(path, []byte("loaded-old"), 0o755); err != nil {
		t.Fatal(err)
	}
	loaded := fixtureWakeIdentity(t, path)
	if err := os.Rename(path, path+".retained"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("installed-new"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The installer won BEFORE construction, while the process still has old
	// executable code mapped. A pathname-only baseline would miss this upgrade.
	w, err := watchWakeExecutable(path, loaded)
	if err != nil {
		t.Fatal(err)
	}
	if !w.replaced() {
		t.Fatal("pre-capture replacement mistaken for loaded generation")
	}
	if wakeUpgradeReady(w.replaced(), true) {
		t.Fatal("pre-capture replacement interrupted work")
	}
	if !wakeUpgradeReady(w.replaced(), false) {
		t.Fatal("old idle controller did not yield")
	}
}

func TestNativeWakeImageMatchesActualTestExecutable(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := nativeWakeImageIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if expected := fixtureWakeIdentity(t, path); loaded != expected {
		t.Fatalf("kernel loaded image %+v does not match actual executable %+v", loaded, expected)
	}
}

func TestNativeLoadedImageDetectsPreCaptureAtomicReplacement(t *testing.T) {
	if root := os.Getenv("SIRSI_WAKE_IMAGE_FIXTURE"); root != "" {
		// Parent waits for this actual executable process, replaces its name,
		// then allows observation. Kernel mapping must still name the old inode.
		if err := os.WriteFile(filepath.Join(root, "ready"), []byte("ready"), 0o600); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			if _, err := os.Stat(filepath.Join(root, "replaced")); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("parent replacement not observed")
			}
			time.Sleep(5 * time.Millisecond)
		}
		loaded, err := nativeWakeImageIdentity()
		if err != nil {
			t.Fatal(err)
		}
		watcher, err := watchWakeExecutable(os.Args[0], loaded)
		if err != nil {
			t.Fatal(err)
		}
		if !watcher.replaced() {
			t.Fatal("kernel identity was confused with replaced executable name")
		}
		return
	}
	root := t.TempDir()
	path := filepath.Join(root, "sirsi-fixture")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, path, "-test.run=^TestNativeLoadedImageDetectsPreCaptureAtomicReplacement$")
	child.Env = append(os.Environ(), "SIRSI_WAKE_IMAGE_FIXTURE="+root)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if child.ProcessState == nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture child did not become ready")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := os.Rename(path, path+".retained"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "replaced"), []byte("replaced"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err != nil {
		t.Fatalf("real loaded-image replacement control: %v", err)
	}
}
