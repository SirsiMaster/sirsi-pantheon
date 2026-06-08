package router

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testLaunchPlist renders a minimal launchd plist whose first ProgramArguments
// entry is binary — enough to exercise LaunchAgentProgram and node-status's
// launch-agent inspection. (The push-model RenderLaunchAgentPlist that used to
// build these was removed with the daemon cluster per A26/A27.)
func testLaunchPlist(binary string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>ProgramArguments</key>
  <array>
    <string>` + binary + `</string>
  </array>
</dict>
</plist>
`
}

func TestDefaultServiceOptionsAreRepoSpecific(t *testing.T) {
	opts := DefaultServiceOptions("/tmp/Sirsi Pantheon!", "/bin/sirsi")
	if !strings.Contains(opts.Label, "sirsi-pantheon") {
		t.Fatalf("label = %q, want repo slug", opts.Label)
	}
	if !strings.Contains(opts.PlistPath, opts.Label+".plist") {
		t.Fatalf("plist path = %q, want label plist", opts.PlistPath)
	}
}

func TestIsGoRunBinaryDetectsTemporaryExecutable(t *testing.T) {
	path := filepath.Join(os.TempDir(), "go-build123", "b001", "exe", "sirsi")
	if !IsGoRunBinary(path) {
		t.Fatalf("expected %q to be detected as go-run binary", path)
	}
	cachePath := filepath.Join(os.TempDir(), "sirsi-go-cache", "ad", "ad34eab1485676f0ffa3732e9201413cee6d5300d011f99f677bf74bef13aa7d-d", "sirsi")
	t.Setenv("GOCACHE", filepath.Join(os.TempDir(), "sirsi-go-cache"))
	if !IsGoRunBinary(cachePath) {
		t.Fatalf("expected %q to be detected as go-run cache binary", cachePath)
	}
	if IsGoRunBinary("/usr/local/bin/sirsi") {
		t.Fatal("stable binary reported as go-run binary")
	}
}

func TestLaunchAgentProgramReadsConfiguredBinary(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "agent.plist")
	plist := testLaunchPlist("/tmp/with spaces/sirsi")
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LaunchAgentProgram(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/with spaces/sirsi" {
		t.Fatalf("program = %q, want configured binary", got)
	}
}
