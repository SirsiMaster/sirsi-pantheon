package agentguard

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDisplayPowerOrdinaryShellForms(t *testing.T) {
	for _, tc := range []struct {
		command string
		denied  bool
	}{
		{"pmset displaysleepnow", true},
		{"/usr/bin/pmset sleepnow", true},
		{"pmset -a displaysleep 1", true},
		{"pmset lock", true},
		{"env pmset displaysleepnow", true},
		{"/usr/bin/env -i X=1 pmset displaysleepnow", true},
		{"env --unset EXAMPLE command -p pmset displaysleepnow", true},
		{"X=1 Y=2 pmset displaysleepnow", true},
		{"command -- /usr/bin/pmset displaysleepnow", true},
		{"sudo -u root /usr/bin/pmset displaysleepnow", true},
		{"exec pmset displaysleepnow", true},
		{"exec -a harmless /usr/bin/pmset displaysleepnow", true},
		{"exec -a harmless -- /usr/bin/pmset displaysleepnow", true},
		{"exec -a harmless printf harmless", false},
		{"sh -c 'printf harmless;' 'pmset displaysleepnow'", false},
		{"zsh -lc 'pmset displaysleepnow'", true},
		{"echo harmless; /usr/bin/pmset displaysleepnow", true},
		{"env X=1 printf '%s' 'pmset displaysleepnow'", false},
		{"command -v pmset", false},
		{"command -V pmset", false},
		{"env X=1 command pmset -g", false},
		{"X=1 pmset -g", false},
		{"# pmset displaysleepnow", false},
	} {
		t.Run(tc.command, func(t *testing.T) {
			if got := IsDirectDisplayPowerShell(tc.command); got != tc.denied {
				t.Fatalf("denied=%v want=%v", got, tc.denied)
			}
		})
	}
}

func TestDisplayPowerParseBudget(t *testing.T) {
	for _, depth := range []int{1, maxDisplayPowerParseDepth - 3, maxDisplayPowerParseDepth + 1, 8192} {
		command := strings.Repeat("$(", depth) + "printf harmless" + strings.Repeat(")", depth)
		want := depth >= maxDisplayPowerParseDepth-1
		if got := IsDirectDisplayPowerShell(command); got != want {
			t.Fatalf("substitution depth=%d denied=%v want=%v", depth, got, want)
		}
		if got := IsDirectDisplayPowerCommand([]string{"sh", "-c", command}); depth > maxDisplayPowerParseDepth && !got {
			t.Fatalf("argv wrapper reset substitution budget at depth=%d", depth)
		}
	}
	// A wrapper must not reset the budget at the shell-code boundary. Unquoted
	// trailing operands are positional arguments, not another nested program.
	if !isDirectDisplayPowerInvocation([]string{"sh", "-c", "printf harmless"}, maxDisplayPowerParseDepth-1) {
		t.Fatal("shell wrapper reset the budget")
	}
}

func TestDisplayPowerShellCommandOperand(t *testing.T) {
	// Pure argv fixtures: neither code nor positional arguments are executed.
	for _, shell := range []string{"sh", "bash", "zsh"} {
		if IsDirectDisplayPowerCommand([]string{shell, "-c", "printf harmless;", "pmset displaysleepnow"}) {
			t.Fatalf("%s positional argument treated as code", shell)
		}
		if !IsDirectDisplayPowerCommand([]string{shell, "-c", "pmset displaysleepnow", "harmless"}) {
			t.Fatalf("%s actual command operand missed", shell)
		}
	}
}

