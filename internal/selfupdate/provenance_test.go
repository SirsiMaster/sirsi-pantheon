package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

func TestCheckProvenance(t *testing.T) {
	cases := []struct {
		name    string
		info    version.Info
		wantErr error
	}{
		{
			name:    "clean stamped build passes",
			info:    version.Info{Version: "v0.22.0", Commit: "abc1234", Dirty: false},
			wantErr: nil,
		},
		{
			name:    "dirty build rejected",
			info:    version.Info{Version: "v0.22.0", Commit: "abc1234", Dirty: true},
			wantErr: ErrDirtyBuild,
		},
		{
			name:    "dev version rejected",
			info:    version.Info{Version: "dev", Commit: "abc1234", Dirty: false},
			wantErr: ErrUnstampedBuild,
		},
		{
			name:    "empty version rejected",
			info:    version.Info{Version: "", Commit: "abc1234", Dirty: false},
			wantErr: ErrUnstampedBuild,
		},
		{
			name:    "none commit rejected",
			info:    version.Info{Version: "v0.22.0", Commit: "none", Dirty: false},
			wantErr: ErrUnstampedBuild,
		},
		{
			name:    "empty commit rejected",
			info:    version.Info{Version: "v0.22.0", Commit: "", Dirty: false},
			wantErr: ErrUnstampedBuild,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckProvenance(tt.info)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want errors.Is %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckVersionProbe_missingBinary(t *testing.T) {
	_, err := CheckVersionProbe("/nonexistent/path/to/sirsi")
	if !errors.Is(err, ErrVersionProbeFailed) {
		t.Fatalf("expected ErrVersionProbeFailed, got %v", err)
	}
}

func TestCheckVersionProbe_badJSON(t *testing.T) {
	// Write a fake binary that outputs garbage.
	dir := t.TempDir()
	fake := filepath.Join(dir, "sirsi")
	if err := os.WriteFile(fake, []byte("not-json"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := setProbeForTest(t, func(path string) (version.Info, error) {
		return version.Info{}, errors.New("json decode failed")
	})
	defer setProbeForTestRestore(t, old)

	_, err := CheckVersionProbe(fake)
	if !errors.Is(err, ErrVersionProbeFailed) {
		t.Fatalf("expected ErrVersionProbeFailed, got %v", err)
	}
}

func TestCheckVersionProbe_emptyVersion(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "sirsi")
	if err := os.WriteFile(fake, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := setProbeForTest(t, func(string) (version.Info, error) {
		return version.Info{Binary: "sirsi"}, nil // Version is empty
	})
	defer setProbeForTestRestore(t, old)

	_, err := CheckVersionProbe(fake)
	if !errors.Is(err, ErrVersionProbeFailed) {
		t.Fatalf("expected ErrVersionProbeFailed, got %v", err)
	}
}

func TestCheckVersionProbe_valid(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "sirsi")
	if err := os.WriteFile(fake, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := setProbeForTest(t, func(string) (version.Info, error) {
		return version.Info{Binary: "sirsi", Version: "v0.22.0", Commit: "abc"}, nil
	})
	defer setProbeForTestRestore(t, old)

	info, err := CheckVersionProbe(fake)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Version != "v0.22.0" {
		t.Fatalf("unexpected version %q", info.Version)
	}
}

// setProbeForTest swaps probeFn and returns the old value.
func setProbeForTest(t *testing.T, fn func(string) (version.Info, error)) func(string) (version.Info, error) {
	t.Helper()
	old := probeFn
	probeFn = fn
	return old
}

func setProbeForTestRestore(t *testing.T, old func(string) (version.Info, error)) {
	t.Helper()
	probeFn = old
}
