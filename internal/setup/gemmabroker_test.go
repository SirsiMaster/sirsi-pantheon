package setup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeSNERuntime drops the native binary and pinned model markers into a temp
// HOME so InstallGemmaBroker passes its fail-closed install guards.
func fakeSNERuntime(t *testing.T, home string) {
	t.Helper()
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "sirsi-inference"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	modelDir := filepath.Join(home, ".cache", "huggingface", "hub", "models--mlx-community--gemma-4-12B-it-8bit", "snapshots", "200bb6db075e137a4deb08838865ac4ddb86292e")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "model.safetensors.index.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGemmaBrokerPlistContent(t *testing.T) {
	got := gemmaBrokerPlistContent("/usr/local/bin/sirsi", "/Users/u")
	for _, want := range []string{
		"<string>ai.sirsi.gemma-broker</string>",
		"<string>/Users/u/.local/bin/sirsi-inference</string>",
		"<string>--profile</string>",
		"<string>interactive</string>",
		"<string>127.0.0.1:8477</string>",
		"<key>KeepAlive</key>",
		"<key>RunAtLoad</key>",
		"<key>ThrottleInterval</key>",
		"<integer>30</integer>",
		"<string>Interactive</string>",
		"/Users/u/.sirsi/sne-server.log",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plist missing %q:\n%s", want, got)
		}
	}
}

func TestInstallGemmaBroker_SkipsWithoutSNE(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin only")
	}
	t.Setenv("HOME", t.TempDir())
	res := InstallGemmaBroker()
	if res.Status != StatusSkipped || !strings.Contains(res.Message, "SNE") {
		t.Fatalf("want skipped-without-SNE, got %v %q", res.Status, res.Message)
	}
}

func TestInstallGemmaBroker_ProvidedByApolloApp(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	fakeSNERuntime(t, home) // SNE present too: Apollo.app must still win
	router := filepath.Join(home, "Applications", "Apollo.app", "Contents", "Helpers", "apollo-router")
	if err := os.MkdirAll(filepath.Dir(router), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(router, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	res := InstallGemmaBroker()
	if res.Status != StatusSkipped || !strings.Contains(res.Message, "Apollo.app") {
		t.Fatalf("want provided-by-Apollo skip, got %v %q", res.Status, res.Message)
	}
	if fileExists(GemmaBrokerPlistPath()) {
		t.Fatal("setup wrote the broker plist although Apollo.app provides the model")
	}
	port, err := os.ReadFile(filepath.Join(home, ".sirsi", "gemma-server.port"))
	if err != nil || strings.TrimSpace(string(port)) != "8520" {
		t.Fatalf("port file = %q, %v; want 8520", port, err)
	}
}

// apolloHome builds a temp HOME with SNE and Apollo.app's router present.
func apolloHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	fakeSNERuntime(t, home)
	router := filepath.Join(home, "Applications", "Apollo.app", "Contents", "Helpers", "apollo-router")
	if err := os.MkdirAll(filepath.Dir(router), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(router, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

func assertApolloRepointFailed(t *testing.T, res InstallResult) {
	t.Helper()
	if res.Status != StatusFailed || !strings.Contains(res.Message, "could not be repointed") {
		t.Fatalf("want failed repoint, got %v %q", res.Status, res.Message)
	}
	if fileExists(GemmaBrokerPlistPath()) {
		t.Fatal("failed repoint fell back to writing the legacy broker plist")
	}
}

func TestInstallGemmaBroker_ApolloRepointFailures(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin only")
	}
	t.Run("obstructed parent dir", func(t *testing.T) {
		home := apolloHome(t)
		if err := os.WriteFile(filepath.Join(home, ".sirsi"), []byte("not a dir"), 0o644); err != nil {
			t.Fatal(err)
		}
		assertApolloRepointFailed(t, InstallGemmaBroker())
	})
	t.Run("obstructed target file", func(t *testing.T) {
		home := apolloHome(t)
		target := filepath.Join(home, ".sirsi", "gemma-server.port")
		if err := os.MkdirAll(filepath.Join(target, "child"), 0o755); err != nil {
			t.Fatal(err)
		}
		assertApolloRepointFailed(t, InstallGemmaBroker())
		if fi, err := os.Stat(filepath.Join(target, "child")); err != nil || !fi.IsDir() {
			t.Fatalf("obstruction was disturbed: %v", err)
		}
	})
	t.Run("unwritable dir keeps previous pointer", func(t *testing.T) {
		home := apolloHome(t)
		dir := filepath.Join(home, ".sirsi")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "gemma-server.port"), []byte("8477\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		assertApolloRepointFailed(t, InstallGemmaBroker())
		if b, _ := os.ReadFile(filepath.Join(dir, "gemma-server.port")); string(b) != "8477\n" {
			t.Fatalf("previous pointer changed to %q", b)
		}
	})
}

func TestInstallGemmaBroker_HonorsQuarantine(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	fakeSNERuntime(t, home)
	if err := os.MkdirAll(filepath.Dir(GemmaBrokerQuarantinePath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(GemmaBrokerQuarantinePath(), []byte("<plist/>\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := InstallGemmaBroker()
	if res.Status != StatusSkipped || !strings.Contains(res.Message, "quarantined") {
		t.Fatalf("want quarantine-preserving skip, got %v %q", res.Status, res.Message)
	}
	if fileExists(GemmaBrokerPlistPath()) {
		t.Fatal("setup recreated the canonical broker plist during quarantine")
	}
}

func TestGemmaBrokerQuarantineRejectsConflictingDefinitions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Dir(GemmaBrokerPlistPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{GemmaBrokerPlistPath(), GemmaBrokerQuarantinePath()} {
		if err := os.WriteFile(path, []byte("<plist/>\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := QuarantineGemmaBrokerPlist(); err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("quarantine error = %v, want conflicting definitions", err)
	}
	if err := RestoreGemmaBrokerPlist(); err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("restore error = %v, want conflicting definitions", err)
	}
}

func TestInstallGemmaBroker_InstallsAndRetiresLegacy(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	fakeSNERuntime(t, home)
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
	if !strings.Contains(string(b), "sirsi-inference") || !strings.Contains(string(b), "127.0.0.1:8477") {
		t.Error("broker plist does not run native SNE on port 8477")
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
