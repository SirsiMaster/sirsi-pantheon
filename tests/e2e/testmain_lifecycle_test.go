package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const e2eLifecycleChildEnv = "SIRSI_E2E_TESTMAIN_LIFECYCLE_CHILD"
const e2eLifecycleReceiptEnv = "SIRSI_E2E_TESTMAIN_LIFECYCLE_RECEIPT"

type e2eLifecycleReceipt struct {
	Root     string `json:"root"`
	RanTests bool   `json:"ran_tests"`
}

// Entered only by this test binary's TestMain, before its ordinary CLI build.
// The actual lifecycle helper is exercised with fake build/run callbacks; the
// child still reaches the real os.Exit boundary. No sirsi command is executed.
func runE2ELifecycleChild(mode string) int {
	if mode != "pass" && mode != "test-failure" && mode != "build-failure" && mode != "replacement" && mode != "replacement-pass" {
		fmt.Fprintln(os.Stderr, "unknown e2e lifecycle fixture mode")
		return 99
	}
	var receipt e2eLifecycleReceipt
	writeReceipt := func() {
		data, err := json.Marshal(receipt)
		if err == nil {
			err = os.WriteFile(os.Getenv(e2eLifecycleReceiptEnv), data, 0600)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "fixture receipt failure: %v\n", err)
			os.Exit(98)
		}
	}
	return runE2ETestMain(func(binary string) int {
		receipt.Root = filepath.Dir(binary)
		if err := os.WriteFile(binary, []byte("owned fake binary"), 0600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 97
		}
		writeReceipt()
		if mode == "build-failure" {
			return 7
		}
		return 0
	}, func() int {
		receipt.RanTests = true
		writeReceipt()
		if strings.HasPrefix(mode, "replacement") {
			// Both roots belong to this child's parent fixture. A substitute must
			// not be deleted, and unknown ownership must remain visible debt.
			if err := os.Rename(receipt.Root, receipt.Root+".moved"); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 97
			}
			if err := os.Mkdir(receipt.Root, 0700); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 97
			}
			if err := os.WriteFile(filepath.Join(receipt.Root, "replacement"), []byte("do not remove"), 0600); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 97
			}
		}
		if mode == "test-failure" || mode == "replacement" {
			return 23
		}
		return 0
	}, os.Stderr)
}

func TestE2ETestMainOwnedRootExitCleanup(t *testing.T) {
	for _, tc := range []struct {
		mode string
		code int
	}{
		{"pass", 0}, {"test-failure", 23}, {"build-failure", 7}, {"replacement", 23}, {"replacement-pass", 1},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			owned := t.TempDir()
			receiptPath := filepath.Join(owned, "receipt.json")
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0])
			for _, entry := range os.Environ() {
				key, _, _ := strings.Cut(entry, "=")
				if key != "HOME" && key != "TMPDIR" && key != "TMP" && key != "TEMP" &&
					!strings.HasPrefix(key, "SIRSI_") && !strings.HasPrefix(key, "MAAT_") && !strings.HasPrefix(key, "GIT_") {
					cmd.Env = append(cmd.Env, entry)
				}
			}
			cmd.Env = append(cmd.Env, "HOME="+owned, "TMPDIR="+owned, "TMP="+owned, "TEMP="+owned,
				e2eLifecycleChildEnv+"="+tc.mode, e2eLifecycleReceiptEnv+"="+receiptPath)
			out, err := cmd.CombinedOutput() // waits/joins before output inspection
			if ctx.Err() != nil {
				t.Fatalf("lifecycle child did not finish: %v", ctx.Err())
			}
			code := 0
			if err != nil {
				if exit, ok := err.(*exec.ExitError); ok {
					code = exit.ExitCode()
				} else {
					t.Fatalf("start child: %v", err)
				}
			}
			if code != tc.code {
				t.Fatalf("child exit=%d want=%d: %s", code, tc.code, out)
			}
			data, err := os.ReadFile(receiptPath)
			if err != nil {
				t.Fatal(err)
			}
			var receipt e2eLifecycleReceipt
			if err := json.Unmarshal(data, &receipt); err != nil {
				t.Fatal(err)
			}
			if filepath.Clean(filepath.Dir(receipt.Root)) != filepath.Clean(owned) || !strings.HasPrefix(filepath.Base(receipt.Root), "sirsi-e2e-") {
				t.Fatalf("child root escaped fixture: %q", receipt.Root)
			}
			if receipt.RanTests != (tc.mode != "build-failure") {
				t.Fatalf("build failure ran tests or normal exit skipped them: %+v", receipt)
			}
			if strings.HasPrefix(tc.mode, "replacement") {
				if !strings.Contains(string(out), "e2e cleanup debt") || !strings.Contains(string(out), "identity changed") || !strings.Contains(string(out), receipt.Root) {
					t.Fatalf("substitution did not report exact cleanup debt: %s", out)
				}
				for _, leaf := range []string{filepath.Join(receipt.Root, "replacement"), filepath.Join(receipt.Root+".moved", filepath.Base(testBinary))} {
					if _, err := os.Stat(leaf); err != nil {
						t.Fatalf("cleanup removed substituted/original fixture %q: %v", leaf, err)
					}
				}
				return
			}
			if _, err := os.Lstat(receipt.Root); !os.IsNotExist(err) {
				t.Fatalf("owned binary root survived child exit: %q err=%v output=%s", receipt.Root, err, out)
			}
			if strings.Contains(string(out), "e2e cleanup debt") {
				t.Fatalf("normal cleanup reported debt: %s", out)
			}
		})
	}
}
