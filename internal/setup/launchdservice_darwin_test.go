//go:build darwin

package setup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/sys/unix"
)

func TestServiceArtifactRetainedSnapshot(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "service.plist")
	body := []byte(`<?xml version="1.0"?><plist version="1.0"><dict><key>Label</key><string>fixture</string><key>Program</key><string>/usr/bin/true</string></dict></plist>`)
	if err = os.WriteFile(p, body, 0600); err != nil {
		t.Fatal(err)
	}
	got, digest, err := serviceArtifact(p, true)
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(body)
	if string(got) != string(body) || digest != hex.EncodeToString(want[:]) {
		t.Fatal("snapshot/hash mismatch")
	}
	decoded, err := serviceReadInput(context.Background(), got, "/usr/bin/plutil", "-convert", "json", "-o", "-", "-")
	if err != nil || len(decoded) == 0 {
		t.Fatalf("retained plist parse: %s %v", decoded, err)
	}
	link := filepath.Join(dir, "link.plist")
	if err = os.Symlink(p, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err = serviceArtifact(link, true); err == nil {
		t.Fatal("symlink leaf accepted")
	}
	parentLink := filepath.Join(dir, "parent-link")
	if err = os.Symlink(dir, parentLink); err != nil {
		t.Fatal(err)
	}
	if _, _, err = serviceArtifact(filepath.Join(parentLink, "service.plist"), true); err == nil {
		t.Fatal("symlink parent accepted")
	}
	fifo := filepath.Join(dir, "fifo")
	if err = unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = serviceArtifact(fifo, true); err == nil {
		t.Fatal("nonregular artifact accepted")
	}
}

func TestServiceScopeRejectsUnrelatedLabels(t *testing.T) {
	for _, label := range []string{"ai.sirsi.router", "ai.sirsi.router.wake.sirsi-software-admin", "system/ai.sirsi.mercury-monitor", "../ai.sirsi.apollo.observer", "ai.sirsi.*"} {
		if _, _, err := serviceLocation(label); err == nil {
			t.Fatalf("unrelated scope accepted: %s", label)
		}
	}
}

func TestRouterSurfaceLocationIsExactNativeUserService(t *testing.T) {
	const label = "ai.sirsi.router.surface"
	if os.Getuid() == 0 || os.Getuid() != os.Geteuid() {
		if _, _, err := serviceLocation(label); err == nil {
			t.Fatal("gateway accepted privileged or mismatched owner")
		}
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	target, plist, err := serviceLocation(label)
	if err != nil || target != "gui/"+strconv.Itoa(os.Getuid())+"/"+label || plist != filepath.Join(home, "Library", "LaunchAgents", label+".plist") {
		t.Fatalf("exact user gateway location = %q %q %v", target, plist, err)
	}
	for _, other := range []string{"ai.sirsi.router.surface.other", "system/" + label, "../" + label, label + "*", "ai.sirsi.router.wake.ra"} {
		if _, _, err := serviceLocation(other); err == nil {
			t.Fatalf("gateway scope expanded to %q", other)
		}
	}
}

func TestServiceLockRefusesConcurrentApplyAndUnsafeLock(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plist := filepath.Join(dir, "fixture.plist")
	if err = withServiceLock(plist, func() error {
		called := false
		err := withServiceLock(plist, func() error { called = true; return nil })
		if err == nil || called {
			t.Fatal("second maintainer entered selected-service lock")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = withServiceLock(plist, func() error { return nil }); err != nil {
		t.Fatal("released lock remained held", err)
	}
	lock := filepath.Join(dir, ".fixture.plist.lifecycle-lock")
	if err = os.Chmod(lock, 0666); err != nil {
		t.Fatal(err)
	}
	called := false
	if err = withServiceLock(plist, func() error { called = true; return nil }); err == nil || called {
		t.Fatal("unsafe lock accepted")
	}
}
