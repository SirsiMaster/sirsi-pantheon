//go:build darwin

package cleaner

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// TestLaunchctlPrintShapeCanary detects drift in the `launchctl print` output
// format this package parses.
//
// `launchctl print` is a human-facing diagnostic with no stability guarantee.
// If Apple changes the `arguments = { … }` block, parseLaunchctlArguments
// returns nothing. Runtime behavior is safe either way — discovery failure
// fails CLOSED — but the failure mode is "all cleanup refuses", which is
// correct and also useless. This canary turns a silent format change into a
// visible CI failure BEFORE it reaches a workstation.
//
// The canary is diagnostic, not the safety control. Skipped when no SNE job is
// loaded, so it is meaningful on the self-hosted macOS runner and quiet on
// hosted runners and clean checkouts.
func TestLaunchctlPrintShapeCanary(t *testing.T) {
	uid := strconv.Itoa(os.Getuid())
	out, err := exec.Command("launchctl", "print", "gui/"+uid+"/"+canonicalSNELabel).Output()
	if err != nil {
		t.Skipf("%s not loaded in this domain — canary is only meaningful where the engine runs", canonicalSNELabel)
	}

	printed := string(out)
	if !strings.Contains(printed, "arguments = {") {
		t.Fatalf("launchctl print output no longer contains an `arguments = {` block — parseLaunchctlArguments will return nothing and the cleaner will fail closed on every delete. Output was:\n%s", printed)
	}

	args := parseLaunchctlArguments(printed)
	if len(args) == 0 {
		t.Fatal("parseLaunchctlArguments returned nothing from real launchctl output — the format has drifted")
	}
	// The serve contract itself: a loaded engine must still yield a model dir.
	if _, ok := sneModelDir(args); !ok {
		t.Errorf("real argv %v no longer matches the `serve <modelDir>` contract — either launchd output or the engine's invocation changed", args)
	}
}
