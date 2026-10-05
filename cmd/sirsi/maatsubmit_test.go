package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalRepo(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"already canonical", "sirsimaster/sirsi-mercury", "sirsimaster/sirsi-mercury", false},
		{"mixed case folds to canonical", "SirsiMaster/SIRSI-MERCURY", "sirsimaster/sirsi-mercury", false},
		{"missing slash", "sirsi-mercury", "", true},
		{"empty owner", "/sirsi-mercury", "", true},
		{"empty name", "sirsimaster/", "", true},
		{"extra slash", "sirsimaster/sirsi/mercury", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := canonicalRepo(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", c.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("canonicalRepo(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestCheckSubmitPolicy(t *testing.T) {
	cases := []struct {
		name          string
		repo          string
		requester     string
		wantDeterm    string
		wantWhyHasAny string
	}{
		{"policy match", "sirsimaster/sirsi-mercury", "mercury", "grant", "matches repo policy"},
		{"policy mismatch", "sirsimaster/sirsi-mercury", "claude-pantheon", "refuse", "not in repo policy"},
		{"photon policy match", "sirsimaster/sirsi-photon", "mercury", "grant", "matches repo policy"},
		{"mercury repo policy match", "sirsimaster/sirsi-mercury", "mercury", "grant", "matches repo policy"},
		{"mercury refuses other lanes", "sirsimaster/sirsi-mercury", "claude-pantheon", "refuse", "not in repo policy"},
		{"unlisted repo", "sirsimaster/sirsi-pantheon", "claude-pantheon", "grant", "no policy defined"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, why := checkSubmitPolicy(c.repo, c.requester)
			if got != c.wantDeterm {
				t.Errorf("determination = %q, want %q", got, c.wantDeterm)
			}
			if !strings.Contains(why, c.wantWhyHasAny) {
				t.Errorf("why = %q, want substring %q", why, c.wantWhyHasAny)
			}
		})
	}
}

// caseFoldedRepoCannotBypassPolicy is the end-to-end version of
// codex-pantheon finding 2 (item 20260930-231226): mixed-case input for a
// protected repo must land on the same policy row as the canonical form.
func TestCaseFoldedRepoCannotBypassPolicy(t *testing.T) {
	repo, err := canonicalRepo("sirsiMASTER/SIRSI-mercury")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	determ, _ := checkSubmitPolicy(repo, "claude-pantheon")
	if determ != "refuse" {
		t.Fatalf("case-folded protected repo should still refuse a non-mercury requester, got %q", determ)
	}
}

func TestResolveSubmitRequester(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Run("no session id refuses", func(t *testing.T) {
		t.Setenv("CLAUDE_CODE_SESSION_ID", "")
		if _, err := resolveSubmitRequester(); err == nil {
			t.Fatal("expected error with no session id, got nil")
		}
	})

	t.Run("unregistered session refuses", func(t *testing.T) {
		t.Setenv("CLAUDE_CODE_SESSION_ID", "sid-unregistered")
		if _, err := resolveSubmitRequester(); err == nil {
			t.Fatal("expected error for unregistered session, got nil")
		}
	})

	writeMarker := func(t *testing.T, sid, agent string) {
		t.Helper()
		dir := filepath.Join(home, ".claude", "run", "agent-by-session")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, sid), []byte(agent+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("registered session resolves", func(t *testing.T) {
		const sid = "sid-registered"
		t.Setenv("CLAUDE_CODE_SESSION_ID", sid)
		writeMarker(t, sid, "mercury")
		got, err := resolveSubmitRequester()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "mercury" {
			t.Errorf("requester = %q, want %q", got, "mercury")
		}
	})

	// codex-pantheon finding 3 (item 20260930-231226): a marker naming an
	// agent id that isn't in the declared registry must refuse, not be
	// trusted as an arbitrary self-declared string.
	t.Run("marker naming unregistered agent id refuses", func(t *testing.T) {
		const sid = "sid-forged"
		t.Setenv("CLAUDE_CODE_SESSION_ID", sid)
		writeMarker(t, sid, "totally-not-a-real-agent")
		if _, err := resolveSubmitRequester(); err == nil {
			t.Fatal("expected error for marker naming an unregistered agent id, got nil")
		}
	})
}

// TestSubmitCommandExitCode is codex-pantheon finding 1 (item
// 20260930-231226): the maatJSON branch returned before the refusal
// os.Exit(97), so a denied request encoded as --json exited 0. This drives
// the actual command as a subprocess (os.Exit can't be caught in-process)
// in both text and --json mode, for both a granted and a refused caller.
func TestSubmitCommandExitCode(t *testing.T) {
	if os.Getenv("MAAT_SUBMIT_HELPER") == "1" {
		runSubmitHelper()
		return
	}

	// Manual MkdirTemp + best-effort cleanup, not t.TempDir(): each subtest
	// below spawns this binary as a real OS subprocess, and on this host that
	// occasionally leaves module-cache-adjacent files transiently locked
	// (Gatekeeper/mds on a freshly-exec'd binary), which turns t.TempDir()'s
	// strict RemoveAll-must-succeed cleanup into a spurious fatal failure
	// unrelated to the assertions below.
	home, err := os.MkdirTemp("", "maat-submit-exit-code-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(home) }()
	const sid = "sid-exit-code-test"
	dir := filepath.Join(home, ".claude", "run", "agent-by-session")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name         string
		agent        string
		repo         string
		json         bool
		wantExitCode int
	}{
		{"text grant", "mercury", "sirsimaster/sirsi-mercury", false, 0},
		{"text refuse", "claude-pantheon", "sirsimaster/sirsi-mercury", false, admissionRefusedExit},
		{"json grant", "mercury", "sirsimaster/sirsi-mercury", true, 0},
		{"json refuse", "claude-pantheon", "sirsimaster/sirsi-mercury", true, admissionRefusedExit},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(dir, sid), []byte(c.agent+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			args := []string{"-test.run=TestSubmitCommandExitCode", "-test.v"}
			cmd := exec.Command(os.Args[0], args...)
			cmd.Env = append(os.Environ(),
				"MAAT_SUBMIT_HELPER=1",
				"HOME="+home,
				"CLAUDE_CODE_SESSION_ID="+sid,
				"MAAT_SUBMIT_HELPER_REPO="+c.repo,
				"MAAT_SUBMIT_HELPER_JSON="+boolToStr(c.json),
				"SIRSI_MAAT_DECISIONS_PATH="+filepath.Join(home, "decisions.jsonl"),
			)
			out, err := cmd.CombinedOutput()
			exitCode := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					t.Fatalf("failed to run helper process: %v, output: %s", err, out)
				}
			}
			if exitCode != c.wantExitCode {
				t.Errorf("exit code = %d, want %d (output: %s)", exitCode, c.wantExitCode, out)
			}
		})
	}
}

