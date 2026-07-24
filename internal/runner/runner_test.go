package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseRepoArg(t *testing.T) {
	cases := []struct{ in, owner, repo string }{
		{"assiduous", DefaultOwner, "assiduous"},
		{"SirsiMaster/assiduous", "SirsiMaster", "assiduous"},
		{"other-org/tool", "other-org", "tool"},
	}
	for _, c := range cases {
		o, r := ParseRepoArg(c.in)
		if o != c.owner || r != c.repo {
			t.Errorf("ParseRepoArg(%q) = %q/%q, want %q/%q", c.in, o, r, c.owner, c.repo)
		}
	}
}

func TestParseRunnerFile(t *testing.T) {
	data := []byte(`{"gitHubUrl":"https://github.com/SirsiMaster/assiduous","agentName":"m5-sirsi"}`)
	ownerRepo, name, err := ParseRunnerFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if ownerRepo != "SirsiMaster/assiduous" || name != "m5-sirsi" {
		t.Errorf("got %q %q", ownerRepo, name)
	}
	if _, _, badErr := ParseRunnerFile([]byte(`{"gitHubUrl":"https://example.com/x"}`)); badErr == nil {
		t.Error("non-github URL should error")
	}
	if _, _, badErr := ParseRunnerFile([]byte(`not json`)); badErr == nil {
		t.Error("bad JSON should error")
	}
	// The real actions-runner writes .runner with a UTF-8 BOM — must parse.
	bom := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"gitHubUrl":"https://github.com/SirsiMaster/sirsi-pantheon","agentName":"m5-sirsi"}`)...)
	ownerRepo, name, err = ParseRunnerFile(bom)
	if err != nil || ownerRepo != "SirsiMaster/sirsi-pantheon" || name != "m5-sirsi" {
		t.Errorf("BOM file: got %q %q err=%v", ownerRepo, name, err)
	}
}

func TestParseGitHubRemote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"git@github.com:SirsiMaster/assiduous.git", "SirsiMaster/assiduous"},
		{"https://github.com/SirsiMaster/assiduous.git", "SirsiMaster/assiduous"},
		{"https://github.com/SirsiMaster/assiduous", "SirsiMaster/assiduous"},
		{"ssh://git@github.com/o/r.git", "o/r"},
		{"https://gitlab.com/o/r.git", ""},
		{"not a url", ""},
		{"https://github.com/only-owner", ""},
	}
	for _, c := range cases {
		if got := ParseGitHubRemote(c.in); got != c.want {
			t.Errorf("ParseGitHubRemote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestClassifyStatus(t *testing.T) {
	rows := []APIRunner{
		{Name: "m5-sirsi", Status: "online", Busy: true},
		{Name: "other", Status: "offline"},
	}
	if st, busy := ClassifyStatus(rows, "m5-sirsi"); st != "online" || !busy {
		t.Errorf("got %q busy=%v", st, busy)
	}
	if st, _ := ClassifyStatus(rows, "ghost"); st != "unregistered" {
		t.Errorf("missing runner should be unregistered, got %q", st)
	}
}

func TestInstalledDirs(t *testing.T) {
	base := t.TempDir()
	configured := filepath.Join(base, "repo-a")
	bare := filepath.Join(base, "repo-b")
	for _, d := range []string{configured, bare} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(configured, ".runner"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirs, err := InstalledDirs(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || filepath.Base(dirs[0]) != "repo-a" {
		t.Errorf("want only repo-a, got %v", dirs)
	}
	// Missing base is not an error — just an empty fleet.
	if dirs, err := InstalledDirs(filepath.Join(base, "nope")); err != nil || dirs != nil {
		t.Errorf("missing base: got %v, %v", dirs, err)
	}
}

func TestInstanceNaming(t *testing.T) {
	cases := []struct {
		instance      int
		wantSuffix    string
		wantRunner    string
		wantDirSuffix string
	}{
		{0, "", "m5-sirsi", "sirsi-pantheon"}, // guard: <=1 is instance 1
		{1, "", "m5-sirsi", "sirsi-pantheon"}, // canonical single runner — no suffix
		{2, "-2", "m5-sirsi-2", "sirsi-pantheon-2"},
		{3, "-3", "m5-sirsi-3", "sirsi-pantheon-3"},
	}
	for _, c := range cases {
		if got := instanceSuffix(c.instance); got != c.wantSuffix {
			t.Errorf("instanceSuffix(%d) = %q, want %q", c.instance, got, c.wantSuffix)
		}
		if got := instanceRunnerName(c.instance); got != c.wantRunner {
			t.Errorf("instanceRunnerName(%d) = %q, want %q", c.instance, got, c.wantRunner)
		}
		if got := instanceDir("sirsi-pantheon", c.instance); filepath.Base(got) != c.wantDirSuffix {
			t.Errorf("instanceDir(%d) base = %q, want %q", c.instance, filepath.Base(got), c.wantDirSuffix)
		}
	}
	// Instance 1 must be byte-identical to the pre-instance path.
	if instanceRunnerName(1) != RunnerName {
		t.Error("instance 1 must equal RunnerName exactly")
	}
}
