//go:build darwin || linux

package completionproof

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

const completionProofFIFOChildEnv = "SIRSI_TEST_COMPLETION_PROOF_FIFO"

func TestCaptureCompletionProofRegularFileAndSymlinkLeaf(t *testing.T) {
	root := t.TempDir()
	proof := filepath.Join(root, "proof.json")
	want := []byte(" {\"status\":\"completed\",\"blockers\":[]}\n\t")
	if err := os.WriteFile(proof, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := captureCompletionProof(proof); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("regular proof capture = %q, %v; want exact bytes %q", got, err, want)
	}
	link := filepath.Join(root, "proof-link.json")
	if err := os.Symlink(proof, link); err != nil {
		t.Fatal(err)
	}
	if got, err := captureCompletionProof(link); err == nil || got != nil {
		t.Fatalf("direct symlink leaf accepted: contents=%q error=%v", got, err)
	}
	// Caller-contained aliases remain supported through the existing resolver;
	// the opener only refuses a symlink still present at the resolved leaf.
	_, resolved, err := resolveProof(root, link)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := captureCompletionProof(resolved); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("resolved caller proof capture = %q, %v; want exact bytes %q", got, err, want)
	}
}

func TestCaptureCompletionProofRejectsFIFOWithoutWriter(t *testing.T) {
	proof := filepath.Join(t.TempDir(), "proof.fifo")
	if err := unix.Mkfifo(proof, 0o600); err != nil {
		t.Fatal(err)
	}
	// Isolate the blocking-open regression in a child that the watchdog kills
	// and joins, rather than leaving a blocked goroutine in the test process.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCaptureCompletionProofFIFOChild$")
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key != completionProofFIFOChildEnv {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, completionProofFIFOChildEnv+"="+proof)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("completion proof capture blocked on FIFO without a writer; child terminated: %s", output)
	}
	if err != nil {
		t.Fatalf("FIFO refusal child: %v\n%s", err, output)
	}
}

func TestCaptureCompletionProofFIFOChild(t *testing.T) {
	proof := os.Getenv(completionProofFIFOChildEnv)
	if proof == "" {
		return
	}
	contents, err := captureCompletionProof(proof)
	if err == nil || !strings.Contains(err.Error(), "must be a regular file") {
		t.Fatalf("FIFO was not refused at the regular-file check: %v", err)
	}
	if contents != nil {
		t.Fatal("FIFO refusal returned proof contents")
	}
}
