package dashboard

import (
	"os/exec"
	"testing"
)

// The offline fallback must treat only a verified 200 "ok" as live: an opaque,
// redirected, failed or login-page answer is unknown, and the first healthy
// console wins even when an earlier one is broken.
func TestConsoleProbeJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	out, err := exec.Command(node, "ui/consoleprobe.test.js").CombinedOutput()
	if err != nil {
		t.Fatalf("consoleprobe.test.js: %v\n%s", err, out)
	}
}
