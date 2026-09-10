package router

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func stubRelayLoad(t *testing.T) *[]string {
	t.Helper()
	var loaded []string
	prev := loadRelayAgentFn
	loadRelayAgentFn = func(p string) error { loaded = append(loaded, p); return nil }
	t.Cleanup(func() { loadRelayAgentFn = prev })
	return &loaded
}

func TestInstallRelayLaunchAgentIsPrivateAndCarriesTheToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	loaded := stubRelayLoad(t)
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o700); err != nil {
		t.Fatal(err)
	}
	spool := filepath.Join(home, ".sirsi", "relay")
	changed, path, err := InstallRelayLaunchAgent(spool, "https://router.example.test", "t<&>k")
	canonSpool, _ := filepath.EvalSymlinks(spool)
	if err != nil || !changed {
		t.Fatalf("install: changed=%v err=%v", changed, err)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("plist mode %o, want 0600", st.Mode().Perm())
	}
	self, _ := os.Executable()
	self, _ = filepath.EvalSymlinks(self)
	b, _ := os.ReadFile(path)
	for _, want := range []string{"<string>ai.sirsi.router.relay</string>", "<string>https://router.example.test</string>", "<string>t&lt;&amp;&gt;k</string>", "<string>relay</string>", "<string>--spool</string>", "<string>" + canonSpool + "</string>", "<string>" + self + "</string>"} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("plist missing %q:\n%s", want, b)
		}
	}
	if st, _ := os.Stat(spool); st == nil || st.Mode().Perm() != 0o700 {
		t.Fatal("spool dir must exist at 0700")
	}
	if len(*loaded) != 1 || (*loaded)[0] != path {
		t.Fatalf("launchctl must receive exactly the generated plist: %v", *loaded)
	}
	// Idempotent and re-tightened.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	changed, _, err = InstallRelayLaunchAgent(spool, "https://router.example.test", "t<&>k")
	if err != nil || changed {
		t.Fatalf("idempotent: changed=%v err=%v", changed, err)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Fatalf("idempotent mode %o, want 0600", st.Mode().Perm())
	}
	// Refusals: spool URL, empty token, relative spool.
	for name, c := range map[string][3]string{
		"spool URL":      {spool, "spool://x", "t"},
		"empty token":    {spool, "https://x", ""},
		"relative spool": {"relay", "https://x", "t"},
	} {
		if _, _, err := InstallRelayLaunchAgent(c[0], c[1], c[2]); err == nil {
			t.Fatalf("%s must be refused", name)
		}
	}
}

// The spool must be a real owned directory: symlink, regular file, and a
// parent that resolves through a symlink are all refused; a loose mode is
// tightened.
func TestCheckSpoolDirFailsClosed(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := routerstore.CheckSpoolDir(link); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink spool must be refused: %v", err)
	}
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := routerstore.CheckSpoolDir(file); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("non-directory spool must be refused: %v", err)
	}
	// A symlinked parent is canonicalized: the returned path is the real one,
	// which is what the plist and the relay bind to.
	if got, err := routerstore.CheckSpoolDir(filepath.Join(link, "child")); err != nil || !strings.HasPrefix(got, mustEval(t, real)) {
		t.Fatalf("symlinked parent must canonicalize to the real tree: %q %v", got, err)
	}
	if _, err := routerstore.CheckSpoolDir("relative/spool"); err == nil {
		t.Fatal("relative spool must be refused")
	}
	loose := filepath.Join(base, "loose")
	if err := os.Mkdir(loose, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(loose, 0o777); err != nil { // umask would otherwise make it 0755
		t.Fatal(err)
	}
	if _, err := routerstore.CheckSpoolDir(filepath.Join(loose, "spool")); err == nil || !strings.Contains(err.Error(), "writable by others") {
		t.Fatalf("world-writable parent must be refused: %v", err)
	}
	if got, err := routerstore.CheckSpoolDir(real); err != nil || got != mustEval(t, real) {
		t.Fatalf("real dir must pass with its canonical path: %q %v", got, err)
	}
	if st, _ := os.Stat(real); st.Mode().Perm() != 0o700 {
		t.Fatalf("loose mode must be tightened to 0700, got %o", st.Mode().Perm())
	}
	fresh := filepath.Join(base, "fresh")
	if _, err := routerstore.CheckSpoolDir(fresh); err != nil {
		t.Fatalf("absent spool must be created: %v", err)
	}
}

// launchctl only ever consumes the exact bytes the installer wrote at 0600: a
// substituted plist, a plist changed after write, a wrong mode and a symlink
// are all refused before bootstrap.
func TestLoadRelayAgentRefusesSubstitutedPlist(t *testing.T) {
	stubRelayLoad(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "ai.sirsi.router.relay.plist")
	content := "<plist>generated</plist>\n"
	if _, _, err := writePrivatePlist(p, content); err != nil {
		t.Fatal(err)
	}
	if err := verifyPlist(p, content); err != nil {
		t.Fatalf("exact bytes at 0600 must verify: %v", err)
	}
	if err := os.WriteFile(p, []byte("<plist>substituted</plist>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loadRelayAgent(p, content); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("substituted plist must be refused: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o644); err != nil { // WriteFile keeps an existing file's mode
		t.Fatal(err)
	}
	if err := loadRelayAgent(p, content); err == nil || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("wrong mode must be refused: %v", err)
	}
	_ = os.Remove(p)
	other := filepath.Join(dir, "other.plist")
	if err := os.WriteFile(other, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, p); err != nil {
		t.Fatal(err)
	}
	if err := loadRelayAgent(p, content); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("symlinked plist must be refused: %v", err)
	}
}

// The LaunchAgent is scoped to the running binary: the identity check refuses
// relative, non-executable and non-regular candidates and resolves symlinks.
func TestCheckExecutableIdentity(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "sirsi")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(exe, link); err != nil {
		t.Fatal(err)
	}
	if got, err := checkExecutable(link); err != nil || got != mustEval(t, exe) {
		t.Fatalf("symlink must resolve to the real executable: %q %v", got, err)
	}
	noexec := filepath.Join(dir, "noexec")
	if err := os.WriteFile(noexec, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := checkExecutable(noexec); err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("non-executable must be refused: %v", err)
	}
	if _, err := checkExecutable("sirsi"); err == nil {
		t.Fatal("relative path must be refused")
	}
	if _, err := checkExecutable(dir); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("directory must be refused: %v", err)
	}
	if self, err := runningExecutable(); err != nil || !filepath.IsAbs(self) {
		t.Fatalf("running executable must resolve: %q %v", self, err)
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	c, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
