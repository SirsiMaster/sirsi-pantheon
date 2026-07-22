package rules

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestMain hardens this package's git-fixture tests against the #99 env-leak
// class. The Ma'at pre-push gate runs `go test` WITH GIT_DIR/GIT_WORK_TREE
// exported by git itself; git honors those OVER `-C dir`, so an inherited
// pointer silently retargets every fixture command at the REAL repo. That
// exact leak committed initGitRepo's fixture ("init", one README) onto a
// live feature branch twice on 2026-07-22 (and hit internal/osiris the same
// way on 2026-07-09). Two defenses:
//  1. scrub every GIT_* variable once, before any test runs (process-wide,
//     parallel-safe — t.Setenv cannot be used with t.Parallel tests);
//  2. record the host repo's HEAD before the suite and fail loudly if any
//     test mutated it — the guard nexus asked for, so an escape can never
//     again pass silently.
func TestMain(m *testing.M) {
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_") {
			key, _, _ := strings.Cut(kv, "=")
			os.Unsetenv(key)
		}
	}
	before := hostHEAD()
	code := m.Run()
	if after := hostHEAD(); before != "" && after != before {
		fmt.Fprintf(os.Stderr,
			"FATAL: rules test suite mutated the enclosing repo's HEAD (%s -> %s) — a git fixture escaped its tempdir (#99 class)\n",
			before, after)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

// hostHEAD returns the enclosing (host) repo's HEAD, or "" when the tests run
// outside any git checkout (CI sandboxes) — the guard then no-ops.
func hostHEAD() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitTestEnv returns the process environment with every GIT_* variable
// removed and only the fixture identity re-added. Helpers must use this
// instead of os.Environ() so a leaked GIT_DIR can never retarget a fixture
// command, even if a future caller bypasses TestMain's scrub.
func gitTestEnv() []string {
	env := make([]string, 0, len(os.Environ())+4)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	return append(env,
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
	)
}
