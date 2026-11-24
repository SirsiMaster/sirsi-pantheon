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
	root := mustEval(t, t.TempDir())
	proof := filepath.Join(root, "proof.json")
	want := []byte(" {\"status\":\"completed\",\"blockers\":[]}\n\t")
	if err := os.WriteFile(proof, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := captureCompletionProof(root, "proof.json"); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("regular proof capture = %q, %v; want exact bytes %q", got, err, want)
	}
	link := filepath.Join(root, "proof-link.json")
	if err := os.Symlink(proof, link); err != nil {
		t.Fatal(err)
	}
	if got, err := captureCompletionProof(root, "proof-link.json"); err == nil || got != nil {
		t.Fatalf("direct symlink leaf accepted: contents=%q error=%v", got, err)
	}
	if err := os.Symlink(root, filepath.Join(root, "linked-parent")); err != nil {
		t.Fatal(err)
	}
	if got, err := captureCompletionProof(root, "linked-parent/proof.json"); err == nil || got != nil {
		t.Fatal("symlink parent accepted")
	}
}

func TestCaptureCompletionProofRejectsFIFOWithoutWriter(t *testing.T) {
	proof := filepath.Join(mustEval(t, t.TempDir()), "proof.fifo")
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
	contents, err := captureCompletionProof(filepath.Dir(proof), filepath.Base(proof))
	if err == nil || !strings.Contains(err.Error(), "must be a regular file") {
		t.Fatalf("FIFO was not refused at the regular-file check: %v", err)
	}
	if contents != nil {
		t.Fatal("FIFO refusal returned proof contents")
	}
}

func TestProofOpenerRejectsSubstitutedRootAndUnsafeComponents(t *testing.T) {
	base := mustEval(t, t.TempDir())
	root := filepath.Join(base, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTest(t, filepath.Join(root, "proof.json"), []byte(`{}`))
	if err := os.Rename(root, root+"-original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root+"-original", root); err != nil {
		t.Fatal(err)
	}
	if f, err := openCompletionProof(root, "proof.json"); err == nil {
		f.Close()
		t.Fatal("substituted symlink root accepted")
	}
	for _, relative := range []string{"..", "../proof.json", "/proof.json", "x/../proof.json", ".", ""} {
		if f, err := openCompletionProof(root+"-original", relative); err == nil {
			f.Close()
			t.Fatalf("unsafe relative path accepted: %q", relative)
		}
	}
}

func TestValidatorExportExactExclusiveAndNoFollow(t *testing.T) {
	root := mustEval(t, t.TempDir())
	path := filepath.Join(root, "validator.py")
	if err := ExportValidator(path); err != nil {
		t.Fatal(err)
	}
	if got := readTest(t, path); string(got) != productionValidator {
		t.Fatal("export bytes differ from embed")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("export permissions: %v, %v", info, err)
	}
	if err := ExportValidator(path); err == nil {
		t.Fatal("export overwrote existing file")
	}
	link := filepath.Join(root, "symlink.py")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err := ExportValidator(link); err == nil {
		t.Fatal("export followed leaf symlink")
	}
	parent := filepath.Join(root, "linked-parent")
	if err := os.Symlink(root, parent); err != nil {
		t.Fatal(err)
	}
	if err := ExportValidator(filepath.Join(parent, "new.py")); err == nil {
		t.Fatal("export followed parent symlink")
	}
	if _, err := os.Stat(filepath.Join(root, "new.py")); !os.IsNotExist(err) {
		t.Fatal("failed export created a file through symlink")
	}
	if err := ExportValidator(filepath.Join(root, "absent-parent", "new.py")); err == nil {
		t.Fatal("export created parents")
	}
	if err := ExportValidator(""); err == nil {
		t.Fatal("export guessed a destination")
	}
}
