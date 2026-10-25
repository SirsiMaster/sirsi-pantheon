//go:build !windows

package routerstore

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/servicedrain"
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

func TestSpoolAdmissionPendingMaintenanceWaitDoesNotPublishAndCanResume(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, servicedrain.MaintenanceMarkerName)
	if err := os.WriteFile(marker, []byte("retained transaction fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	short := newSpoolTransport(dir, "own")
	short.wait = 30 * time.Millisecond
	req, err := http.NewRequest(http.MethodPost, "http://spool/v1/call/Inbox", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = short.RoundTrip(req)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "request not published") {
		t.Fatalf("pending maintenance must produce a known-not-published timeout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "own")); !os.IsNotExist(err) {
		t.Fatalf("timed-out pending request created a lane: %v", err)
	}
	tr := newSpoolTransport(dir, "own")
	tr.wait = 2 * time.Second

	result := make(chan error, 1)
	go func() {
		response, callErr := tr.RoundTrip(req)
		if callErr == nil {
			_, callErr = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
		result <- callErr
	}()
	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, "own")); !os.IsNotExist(err) {
			t.Fatalf("request was published before retained marker cleared: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(dir, "own")); !os.IsNotExist(err) {
		t.Fatalf("pending request created a lane: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	var requests []string
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		requests, _ = filepath.Glob(filepath.Join(dir, "own", "req", "*.json"))
		if len(requests) == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if len(requests) != 1 {
		t.Fatal("waiting request was not published after the marker cleared")
	}
	if err := writeAtomic(filepath.Join(dir, "own", "res", filepath.Base(requests[0])), spoolResponse{
		Status: http.StatusOK, Body: []byte(`{"result":[]}`),
	}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatalf("call did not resume after retained hold cleared: %v", err)
	}
}

func TestSpoolAdmissionAndResponseShareOneCallBudget(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, servicedrain.MaintenanceMarkerName)
	if err := os.WriteFile(marker, []byte("retained transaction fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	tr := newSpoolTransport(dir, "budget-lane")
	tr.wait = 500 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startedBody := make(chan struct{})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://spool/v1/call/Inbox", &signalBody{read: startedBody})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, callErr := tr.RoundTrip(req) // no caller deadline: transport owns one shared budget
		result <- callErr
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Error("RoundTrip did not finish during fixture cleanup")
		}
	})
	select {
	case <-startedBody: // RoundTrip created its budget and began reading this request.
	case <-time.After(time.Second):
		t.Fatal("RoundTrip did not begin")
	}
	started := time.Now()
	time.Sleep(180 * time.Millisecond)
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	publicationDeadline := started.Add(350 * time.Millisecond)
	var published []string
	for time.Now().Before(publicationDeadline) {
		published, _ = filepath.Glob(filepath.Join(dir, "budget-lane", "req", "*.json"))
		if len(published) == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if len(published) != 1 {
		t.Fatal("budget regression was not exercised: request never reached published response-wait state")
	}
	remaining := time.Until(started.Add(tr.wait + 100*time.Millisecond))
	if remaining <= 0 {
		t.Fatal("request was not observed before the original call budget")
	}
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "nothing was sent") {
			t.Fatalf("withheld published request did not retain the no-response classification: %v", err)
		}
		if elapsed := time.Since(started); elapsed > tr.wait+100*time.Millisecond {
			t.Fatalf("admission time was reset for response waiting: total call took %s", elapsed)
		}
	case <-time.After(remaining):
		t.Fatal("published request did not honor the original shared call deadline")
	}
	if pending, _ := filepath.Glob(filepath.Join(dir, "budget-lane", "req", "*.json")); len(pending) != 0 {
		t.Fatalf("request remained pending after the bounded call: %v", pending)
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
