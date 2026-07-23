package setup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeMLXRuntime drops the mlx_lm.server marker into a temp HOME so
// InstallGemmaBroker passes its runtime guard.
func fakeMLXRuntime(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".venvs", "mlx", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mlx_lm.server"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestGemmaBrokerPlistContent(t *testing.T) {
	got := gemmaBrokerPlistContent("/usr/local/bin/sirsi", "/Users/u")
	for _, want := range []string{
		"<string>ai.sirsi.gemma-broker</string>",
		"<string>/usr/local/bin/sirsi</string>",
		"<string>--foreground</string>",
		"<key>KeepAlive</key>",
		"<key>RunAtLoad</key>",
		"<key>ThrottleInterval</key>",
		"<integer>30</integer>",
		"<string>Interactive</string>",
		"HF_HUB_OFFLINE",
		"/Users/u/.sirsi/gemma-server.log",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plist missing %q:\n%s", want, got)
		}
	}
}

func TestInstallGemmaBroker_SkipsWithoutMLX(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin only")
	}
	t.Setenv("HOME", t.TempDir())
	res := InstallGemmaBroker()
	if res.Status != StatusSkipped || !strings.Contains(res.Message, "MLX") {
		t.Fatalf("want skipped-without-MLX, got %v %q", res.Status, res.Message)
	}
}

func TestInstallGemmaBroker_InstallsAndRetiresLegacy(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	fakeMLXRuntime(t, home)
	calls := stubLaunchctl(t)

	// Legacy one-shot launcher present — must be retired.
	agentDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(agentDir, legacyGemmaLauncherLabel+".plist")
	if err := os.WriteFile(legacy, []byte("<plist/>\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := InstallGemmaBroker()
	if res.Status != StatusOK {
		t.Fatalf("install failed: %v %q", res.Status, res.Message)
	}
	if !strings.Contains(res.Message, "retired") {
		t.Errorf("message should report the retired legacy launcher: %q", res.Message)
	}
	if fileExists(legacy) {
		t.Error("legacy ai.sirsi.gemma.plist still present")
	}
	plist := filepath.Join(agentDir, GemmaBrokerLabel+".plist")
	b, err := os.ReadFile(plist)
	if err != nil {
		t.Fatalf("broker plist not written: %v", err)
	}
	if !strings.Contains(string(b), "--foreground") {
		t.Error("broker plist does not run `gemma serve --foreground`")
	}
	var loaded bool
	for _, c := range *calls {
		if len(c) == 2 && c[0] == "load" && c[1] == plist {
			loaded = true
		}
	}
	if !loaded {
		t.Errorf("launchctl load not invoked; calls: %v", *calls)
	}
	if !GemmaBrokerInstalled() {
		t.Error("GemmaBrokerInstalled() false after install")
	}

	// Idempotence: a second run changes nothing.
	res2 := InstallGemmaBroker()
	if res2.Status != StatusOK || !strings.Contains(res2.Message, "no change") {
		t.Fatalf("second install should be a no-change no-op, got %v %q", res2.Status, res2.Message)
	}
}
