package router

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallRelayLaunchAgentIsPrivateAndCarriesTheToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	spool := filepath.Join(home, ".sirsi", "relay")
	changed, path, err := InstallRelayLaunchAgent("/usr/local/bin/sirsi", spool, "https://router.example.test", "t<&>k")
	if err != nil || !changed {
		t.Fatalf("install: changed=%v err=%v", changed, err)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("plist mode %o, want 0600", st.Mode().Perm())
	}
	b, _ := os.ReadFile(path)
	for _, want := range []string{"<string>ai.sirsi.router.relay</string>", "<string>https://router.example.test</string>", "<string>t&lt;&amp;&gt;k</string>", "<string>relay</string>", "<string>--spool</string>", "<string>" + spool + "</string>"} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("plist missing %q:\n%s", want, b)
		}
	}
	if st, _ := os.Stat(spool); st == nil || st.Mode().Perm() != 0o700 {
		t.Fatal("spool dir must exist at 0700")
	}
	// Idempotent and re-tightened.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	changed, _, err = InstallRelayLaunchAgent("/usr/local/bin/sirsi", spool, "https://router.example.test", "t<&>k")
	if err != nil || changed {
		t.Fatalf("idempotent: changed=%v err=%v", changed, err)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Fatalf("idempotent mode %o, want 0600", st.Mode().Perm())
	}
	// Refusals: spool URL, empty token, relative spool.
	if _, _, err := InstallRelayLaunchAgent("/b", spool, "spool://x", "t"); err == nil {
		t.Fatal("spool:// URL must be refused")
	}
	if _, _, err := InstallRelayLaunchAgent("/b", spool, "https://x", ""); err == nil {
		t.Fatal("empty token must be refused")
	}
	if _, _, err := InstallRelayLaunchAgent("/b", "relay", "https://x", "t"); err == nil {
		t.Fatal("relative spool must be refused")
	}
}