func TestShellCommandStringOptionSelection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		argv     []string
		want     string
		selected bool
	}{
		{"long_option_then_c", []string{"bash", "--norc", "-c", "printf harmless"}, "printf harmless", true},
		{"bundled_login_c", []string{"bash", "-lc", "printf harmless", "argument"}, "printf harmless", true},
		{"bundled_interactive_c", []string{"zsh", "-ic", "printf harmless"}, "printf harmless", true},
		{"plus_flag_before_c", []string{"bash", "+e", "-c", "printf harmless"}, "printf harmless", true},
		{"plus_bundle_before_c", []string{"bash", "+eH", "-c", "printf harmless"}, "printf harmless", true},
		{"short_options_with_values", []string{"bash", "-o", "pipefail", "-O", "extglob", "-c", "printf harmless"}, "printf harmless", true},
		{"plus_short_options_with_values", []string{"bash", "+o", "errexit", "+O", "extglob", "-c", "printf harmless"}, "printf harmless", true},
		{"plus_short_options_attached_values", []string{"bash", "+oerrexit", "+Oextglob", "-c", "printf harmless"}, "printf harmless", true},
		{"bash_omitted_optional_name_before_plus_option", []string{"bash", "-o", "+e", "-c", "printf harmless"}, "printf harmless", true},
		{"bash_optional_o_name_omitted_minus", []string{"bash", "-o", "-c", "printf harmless"}, "printf harmless", true},
		{"bash_optional_o_name_omitted_plus", []string{"bash", "+o", "-c", "printf harmless"}, "printf harmless", true},
		{"bash_optional_O_name_omitted_minus", []string{"bash", "-O", "-c", "printf harmless"}, "printf harmless", true},
		{"bash_optional_O_name_omitted_plus", []string{"bash", "+O", "-c", "printf harmless"}, "printf harmless", true},
		{"bash_long_options_with_values", []string{"bash", "--rcfile", "one.rc", "--init-file", "two.rc", "-c", "printf harmless"}, "printf harmless", true},
		{"first_c_operand_only", []string{"bash", "-c", "printf harmless", "-c", "later"}, "printf harmless", true},
		{"zsh_upper_O_minus", []string{"zsh", "-O", "-c", "printf harmless"}, "printf harmless", true},
		{"zsh_upper_O_plus", []string{"zsh", "+O", "-c", "printf harmless"}, "printf harmless", true},
		{"zsh_upper_O_bundle", []string{"zsh", "-Oc", "printf harmless"}, "printf harmless", true},
		{"zsh_lower_o_minus_name", []string{"zsh", "-o", "shwordsplit", "-c", "printf harmless"}, "printf harmless", true},
		{"zsh_lower_o_plus_name", []string{"zsh", "+o", "shwordsplit", "-c", "printf harmless"}, "printf harmless", true},
		{"zsh_lower_o_attached_name", []string{"zsh", "-xoshwordsplit", "-c", "printf harmless"}, "printf harmless", true},
		{"sh_required_o_name", []string{"sh", "-o", "errexit", "-c", "printf harmless"}, "printf harmless", true},
		{"zsh_emulate_zsh_mode", []string{"zsh", "--emulate", "zsh", "-c", "printf harmless"}, "printf harmless", true},
		{"zsh_emulate_sh_mode", []string{"zsh", "--emulate", "sh", "-c", "printf harmless"}, "printf harmless", true},
		{"zsh_emulate_sh_with_upper_O", []string{"zsh", "--emulate", "sh", "-O", "-c", "printf harmless"}, "printf harmless", true},
		{"zsh_emulate_sh_with_lower_o", []string{"zsh", "--emulate", "sh", "-o", "shwordsplit", "-c", "printf harmless"}, "printf harmless", true},
		{"zsh_emulate_ksh_mode", []string{"zsh", "--emulate", "ksh", "-c", "printf harmless"}, "printf harmless", true},
		{"zsh_emulate_csh_mode", []string{"zsh", "--emulate", "csh", "-c", "printf harmless"}, "printf harmless", true},
		{"long_option_is_not_c", []string{"bash", "--norc", "printf harmless", "-c"}, "", false},
		{"end_of_options", []string{"bash", "--", "-c", "printf harmless"}, "", false},
		{"script_operand_boundary", []string{"bash", "script.sh", "-c", "printf harmless"}, "", false},
		{"option_value_before_script", []string{"bash", "-o", "pipefail", "script.sh", "-c", "printf harmless"}, "", false},
		{"missing_c_operand", []string{"bash", "--norc", "-c"}, "", false},
		{"missing_zsh_emulate_mode", []string{"zsh", "--emulate"}, "", false},
		{"sh_required_o_name_consumes_next_option", []string{"sh", "-o", "-c", "printf harmless"}, "", false},
		{"plus_c_does_not_select", []string{"bash", "+c", "printf harmless"}, "", false},
		{"plus_option_script_boundary", []string{"bash", "+e", "script.sh", "-c", "printf harmless"}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, selected := shellCommandString(filepath.Base(tc.argv[0]), tc.argv)
			if got != tc.want || selected != tc.selected {
				t.Fatalf("shellCommandString(%q) = %q, %v; want %q, %v", tc.argv, got, selected, tc.want, tc.selected)
			}
			if isDirectDisplayPowerInvocation(tc.argv, 0) {
				t.Fatalf("harmless printf control was denied: %q", tc.argv)
			}
		})
	}
}

