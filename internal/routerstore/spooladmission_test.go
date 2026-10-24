//go:build !windows

package routerstore

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSpoolAdmissionCrossProcess(t *testing.T) {
	if os.Getenv("SIRSI_ADMISSION_CHILD") == "1" {
		r, e := os.OpenRoot(os.Getenv("SIRSI_ADMISSION_TEST_ROOT"))
		if e != nil {
			os.Exit(3)
		}
		ctx, c := context.WithTimeout(context.Background(), 40*time.Millisecond)
		defer c()
		f, e := lockSpoolAdmission(ctx, r, false)
		if f != nil {
			f.Close()
		}
		if !errors.Is(e, context.DeadlineExceeded) {
			os.Exit(4)
		}
		return
	}
	dir := t.TempDir()
	err := WithSpoolQuiescence(context.Background(), dir, func() error {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSpoolAdmissionCrossProcess$")
		cmd.Env = append(os.Environ(), "SIRSI_ADMISSION_CHILD=1", "SIRSI_ADMISSION_TEST_ROOT="+dir)
		return cmd.Run()
	})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := os.OpenRoot(dir)
	defer r.Close()
	ctx, c := context.WithTimeout(context.Background(), time.Second)
	defer c()
	f, e := lockSpoolAdmission(ctx, r, false)
	if e != nil {
		t.Fatal("resume", e)
	}
	f.Close()
}
func TestSpoolAdmissionWaitsInflightAndCancellation(t *testing.T) {
	dir := t.TempDir()
	r, _ := os.OpenRoot(dir)
	defer r.Close()
	active, e := lockSpoolAdmission(context.Background(), r, false)
	if e != nil {
		t.Fatal(e)
	}
	ctx, c := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer c()
	ran := false
	e = WithSpoolQuiescence(ctx, dir, func() error { ran = true; return nil })
	if !errors.Is(e, context.DeadlineExceeded) || ran {
		t.Fatal("did not wait for active request", e)
	}
	active.Close()
	if e = WithSpoolQuiescence(context.Background(), dir, func() error { return nil }); e != nil {
		t.Fatal(e)
	}
}
func TestSpoolAdmissionPausedCallNeverPublishes(t *testing.T) {
	dir := t.TempDir()
	e := WithSpoolQuiescence(context.Background(), dir, func() error {
		ctx, c := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer c()
		req, _ := http.NewRequestWithContext(ctx, "POST", "http://spool/v1/call/Inbox", nil)
		_, e := newSpoolTransport(dir, "own").RoundTrip(req)
		if !errors.Is(e, context.DeadlineExceeded) {
			t.Fatal(e)
		}
		if _, e := os.Stat(filepath.Join(dir, "own")); !os.IsNotExist(e) {
			t.Fatal("paused call published lane", e)
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}
func TestSpoolAdmissionRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(t.TempDir(), "alias")
	if e := os.Symlink(dir, link); e != nil {
		t.Fatal(e)
	}
	ran := false
	if e := WithSpoolQuiescence(context.Background(), link, func() error { ran = true; return nil }); e == nil || ran {
		t.Fatal("accepted linked root")
	}
}

func TestSpoolAdmissionTrustRefusalPrecedesMaintenanceWait(t *testing.T) {
	dir := t.TempDir()
	setOwnedSpoolFixtureGroupAndMode(t, dir, 0o700|os.ModeSetgid)
	if err := WithSpoolQuiescence(context.Background(), dir, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, "POST", "http://spool/v1/call/Inbox", nil)
		if err != nil {
			return err
		}
		_, err = newSpoolTransport(dir, "own").RoundTrip(req)
		if err == nil || !strings.Contains(err.Error(), spoolTrustGroupEnv+"=") {
			t.Fatalf("expected actionable trust refusal before admission wait, got %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "own")); !os.IsNotExist(err) {
			t.Fatalf("refused client published lane: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSpoolAdmissionOperationFailureResumes(t *testing.T) {
	dir := t.TempDir()
	sentinel := errors.New("guard refusal")
	if e := WithSpoolQuiescence(context.Background(), dir, func() error { return sentinel }); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	if e := WithSpoolQuiescence(context.Background(), dir, func() error { return nil }); e != nil {
		t.Fatal("failed operation left admission closed", e)
	}
}
func TestSpoolAdmissionActualInflightSettlesBeforeExclusive(t *testing.T) {
	dir := t.TempDir()
	// Publication is atomic: a staged .json.tmp is not a relay request. Keep a
	// lexically earlier staged leaf present so observing it cannot pass by luck.
	reqDir := filepath.Join(dir, "own", "req")
	if err := os.MkdirAll(reqDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reqDir, "000-unpublished.json.tmp"), []byte("staged"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, c := context.WithTimeout(context.Background(), time.Second)
	defer c()
	req, _ := http.NewRequestWithContext(ctx, "POST", "http://spool/v1/call/Inbox", nil)
	done := make(chan error, 1)
	go func() { _, e := newSpoolTransport(dir, "own").RoundTrip(req); done <- e }()
	var id string
	for id == "" && ctx.Err() == nil {
		entries, _ := filepath.Glob(filepath.Join(reqDir, "*.json"))
		if len(entries) > 0 {
			id = filepath.Base(entries[0])
			break
		}
		time.Sleep(time.Millisecond)
	}
	if id == "" {
		t.Fatal("request not published")
	}
	wait, stop := context.WithTimeout(ctx, 30*time.Millisecond)
	e := WithSpoolQuiescence(wait, dir, func() error { t.Error("entered while published call outstanding"); return nil })
	stop()
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "own", "res", id), []byte(`{"status":200,"body":"e30="}`), 0600); e != nil {
		t.Fatal(e)
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if e := WithSpoolQuiescence(ctx, dir, func() error { return nil }); e != nil {
		t.Fatal("settled call blocked exclusive", e)
	}
}
