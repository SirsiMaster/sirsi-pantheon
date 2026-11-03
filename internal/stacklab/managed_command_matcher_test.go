package stacklab

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Source-policy coverage only: this never installs or invokes a hook.
func TestManagedMatcherCoversCommandTools(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "stacklab", "managed-requirements.toml"))
	if err != nil {
		t.Fatal(err)
	}
	// Inspect the actual PreToolUse blocks, not unrelated text in the policy.
	var matchers []*regexp.Regexp
	for _, block := range strings.Split(string(raw), "[[hooks.PreToolUse]]")[1:] {
		var matcher string
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "matcher = ") {
				matcher = strings.Trim(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "matcher = ")), "\"")
			}
		}
		if !strings.Contains(block, `command = "/etc/codex/hooks/sirsi-display-power-guard"`) {
			continue
		}
		re, err := regexp.Compile(matcher)
		if err != nil || matcher == "" {
			t.Fatalf("invalid matcher %q: %v", matcher, err)
		}
		matchers = append(matchers, re)
	}
	for _, name := range []string{"Bash", "exec_command", "functions.exec_command", "unified_exec", "shell"} {
		covered := false
		for _, re := range matchers {
			covered = covered || re.MatchString(name)
		}
		if !covered {
			t.Errorf("command tool %q bypasses managed guard", name)
		}
	}
	// Negative control: the legacy matcher cannot cover non-Bash command tools.
	legacy := regexp.MustCompile("^Bash$")
	if !legacy.MatchString("Bash") {
		t.Fatal("legacy control must cover Bash")
	}
	for _, name := range []string{"exec_command", "functions.exec_command", "unified_exec", "shell"} {
		if legacy.MatchString(name) {
			t.Fatalf("invalid legacy matcher control: %s", name)
		}
	}
}
