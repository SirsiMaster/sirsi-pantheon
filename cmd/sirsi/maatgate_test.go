package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/trustboundary"
)

func TestMaatGateRequestModeBoundaries(t *testing.T) {
	// Validate the selection in memory. None of these cases runs a gate,
	// queries a repo, installs a hook, or launches a subprocess.
	tests := []struct {
		name string
		mode gateRequestModes
		want string
	}{
		{name: "default-survey"},
		{name: "report-only", mode: gateRequestModes{report: true}},
		{name: "enforcement", mode: gateRequestModes{prePush: true}},
		{name: "explicit-range", mode: gateRequestModes{base: "base", head: "head", rangeSet: true}},
		{name: "lint-survey", mode: gateRequestModes{all: true, lintOnly: true}},
		{name: "lint-only-explicit-range", mode: gateRequestModes{base: "base", head: "head", rangeSet: true, lintOnly: true}},
		{name: "lint-only-without-scope", mode: gateRequestModes{lintOnly: true}, want: "requires --all or both --base and --head"},
		{name: "lint-only-root-is-not-a-scope", mode: gateRequestModes{root: "/fixture", lintOnly: true}, want: "requires --all or both --base and --head"},
		{name: "informational-conflict", mode: gateRequestModes{report: true, prePush: true}, want: "informational"},
		{name: "informational-repo-conflict", mode: gateRequestModes{report: true, root: "/fixture"}, want: "informational"},
		{name: "pushed-ref-range-conflict", mode: gateRequestModes{prePush: true, base: "base", head: "head"}, want: "full check"},
		{name: "pushed-ref-lint-conflict", mode: gateRequestModes{prePush: true, lintOnly: true}, want: "full check"},
		{name: "pushed-ref-survey-conflict", mode: gateRequestModes{prePush: true, all: true}, want: "full check"},
		{name: "empty-explicit-range", mode: gateRequestModes{rangeSet: true}, want: "no survey fallback"},
		{name: "partial-range", mode: gateRequestModes{rangeSet: true, head: "head"}, want: "no survey fallback"},
		{name: "blank-range-endpoint", mode: gateRequestModes{rangeSet: true, base: " ", head: "head"}, want: "no survey fallback"},
		{name: "new-branch-base-without-pre-push", mode: gateRequestModes{newBranchBase: "abc"}, want: "only with --pre-push"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.mode.validate()
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("mode validation error = %v; want %q", err, tc.want)
			}
		})
	}
}