// TestSubmitCommandLedgerFailureVisibility is codex-pantheon finding (router
// item 20261001-041706): the identity-refusal branch discarded the ledger-
// append error while the policy branch correctly propagated it, so "every
// grant/refusal is written" was false specifically for unregistered-session
// refusals. A denied operation stays denied either way; what must change is
// that the ledger failure becomes VISIBLE rather than silently coexisting
// with a clean-looking refusal. Reproduces codex's exact technique: point
// SIRSI_MAAT_DECISIONS_PATH at an existing DIRECTORY so the ledger write
// fails with a real I/O error, not a mock.
func TestSubmitCommandLedgerFailureVisibility(t *testing.T) {
	home, err := os.MkdirTemp("", "maat-submit-ledger-failure-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(home) }()
	badLedgerPath := filepath.Join(home, "ledger-is-a-directory")
	if err := os.MkdirAll(badLedgerPath, 0o755); err != nil {
		t.Fatal(err)
	}

	runHelper := func(t *testing.T, env ...string) (exitCode int, out string) {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=TestSubmitCommandExitCode", "-test.v")
		cmd.Env = append(append([]string{}, os.Environ()...), env...)
		cmd.Env = append(cmd.Env, "MAAT_SUBMIT_HELPER=1", "SIRSI_MAAT_DECISIONS_PATH="+badLedgerPath)
		raw, runErr := cmd.CombinedOutput()
		exitCode = 0
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				t.Fatalf("failed to run helper process: %v, output: %s", runErr, raw)
			}
		}
		return exitCode, string(raw)
	}

	t.Run("identity refusal with unwritable ledger stays refused and names the problem", func(t *testing.T) {
		const sid = "sid-unregistered-ledger-fail"
		exitCode, out := runHelper(t,
			"HOME="+home,
			"CLAUDE_CODE_SESSION_ID="+sid, // no marker file written → unregistered
			"MAAT_SUBMIT_HELPER_REPO=sirsimaster/sirsi-mercury",
			"MAAT_SUBMIT_HELPER_JSON=0",
		)
		if exitCode != admissionRefusedExit {
			t.Errorf("exit code = %d, want %d (a denied operation must stay denied even when the ledger write also fails): %s", exitCode, admissionRefusedExit, out)
		}
		if !strings.Contains(out, "ledger") {
			t.Errorf("output does not name the ledger failure: %s", out)
		}
	})

	t.Run("identity refusal with unwritable ledger, JSON mode, names the problem", func(t *testing.T) {
		const sid = "sid-unregistered-ledger-fail-json"
		exitCode, out := runHelper(t,
			"HOME="+home,
			"CLAUDE_CODE_SESSION_ID="+sid,
			"MAAT_SUBMIT_HELPER_REPO=sirsimaster/sirsi-mercury",
			"MAAT_SUBMIT_HELPER_JSON=1",
		)
		if exitCode != admissionRefusedExit {
			t.Errorf("exit code = %d, want %d: %s", exitCode, admissionRefusedExit, out)
		}
		if !strings.Contains(out, "ledger_error") {
			t.Errorf("JSON output does not surface ledger_error: %s", out)
		}
	})

	t.Run("policy grant with unwritable ledger refuses to report success and names the problem", func(t *testing.T) {
		const sid = "sid-mercury-ledger-fail"
		dir := filepath.Join(home, ".claude", "run", "agent-by-session")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, sid), []byte("mercury\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		exitCode, out := runHelper(t,
			"HOME="+home,
			"CLAUDE_CODE_SESSION_ID="+sid,
			"MAAT_SUBMIT_HELPER_REPO=sirsimaster/sirsi-mercury",
			"MAAT_SUBMIT_HELPER_JSON=0",
		)
		if exitCode == 0 {
			t.Errorf("exit code = 0, want nonzero — a ledger append failure must never be reported as a clean grant: %s", out)
		}
		if !strings.Contains(out, "append decision ledger") {
			t.Errorf("output does not name the ledger failure: %s", out)
		}
	})
}

func boolToStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// runSubmitHelper re-executes maatSubmitCmd in a subprocess so the real
// os.Exit(97) path can be observed from outside.
func runSubmitHelper() {
	submitRepo = os.Getenv("MAAT_SUBMIT_HELPER_REPO")
	submitKind = "release"
	submitRef = "v1.0.0"
	maatJSON = os.Getenv("MAAT_SUBMIT_HELPER_JSON") == "1"
	if err := maatSubmitCmd.RunE(maatSubmitCmd, nil); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
