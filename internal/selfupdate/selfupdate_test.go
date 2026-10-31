package selfupdate

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

func TestDetectMethod(t *testing.T) {
	tests := []struct {
		name string
		path string
		want Method
	}{
		{"empty", "", MethodUnknown},
		{"homebrew arm", "/opt/homebrew/bin/sirsi", MethodHomebrew},
		{"homebrew intel", "/usr/local/bin/sirsi", MethodHomebrew},
		{"raw local", "/Users/x/.local/bin/sirsi", MethodRaw},
		{"go-build cache", "/tmp/go-build123/b001/exe/sirsi", MethodGoRun},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectMethod(tt.path); got != tt.want {
				t.Errorf("DetectMethod(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func sib(binary, ver, path string) Sibling {
	return Sibling{Info: version.Info{Binary: binary, Version: ver, Path: path}, Method: MethodRaw}
}

func TestBuildReport_Healthy(t *testing.T) {
	self := version.Info{Binary: "sirsi", Version: "v0.22.0", Path: "/Users/x/.local/bin/sirsi"}
	r := BuildReport(self, []Sibling{sib("sirsi-menubar", "v0.22.0", "/opt/homebrew/bin/sirsi-menubar")}, "")
	if !r.Healthy {
		t.Fatalf("expected healthy, got drift: %s", r.Summary())
	}
	if len(r.D2Mismatch) != 0 {
		t.Errorf("D2Mismatch = %d, want 0", len(r.D2Mismatch))
	}
}

func TestBuildReport_D2SiblingDrift(t *testing.T) {
	// The tonight bug: sirsi fresh, menubar stale.
	self := version.Info{Binary: "sirsi", Version: "v0.22.0", Path: "/Users/x/.local/bin/sirsi"}
	r := BuildReport(self, []Sibling{sib("sirsi-menubar", "v0.20.0", "/opt/homebrew/bin/sirsi-menubar")}, "")
	if r.Healthy {
		t.Fatal("expected drift, got healthy")
	}
	if len(r.D2Mismatch) != 1 || r.D2Mismatch[0].Binary != "sirsi-menubar" {
		t.Fatalf("D2Mismatch = %+v, want one sirsi-menubar entry", r.D2Mismatch)
	}
}

func TestBuildReport_IgnoresUnknownAndDevSiblings(t *testing.T) {
	self := version.Info{Binary: "sirsi", Version: "v0.22.0"}
	siblings := []Sibling{
		sib("sirsi-menubar", "dev", "/x/sirsi-menubar"),                   // unstamped — not a real mismatch
		{Info: version.Info{Binary: "sirsi-menubar"}, Err: "exec failed"}, // unprobeable
	}
	r := BuildReport(self, siblings, "")
	if !r.Healthy {
		t.Errorf("dev/errored siblings must not count as drift: %s", r.Summary())
	}
}

func TestBuildReport_D3PathDrift(t *testing.T) {
	self := version.Info{Binary: "sirsi", Version: "v0.22.0", Path: "/Users/x/.local/bin/sirsi"}
	r := BuildReport(self, nil, "/opt/homebrew/bin/sirsi")
	if r.Healthy {
		t.Fatal("expected D3 path drift")
	}
	if r.D3PathBin != "/opt/homebrew/bin/sirsi" {
		t.Errorf("D3PathBin = %q", r.D3PathBin)
	}
}

func TestBuildReport_D3PathCaskSymlinkIsNotDrift(t *testing.T) {
	root := t.TempDir()
	bundleBinary := filepath.Join(root, "Pantheon.app", "Contents", "MacOS", "sirsi")
	if err := os.MkdirAll(filepath.Dir(bundleBinary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bundleBinary, []byte("signed release"), 0o755); err != nil {
		t.Fatal(err)
	}
	pathBinary := filepath.Join(root, "bin", "sirsi")
	if err := os.MkdirAll(filepath.Dir(pathBinary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(bundleBinary, pathBinary); err != nil {
		t.Fatal(err)
	}

	r := BuildReport(version.Info{Binary: "sirsi", Version: "v-test", Path: bundleBinary}, nil, pathBinary)
	if !r.Healthy {
		t.Fatalf("Cask symlink to the running app binary must be healthy, got %s", r.Summary())
	}
}

func TestSameExecutableUsesSameFileThenStableBytesWithoutVersionWaiver(t *testing.T) {
	root := t.TempDir()
	left := filepath.Join(root, "local", "sirsi")
	right := filepath.Join(root, "app", "sirsi")
	for _, path := range []string{left, right} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("identical binary bytes"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if !sameExecutable(left, right) {
		t.Fatal("distinct installed copies with identical bytes were reported as drift")
	}
	if got := BuildReport(version.Info{Binary: "sirsi", Version: "153", Path: right}, nil, left); !got.Healthy {
		t.Fatalf("byte-identical copies with equal versions were reported as drift: %s", got.Summary())
	}

	if err := os.WriteFile(left, []byte("different binary bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	unequal := BuildReport(version.Info{Binary: "sirsi", Version: "153", Path: right}, nil, left)
	if unequal.Healthy || unequal.D3PathBin != filepath.Clean(left) {
		t.Fatalf("same-version unequal bytes were waived: %+v", unequal)
	}
	if err := os.WriteFile(left, []byte("identical binary bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	changedVersion := BuildReport(version.Info{Binary: "sirsi", Version: "154", Path: right},
		[]Sibling{sib("sirsi-menubar", "153", left)}, left)
	if len(changedVersion.D2Mismatch) != 1 || changedVersion.D3PathBin != "" {
		t.Fatalf("byte equality changed the independent version mismatch: %+v", changedVersion)
	}
}

func TestSameExecutablePreservesFailClosedPathAndReadCases(t *testing.T) {
	root := t.TempDir()
	regular := filepath.Join(root, "regular")
	missing := filepath.Join(root, "missing")
	if err := os.WriteFile(regular, []byte("regular"), 0o700); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(root, "other")
	if err := os.WriteFile(other, []byte("regular"), 0o700); err != nil {
		t.Fatal(err)
	}
	if !sameExecutable(missing, missing) {
		t.Fatal("equal unavailable spelling changed its existing conservative behavior")
	}
	if sameExecutable(regular, missing) {
		t.Fatal("missing target was treated as byte-equivalent")
	}
	if sameExecutable(regular, root) {
		t.Fatal("non-regular target was treated as byte-equivalent")
	}
	if sameExecutableWith(regular, missing, func(string) (*os.File, error) {
		return nil, errors.New("injected open failure")
	}, os.Stat, digestExecutable) {
		t.Fatal("open failure was treated as byte-equivalent")
	}
	if sameExecutableWith(regular, other, os.Open, func(path string) (os.FileInfo, error) {
		if path == regular {
			return nil, errors.New("injected path revalidation failure")
		}
		return os.Stat(path)
	}, digestExecutable) {
		t.Fatal("path revalidation failure was treated as byte-equivalent")
	}
	if sameExecutableWith(regular, other, os.Open, os.Stat, func(*os.File) ([sha256.Size]byte, error) {
		return [sha256.Size]byte{}, errors.New("injected read failure")
	}) {
		t.Fatal("read failure was treated as byte-equivalent")
	}
}

func TestSameExecutableRejectsContentMutationAndPathSubstitutionDuringHash(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, path string)
	}{
		{
			name: "same inode content mutation",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				if err := os.WriteFile(path, []byte("mutated binary bytes!"), 0o755); err != nil {
					t.Fatalf("mutate retained inode: %v", err)
				}
				future := time.Now().Add(time.Hour)
				if err := os.Chtimes(path, future, future); err != nil {
					t.Fatalf("stamp mutated inode: %v", err)
				}
			},
		},
		{
			name: "path target substitution",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replacement := path + ".replacement"
				if err := os.WriteFile(replacement, []byte("identical binary bytes"), 0o755); err != nil {
					t.Fatalf("write replacement: %v", err)
				}
				if err := os.Rename(replacement, path); err != nil {
					t.Fatalf("replace path target: %v", err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			left, right := filepath.Join(root, "left"), filepath.Join(root, "right")
			for _, path := range []string{left, right} {
				if err := os.WriteFile(path, []byte("identical binary bytes"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			realDigest := digestExecutable
			calls := 0
			digest := func(file *os.File) ([sha256.Size]byte, error) {
				value, err := realDigest(file)
				if err == nil && calls == 0 {
					calls++
					test.mutate(t, left)
				}
				return value, err
			}
			if sameExecutableWith(left, right, os.Open, os.Stat, digest) {
				t.Fatal("comparison accepted a file or path changed during retained-descriptor hashing")
			}
			if calls != 1 {
				t.Fatalf("mutation hook calls = %d, want exactly one", calls)
			}
		})
	}
}

func TestSummary_InSync(t *testing.T) {
	r := BuildReport(version.Info{Version: "v0.22.0"}, nil, "")
	if got := r.Summary(); got != "v0.22.0 in sync" {
		t.Errorf("Summary = %q, want 'v0.22.0 in sync'", got)
	}
}