func TestMaatGateCLIExplicitRangeLintOnlyUsesCommittedBlobs(t *testing.T) {
	root := t.TempDir()
	hooks := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmdArgs := []string{"-c", "core.hooksPath=" + hooks, "-c", "commit.gpgsign=false",
			"-c", "user.name=Ma'at test", "-c", "user.email=maat-test@example.invalid", "-C", root}
		cmd := exec.Command("/usr/bin/git", append(cmdArgs, args...)...)
		cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + root, "GIT_CONFIG_NOSYSTEM=1",
			"GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0"}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "main", "--template="+t.TempDir())
	if err := os.WriteFile(filepath.Join(root, "clean.go"), []byte("package fixture\nfunc clean() int { return 1 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "clean.go")
	git("commit", "-q", "-m", "fixture base")
	base := git("rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(root, "added.go"), []byte("package fixture\nfunc added() int { return 2 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "added.go")
	git("commit", "-q", "-m", "fixture clean range")
	cleanHead := git("rev-parse", "HEAD")

	// A dirty-tree defect must not affect the selected committed head.
	if err := os.WriteFile(filepath.Join(root, "added.go"), []byte("package fixture\nimport \"strconv\"\nfunc added(s string) []int { n, _ := strconv.Atoi(s); return make([]int, n) }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runMaatGateJSON(t, func() {
		gateRoot, gateBase, gateHead, gateLintOnly = root, base, cleanHead, true
	})
	if err != nil {
		t.Fatalf("explicit lint-only clean committed range refused: %v\n%s", err, out)
	}
	var cleanResult trustboundary.Result
	if err := json.Unmarshal([]byte(out), &cleanResult); err != nil || cleanResult.Scope != "lint-only-range" ||
		!cleanResult.OK || !cleanResult.Complete || len(cleanResult.Steps) != 1 || cleanResult.Steps[0].Name != "trust-boundary-lint" {
		t.Fatalf("clean exact-range result = %q parsed=%+v err=%v", out, cleanResult, err)
	}

	// Commit the defect and prove the same command rejects the exact bad head.
	git("add", "added.go")
	git("commit", "-q", "-m", "fixture lint finding")
	badHead := git("rev-parse", "HEAD")
	out, err = runMaatGateJSON(t, func() {
		gateRoot, gateBase, gateHead, gateLintOnly = root, base, badHead, true
	})
	var badResult trustboundary.Result
	if err == nil || json.Unmarshal([]byte(out), &badResult) != nil || badResult.Scope != "lint-only-range" ||
		badResult.OK || len(badResult.Findings) != 1 || badResult.Findings[0].Rule != "A" ||
		len(badResult.Steps) != 1 || badResult.Steps[0].Name != "trust-boundary-lint" {
		t.Fatalf("bad exact-range result = %q parsed=%+v err=%v", out, badResult, err)
	}
}

func TestMaatGateDoesNotExposeHookInstall(t *testing.T) {
	if maatGateCmd.Flags().Lookup("install") != nil {
		t.Fatal("gate must not expose an install mode that claims to configure hooks")
	}
}

func TestMaatGateEnforcementRejectsIncompleteResult(t *testing.T) {
	incomplete := trustboundary.Result{OK: true, Complete: false}
	if !maatGateFailed(incomplete, true) {
		t.Fatal("pre-push enforcement must refuse a result with skipped required checks")
	}
	if maatGateFailed(incomplete, false) {
		t.Fatal("informational survey may report no executed failure while remaining incomplete")
	}
}

func runMaatGateJSON(t *testing.T, setup func()) (string, error) {
	t.Helper()
	old := struct {
		root, base, head, newBranchBase string
		prePush, all, lintOnly, report  bool
		json                            bool
	}{gateRoot, gateBase, gateHead, gateNewBranchBase, gatePrePush, gateAll, gateLintOnly, gateReport, JsonOutput}
	t.Cleanup(func() {
		gateRoot, gateBase, gateHead, gateNewBranchBase = old.root, old.base, old.head, old.newBranchBase
		gatePrePush, gateAll, gateLintOnly, gateReport, JsonOutput = old.prePush, old.all, old.lintOnly, old.report, old.json
	})
	gateRoot, gateBase, gateHead, gateNewBranchBase = "", "", "", ""
	gatePrePush, gateAll, gateLintOnly, gateReport, JsonOutput = false, false, false, false, true
	setup()
	var stdout, stderr strings.Builder
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(testMaatGateInput))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := maatGateCmd.RunE(cmd, nil)
	return stdout.String(), err
}

const testMaatGateInput = "refs/tags/v1 0123456789abcdef0123456789abcdef01234567 refs/tags/v1 0000000000000000000000000000000000000000\n"

func TestMaatGateJSONSuccessfulNoopAndReport(t *testing.T) {
	out, err := runMaatGateJSON(t, func() { gatePrePush = true })
	if err != nil {
		t.Fatal(err)
	}
	var result trustboundary.Result
	if err := json.Unmarshal([]byte(out), &result); err != nil || result.Scope != "pre-push" || !result.OK || !result.Complete {
		t.Fatalf("tag-only JSON result = %q; parsed=%+v err=%v", out, result, err)
	}
	out, err = runMaatGateJSON(t, func() { gateReport = true })
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Scope  string `json:"scope"`
		Report string `json:"report"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil || report.Scope != "report" || report.Report == "" {
		t.Fatalf("report JSON = %q; parsed=%+v err=%v", out, report, err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &keys); err != nil {
		t.Fatal(err)
	}
	if _, ok := keys["ok"]; ok {
		t.Fatal("informational worksheet must not carry gate-verdict fields")
	}
	if _, ok := keys["complete"]; ok {
		t.Fatal("informational worksheet must not claim gate completeness")
	}
}

func TestMaatGateEmptyPrePushInputRefuses(t *testing.T) {
	oldRoot, oldPrePush, oldJSON := gateRoot, gatePrePush, JsonOutput
	gateRoot, gatePrePush, JsonOutput = t.TempDir(), true, false
	t.Cleanup(func() { gateRoot, gatePrePush, JsonOutput = oldRoot, oldPrePush, oldJSON })
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(" \n\t\n"))
	if err := maatGateCmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "no ref records") {
		t.Fatalf("whitespace-only pre-push input must fail closed, got %v", err)
	}
}
