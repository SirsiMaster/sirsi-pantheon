//go:build darwin || linux

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestMailboxRequestDecodeClosedAndBounded(t *testing.T) {
	valid := `{"action":"ack-notice","manifest_id":"approved","notice_id":"exact"}`
	if req, digest, err := decodeMailboxRequest(strings.NewReader(valid)); err != nil || req.NoticeID != "exact" || len(digest) != 64 {
		t.Fatalf("valid bounded request: %+v %s %v", req, digest, err)
	}
	for _, input := range []string{valid + `{}`, `{"action":"ack-notice","actor":"owner"}`, strings.Repeat(" ", (128<<10)+1), `{`} {
		if _, _, err := decodeMailboxRequest(strings.NewReader(input)); err == nil {
			t.Fatal("unknown-field, malformed, trailing or oversized request accepted")
		}
	}
	var out bytes.Buffer
	cmd := newMailboxReconcileCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--request", "relative.json"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("relative request path accepted")
	}
}

func TestMailboxRequestPlanIsReadOnlyAndRejectsSpecialFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "request.json")
	if err := os.WriteFile(path, []byte(`{"action":"ack-notice","manifest_id":"approved","notice_id":"exact"}`), 0600); err != nil {
		t.Fatal(err)
	}
	// An invalid service configuration must not matter for a read-only plan.
	t.Setenv("SIRSI_ROUTER_URL", "https://invalid.example.test")
	t.Setenv("SIRSI_ROUTER_TOKEN", "")
	var out bytes.Buffer
	cmd := newMailboxReconcileCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--request", path})
	if err := cmd.Execute(); err != nil || !strings.Contains(out.String(), `"Apply":false`) {
		t.Fatalf("default plan contacted service or failed: %s %v", out.String(), err)
	}
	link, fifo := filepath.Join(root, "link.json"), filepath.Join(root, "fifo.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	for _, special := range []string{link, fifo, root} {
		cmd := newMailboxReconcileCmd()
		cmd.SetArgs([]string{"--request", special})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("special request file accepted: %s", special)
		}
	}
}
