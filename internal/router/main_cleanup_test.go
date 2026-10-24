package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
)

const routerMainChildEnv = "SIRSI_ROUTER_TESTMAIN_CLEANUP_CHILD"
const routerMainRootsPrefix = "ROUTER_TESTMAIN_ROOTS "

// Runs only through the exact subprocess selector below. TestMain still runs
// its real setup, m.Run and cleanup; no environment flag bypasses the suite.
func TestRouterMainOwnedTempChild(t *testing.T) {
	mode := os.Getenv(routerMainChildEnv)
	if mode == "" {
		t.Skip("inert subprocess fixture only")
	}
	roots := []string{filepath.Dir(os.Getenv("SIRSI_ROUTER_DB")), getDispatchDir()}
	for _, root := range roots {
		rel, err := filepath.Rel(os.TempDir(), root)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			t.Fatalf("fixture root is not inside private TMPDIR: %q", root)
		}
		if err := os.WriteFile(filepath.Join(root, "owned-marker"), []byte("inert"), 0600); err != nil {
			t.Fatal(err)
		}
		if mode == "substitute" {
			if err := os.Rename(root, root+".retained"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "replacement-marker"), []byte("inert replacement"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	b, err := json.Marshal(roots)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%s%s\n", routerMainRootsPrefix, b)
	switch mode {
	case "pass", "substitute":
	case "fail":
		t.Error("inert intentional failure")
	default:
		t.Fatalf("unknown fixture mode %q", mode)
	}
}

func TestRouterMainOwnedTempExitCleanup(t *testing.T) {
	for _, mode := range []string{"pass", "fail", "substitute"} {
		t.Run(mode, func(t *testing.T) {
			private := t.TempDir()
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestRouterMainOwnedTempChild$")
			// Canonicalize only these fixture variables. Keep the real TestMain's
			// router isolation, reaper/grant stubs and dispatch override intact.
			for _, value := range os.Environ() {
				name, _, _ := strings.Cut(value, "=")
				if name != "TMPDIR" && name != routerMainChildEnv {
					cmd.Env = append(cmd.Env, value)
				}
			}
			cmd.Env = append(cmd.Env, "TMPDIR="+private, routerMainChildEnv+"="+mode)
			out, runErr := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("owned child watchdog: %v", ctx.Err())
			}
			exit := 0
			if runErr != nil {
				var exited *exec.ExitError
				if !errors.As(runErr, &exited) {
					t.Fatalf("start actual test binary: %v", runErr)
				}
				exit = exited.ExitCode()
			}
			wantExit := 0
			if mode != "pass" {
				wantExit = 1
			}
			if exit != wantExit {
				t.Fatalf("child exit=%d, want %d; output=%s", exit, wantExit, out)
			}
			var roots []string
			for _, line := range bytes.Split(out, []byte("\n")) {
				if bytes.HasPrefix(line, []byte(routerMainRootsPrefix)) {
					if err := json.Unmarshal(line[len(routerMainRootsPrefix):], &roots); err != nil {
						t.Fatal(err)
					}
				}
			}
			if len(roots) != 2 || roots[0] == roots[1] {
				t.Fatalf("actual DB/dispatch root metadata absent: %s", out)
			}
			for _, root := range roots {
				if mode == "substitute" {
					for _, marker := range []string{filepath.Join(root, "replacement-marker"), filepath.Join(root+".retained", "owned-marker")} {
						if _, err := os.Stat(marker); err != nil {
							t.Errorf("substituted and retained roots must both survive: %s: %v", marker, err)
						}
					}
				} else if _, err := os.Lstat(root); !os.IsNotExist(err) {
					t.Errorf("actual TestMain root still present after exit: %s: %v", root, err)
				}
			}
			if mode == "substitute" && bytes.Count(out, []byte("router TestMain cleanup debt")) != 2 {
				t.Errorf("both refused removals must report debt: %s", out)
			}
		})
	}
}

func preserveRouterMainSeams(t *testing.T) {
	t.Helper()
	for _, name := range []string{routercfg.StoreWakeEnv, "SIRSI_ROUTER_URL", "SIRSI_ROUTER_TOKEN", "SIRSI_ROUTER_DB"} {
		t.Setenv(name, os.Getenv(name))
	}
	previous := getDispatchDir()
	t.Cleanup(func() { SetDispatchDir(previous) })
}

func TestRouterMainSecondSetupFailureCleansFirstOwnedRoot(t *testing.T) {
	preserveRouterMainSeams(t)
	private := t.TempDir()
	var first string
	calls := 0
	code := runRouterTestsWith(func() int { t.Error("callback ran after setup failure"); return 0 },
		func(_, pattern string) (string, error) {
			calls++
			if calls == 2 {
				return "", errors.New("inert second mkdir failure")
			}
			var err error
			first, err = os.MkdirTemp(private, pattern)
			return first, err
		}, os.RemoveAll)
	if code != 1 || first == "" {
		t.Fatalf("setup result=%d first=%q", code, first)
	}
	if _, err := os.Lstat(first); !os.IsNotExist(err) {
		t.Fatalf("first root leaked on second mkdir failure: %v", err)
	}
}

func TestRouterMainCleanupFailureChangesSuccessButPreservesFailure(t *testing.T) {
	for _, original := range []int{0, 7} {
		t.Run(fmt.Sprint(original), func(t *testing.T) {
			preserveRouterMainSeams(t)
			private := t.TempDir()
			removals := 0
			code := runRouterTestsWith(func() int { return original },
				func(_, pattern string) (string, error) { return os.MkdirTemp(private, pattern) },
				func(string) error { removals++; return errors.New("inert removal failure") })
			want := original
			if want == 0 {
				want = 1
			}
			if code != want || removals != 2 {
				t.Fatalf("code=%d want=%d cleanup attempts=%d want=2", code, want, removals)
			}
		})
	}
}
