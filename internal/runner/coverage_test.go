package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunnerPureHelpersAndMalformedInputs(t *testing.T) {
	if Base() == "" {
		t.Fatal("Base returned an empty path")
	}
	owner, bareRepo := ParseRepoArg("repo")
	if owner != DefaultOwner || bareRepo != "repo" {
		t.Fatalf("bare ParseRepoArg = %q/%q", owner, bareRepo)
	}
	owner, repo := ParseRepoArg("acme/repo")
	if owner != "acme" || repo != "repo" {
		t.Fatalf("ParseRepoArg = %q/%q", owner, repo)
	}
	if got := ParseGitHubRemote("git@github.com:acme/repo.git"); got != "acme/repo" {
		t.Fatalf("ssh remote = %q", got)
	}
	if got := ParseGitHubRemote("ssh://git@github.com/acme/repo"); got != "acme/repo" {
		t.Fatalf("ssh URL = %q", got)
	}
	for _, bad := range []string{"", "https://example.com/a/b", "https://github.com/a", "https://github.com/a/b/c"} {
		if got := ParseGitHubRemote(bad); got != "" {
			t.Errorf("ParseGitHubRemote(%q) = %q", bad, got)
		}
	}
	if status, busy := ClassifyStatus([]APIRunner{{Name: "r", Status: "online", Busy: true}}, "r"); status != "online" || !busy {
		t.Fatalf("runner status = %q/%v", status, busy)
	}
	if status, busy := ClassifyStatus(nil, "missing"); status != "unregistered" || busy {
		t.Fatalf("missing status = %q/%v", status, busy)
	}
	tmp := t.TempDir()
	if dirs, err := InstalledDirs(filepath.Join(tmp, "absent")); err != nil || dirs != nil {
		t.Fatalf("absent installed dirs = %#v, %v", dirs, err)
	}
	d := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, ".runner"), []byte(`{"gitHubUrl":"https://github.com/acme/repo/","agentName":"r"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if dirs, err := InstalledDirs(tmp); err != nil || len(dirs) != 1 || dirs[0] != d {
		t.Fatalf("installed dirs = %#v, %v", dirs, err)
	}
	if ownerRepo, agent, err := ParseRunnerFile([]byte("\ufeff{\"gitHubUrl\":\"https://github.com/acme/repo\",\"agentName\":\"r\"}")); err != nil || ownerRepo != "acme/repo" || agent != "r" {
		t.Fatalf("BOM runner = %q/%q/%v", ownerRepo, agent, err)
	}
	if _, _, err := ParseRunnerFile([]byte("not json")); err == nil {
		t.Fatal("malformed runner accepted")
	}
}

func TestInstallInstanceWithHermeticTooling(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	t.Setenv("HOME", home)
	writeExe := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// These shims model only the external contracts InstallInstance relies on.
	// The final two arguments are the source and destination; flags are ignored.
	writeExe("rsync", `#!/bin/sh
src=""; dst=""
for arg in "$@"; do src="$dst"; dst="$arg"; done
cp -R "$src/." "$dst/."
`)
	writeExe("gh", "#!/bin/sh\ncase \"$*\" in *registration-token*) printf token ;; *) printf '{\"runners\":[{\"name\":\"m5-sirsi\",\"status\":\"online\",\"busy\":false}]}' ;; esac\n")
	writeExe("cp", "#!/bin/sh\nexec /bin/cp \"$@\"\n")
	writeExe("chmod", "#!/bin/sh\nexec /bin/chmod \"$@\"\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	src := filepath.Join(home, ".sirsi", "actions-runner", "sirsi-pantheon")
	if err := os.MkdirAll(filepath.Join(src, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"config.sh":     "#!/bin/sh\nprintf '{\"gitHubUrl\":\"https://github.com/SirsiMaster/demo\",\"agentName\":\"m5-sirsi\"}' > .runner\n",
		"svc.sh":        "#!/bin/sh\nexit 0\n",
		"bin/runsvc.sh": "#!/bin/sh\nexit 0\n",
	} {
		p := filepath.Join(src, name)
		if err := os.WriteFile(p, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := InstallInstance("SirsiMaster", "demo", 1, nil); err != nil {
		t.Fatalf("hermetic install failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".sirsi", "actions-runner", "demo", ".runner")); err != nil {
		t.Fatalf("configured runner missing: %v", err)
	}
}
