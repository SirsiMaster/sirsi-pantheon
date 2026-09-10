package router

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A bare binary name is unspawnable under launchd (its PATH has no
// ~/.local/bin) — the 2026-07-07 nine-loop crash-loop (15k respawns, no
// logs). Install must refuse rather than write an unspawnable plist.
func TestInstallWakeRefusesUnresolvableBinary(t *testing.T) {
	old := launchAgentsDirOverride
	launchAgentsDirOverride = t.TempDir()
	t.Cleanup(func() { launchAgentsDirOverride = old })
	t.Setenv("PATH", t.TempDir()) // no sirsi anywhere

	_, _, err := InstallWakeLaunchAgent(AgentConfig{ID: "claude-test"}, "")
	if err == nil || !strings.Contains(err.Error(), "bare name") {
		t.Fatalf("expected a refuse-bare-name error, got %v", err)
	}
}

// The plist must carry the observability + crash-loop bounds: log paths and
// a respawn throttle. Silent KeepAlive death-loops are how nine watchers
// died 15k times without a line of evidence.
func TestWakePlistCarriesLogsAndThrottle(t *testing.T) {
	content := wakeLaunchAgentPlist("ai.sirsi.router.wake.claude-x", AgentConfig{ID: "claude-x"}, "/abs/sirsi")
	for _, want := range []string{"StandardOutPath", "StandardErrorPath", "wake-claude-x.log", "ThrottleInterval"} {
		if !strings.Contains(content, want) {
			t.Fatalf("plist missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "<string>sirsi</string>") {
		t.Fatal("plist must never carry a bare binary name")
	}
	_ = os.Getenv // keep imports honest if assertions change
}

// The wake loop resolves its declared consumer with exec.LookPath. launchd hands
// a job only /usr/bin:/bin:/usr/sbin:/sbin, so without an explicit PATH every
// agent whose CLI lives elsewhere resolves to "no consumer" and the loop
// degrades to WATCH-ONLY — heartbeating forever, dispatching nothing, while its
// inbox strands. That was the live state of all 11 codex lanes on 2026-08-06
// (`consumer command "codex" not found in PATH`, codex sitting in ~/.local/bin).
func TestWakePlistExportsPATHForConsumerLookup(t *testing.T) {
	content := wakeLaunchAgentPlist("ai.sirsi.router.wake.codex-x",
		AgentConfig{ID: "codex-x"}, "/Users/x/.local/bin/sirsi")

	if !strings.Contains(content, "<key>EnvironmentVariables</key>") {
		t.Fatalf("plist must export EnvironmentVariables:\n%s", content)
	}
	// The sirsi binary's own directory must lead: agent CLIs are installed
	// beside it (codex is a symlink next to sirsi in ~/.local/bin).
	if !strings.Contains(content, "<string>/Users/x/.local/bin:") {
		t.Fatalf("PATH must lead with the sirsi binary's directory:\n%s", content)
	}
	for _, want := range []string{"/usr/bin", "/bin", "/opt/homebrew/bin"} {
		if !strings.Contains(content, want) {
			t.Fatalf("PATH missing %q:\n%s", want, content)
		}
	}
}

func TestLaunchAgentPATHLeadsWithBinDirAndDedupes(t *testing.T) {
	got := LaunchAgentPATH("/usr/bin/sirsi")
	if !strings.HasPrefix(got, "/usr/bin:") {
		t.Fatalf("PATH must lead with the binary's own dir, got %q", got)
	}
	// /usr/bin is both the binary's dir and a system dir — it must appear once,
	// or the exported PATH grows a duplicate on every install.
	if n := strings.Count(":"+got+":", ":/usr/bin:"); n != 1 {
		t.Fatalf("/usr/bin must appear exactly once, got %d in %q", n, got)
	}
}

// A cut-over host must hand its wake loops the service address and token: launchd
// runs no shell, so nothing else can. Absent env → no keys (Anubis unchanged).
func TestWakePlistCarriesRouterServiceEnv(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_URL", "https://router.example.test")
	t.Setenv("SIRSI_ROUTER_TOKEN", "tok<&>")
	got := wakeLaunchAgentPlist("ai.sirsi.router.wake.x", AgentConfig{ID: "x"}, "/usr/local/bin/sirsi")
	for _, want := range []string{"<key>SIRSI_ROUTER_URL</key>", "<string>https://router.example.test</string>", "<key>SIRSI_ROUTER_TOKEN</key>", "<string>tok&lt;&amp;&gt;</string>"} {
		if !strings.Contains(got, want) {
			t.Fatalf("plist must carry %q:\n%s", want, got)
		}
	}
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_TOKEN", "")
	if strings.Contains(wakeLaunchAgentPlist("l", AgentConfig{ID: "x"}, "/usr/local/bin/sirsi"), "SIRSI_ROUTER") {
		t.Fatal("no service env → no service keys")
	}
}

// Upgrading a 0644 plist (prior installs) must end 0600 — both when the content
// changes and when it is already current — because it may now carry a token.
func TestInstallWakeLaunchAgentSecuresExistingPlist(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := AgentConfig{ID: "sec"}
	path := filepath.Join(dir, WakeLaunchAgentLabel(cfg.ID)+".plist")
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := InstallWakeLaunchAgent(cfg, "/usr/local/bin/sirsi"); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Fatalf("rewritten plist mode %o, want 0600", st.Mode().Perm())
	}
	// Equal content at 0644 (the idempotent path) must also be tightened.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	changed, _, err := InstallWakeLaunchAgent(cfg, "/usr/local/bin/sirsi")
	if err != nil || changed {
		t.Fatalf("idempotent install: changed=%v err=%v", changed, err)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Fatalf("idempotent plist mode %o, want 0600", st.Mode().Perm())
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".*.plist")); len(left) != 0 {
		t.Fatalf("temp files left behind: %v", left)
	}
}