func TestShellInvocationsPreserveEmptyQuotedArgument(t *testing.T) {
	for _, tc := range []struct {
		command string
		want    [][]string
	}{
		{`sh -c '' 'printf harmless'`, [][]string{{"sh", "-c", "", "printf harmless"}}},
		{`sh -c 'printf harmless' '' 'printf harmless'`, [][]string{{"sh", "-c", "printf harmless", "", "printf harmless"}}},
		{`printf '%s' "$(printf harmless)"`, [][]string{{"printf", "harmless"}, {"printf", "%s", ""}}},
		{`printf '%s' '$(printf harmless)'`, [][]string{{"printf", "%s", "$(printf harmless)"}}},
		{`printf '%s' "` + "`printf harmless`" + `"`, [][]string{{"printf", "harmless"}, {"printf", "%s", ""}}},
	} {
		got, complete := shellInvocations(tc.command, 0)
		if !complete || !equalInvocations(got, tc.want) {
			t.Errorf("shellInvocations(%q) = %#v, %v; want %#v, true", tc.command, got, complete, tc.want)
		}
	}
}

func equalInvocations(got, want [][]string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if len(got[i]) != len(want[i]) {
			return false
		}
		for j := range got[i] {
			if got[i][j] != want[i][j] {
				return false
			}
		}
	}
	return true
}

// This fixture's ONLY executable is the owned harmless file below. Removing
// the production denial for a red control can only write the owned marker.
func TestSafeRunCannotForceDirectDisplayPowerRoute(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	fake := filepath.Join(root, "pmset")
	marker := filepath.Join(root, "invoked")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nprintf 'called\\n' > \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, force := range []bool{false, true} {
		result, err := SafeRun(context.Background(), RunOptions{
			Command:        []string{fake, marker, "displaysleepnow"},
			Platform:       healthyPlatform(),
			LoadProvider:   func() (float64, float64, error) { return 1, 1, nil },
			HealthProvider: healthyDoctor,
			IgnoreChecks:   []string{"desktop-custody"},
			Force:          force,
		})
		if err == nil || result == nil || result.ExitCode != 126 || result.Report.Verdict != VerdictBlock {
			t.Fatalf("force=%v denial failed: result=%#v err=%v", force, result, err)
		}
		found := false
		for _, finding := range result.Report.Findings {
			found = found || finding.Check == "desktop-custody" && finding.Severity == string(VerdictBlock)
		}
		if !found {
			t.Fatalf("force=%v missing command safety finding", force)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("force=%v fake actuator invoked or marker unreadable: %v", force, err)
		}
	}
	// Positive execution control proves the fake is real/executable. Its -g is
	// merely an argument to our script, never a native query or display action.
	result, err := SafeRun(context.Background(), RunOptions{
		Command:        []string{fake, marker, "-g"},
		Platform:       healthyPlatform(),
		LoadProvider:   func() (float64, float64, error) { return 1, 1, nil },
		HealthProvider: healthyDoctor,
	})
	if err != nil || result == nil || result.ExitCode != 0 {
		t.Fatalf("harmless fake control failed: result=%#v err=%v", result, err)
	}
	if raw, err := os.ReadFile(marker); err != nil || string(raw) != "called\n" {
		t.Fatalf("harmless fake control not invoked: bytes=%q err=%v", raw, err)
	}
}
