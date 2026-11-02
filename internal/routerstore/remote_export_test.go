package routerstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRemoteExportMarkdownIsNotServed: a bulk export over the wire would make
// the service create the caller's directory on the service's own filesystem.
// The service must refuse the method outright and write nothing.
func TestRemoteExportMarkdownIsNotServed(t *testing.T) {
	_, rs := remoteHarness(t)
	if _, err := rs.Send("claude-a", "claude-b", "hello", "proposal", "body"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	// The harness backend shares this machine's filesystem, so a served call
	// would create dir — exactly the server-side write being refused.
	dir := filepath.Join(t.TempDir(), "caller-local", "items")
	n, err := rs.ExportMarkdown(dir)
	if err == nil || !strings.Contains(err.Error(), "no such store method") {
		t.Fatalf("remote ExportMarkdown = (%d, %v), want a not-served refusal", n, err)
	}
	if _, serr := os.Stat(dir); !errors.Is(serr, os.ErrNotExist) {
		t.Fatalf("service touched the caller-named dir %q (stat err=%v)", dir, serr)
	}
}
