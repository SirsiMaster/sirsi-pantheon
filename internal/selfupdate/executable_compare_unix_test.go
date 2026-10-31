//go:build darwin || linux

package selfupdate

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/version"
	"golang.org/x/sys/unix"
)

func TestSameExecutableUsesStableBytesWithoutVersionWaiver(t *testing.T) {
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

func TestSameExecutableKeepsEqualUnavailableSpellingAndRejectsUnsafePaths(t *testing.T) {
	root := t.TempDir()
	regular, other := filepath.Join(root, "regular"), filepath.Join(root, "other")
	for _, path := range []string{regular, other} {
		if err := os.WriteFile(path, []byte("regular"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if !sameExecutable(filepath.Join(root, "missing"), filepath.Join(root, "missing")) {
		t.Fatal("equal unavailable spelling changed its existing behavior")
	}
	if sameExecutable(regular, filepath.Join(root, "missing")) || sameExecutable(regular, root) {
		t.Fatal("missing or non-regular target was treated as equivalent")
	}

	ops := defaultExecutableCompareOps()
	ops.open = func(string) (*os.File, error) { return nil, errors.New("injected open failure") }
	if sameExecutableWith(context.Background(), regular, other, ops) {
		t.Fatal("open failure was treated as equivalent")
	}
	ops = defaultExecutableCompareOps()
	pathIdentity := ops.pathIdentity
	ops.pathIdentity = func(path string) (executableIdentity, error) {
		if path == regular {
			return executableIdentity{}, errors.New("injected name revalidation failure")
		}
		return pathIdentity(path)
	}
	if sameExecutableWith(context.Background(), regular, other, ops) {
		t.Fatal("path metadata failure was treated as equivalent")
	}
	ops = defaultExecutableCompareOps()
	ops.digest = func(context.Context, *os.File, int64) ([sha256.Size]byte, error) {
		return [sha256.Size]byte{}, errors.New("injected read failure")
	}
	if sameExecutableWith(context.Background(), regular, other, ops) {
		t.Fatal("read failure was treated as equivalent")
	}
}

func TestSameExecutableRejectsFIFOWithoutBlockingAndCapsHashInput(t *testing.T) {
	root := t.TempDir()
	regular := filepath.Join(root, "regular")
	fifo := filepath.Join(root, "fifo")
	if err := os.WriteFile(regular, []byte("regular"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	if sameExecutable(regular, fifo) {
		t.Fatal("FIFO target was treated as an executable")
	}

	largeLeft, largeRight := filepath.Join(root, "large-left"), filepath.Join(root, "large-right")
	largeSize := maxExecutableCompareBytes + 1
	for _, path := range []string{largeLeft, largeRight} {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o700)
		if err != nil {
			t.Fatalf("create sparse oversized fixture %q: %v", path, err)
		}
		if err := file.Truncate(largeSize); err != nil {
			_ = file.Close()
			t.Fatalf("truncate sparse oversized fixture %q: %v", path, err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	readCalled := false
	ops := defaultExecutableCompareOps()
	ops.digest = func(context.Context, *os.File, int64) ([sha256.Size]byte, error) {
		readCalled = true
		return [sha256.Size]byte{}, nil
	}
	if sameExecutableWith(context.Background(), largeLeft, largeRight, ops) || readCalled {
		t.Fatal("oversized executable reached the hash reader")
	}
}

func TestSameExecutableRejectsRestoredMtimeMutationAndLinkChange(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string, os.FileInfo)
	}{
		{
			name: "same inode bytes with restored mtime",
			mutate: func(t *testing.T, path string, before os.FileInfo) {
				t.Helper()
				if err := os.WriteFile(path, []byte("different binary bytes"), 0o755); err != nil {
					t.Fatalf("mutate retained inode: %v", err)
				}
				if err := os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
					t.Fatalf("restore mtime: %v", err)
				}
			},
		},
		{
			name: "link count changes during hash",
			mutate: func(t *testing.T, path string, _ os.FileInfo) {
				t.Helper()
				if err := os.Link(path, path+".extra-link"); err != nil {
					t.Fatalf("change link count: %v", err)
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
			before, err := os.Stat(left)
			if err != nil {
				t.Fatal(err)
			}
			ops := defaultExecutableCompareOps()
			realDigest := ops.digest
			calls := 0
			ops.digest = func(ctx context.Context, file *os.File, size int64) ([sha256.Size]byte, error) {
				value, err := realDigest(ctx, file, size)
				if err == nil && calls == 0 {
					calls++
					test.mutate(t, left, before)
				}
				return value, err
			}
			if sameExecutableWith(context.Background(), left, right, ops) || calls != 1 {
				t.Fatal("comparison accepted same-inode metadata mutation during hashing")
			}
		})
	}
}

func TestSameExecutableRejectsPathTargetSubstitutionDuringHash(t *testing.T) {
	root := t.TempDir()
	left, right := filepath.Join(root, "left"), filepath.Join(root, "right")
	for _, path := range []string{left, right} {
		if err := os.WriteFile(path, []byte("identical binary bytes"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ops := defaultExecutableCompareOps()
	realDigest := ops.digest
	calls := 0
	ops.digest = func(ctx context.Context, file *os.File, size int64) ([sha256.Size]byte, error) {
		value, err := realDigest(ctx, file, size)
		if err == nil && calls == 0 {
			calls++
			replacement := left + ".replacement"
			if writeErr := os.WriteFile(replacement, []byte("identical binary bytes"), 0o755); writeErr != nil {
				t.Fatalf("write replacement: %v", writeErr)
			}
			if renameErr := os.Rename(replacement, left); renameErr != nil {
				t.Fatalf("replace target name: %v", renameErr)
			}
		}
		return value, err
	}
	if sameExecutableWith(context.Background(), left, right, ops) || calls != 1 {
		t.Fatal("comparison accepted a changed path target")
	}
}

func TestSameExecutableRevalidatesSameFileMetadata(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	alias := filepath.Join(root, "alias")
	if err := os.WriteFile(target, []byte("same inode"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	if !sameExecutable(alias, target) {
		t.Fatal("intentional symlink to the same file was rejected")
	}

	ops := defaultExecutableCompareOps()
	pathIdentity := ops.pathIdentity
	mutated := false
	ops.pathIdentity = func(path string) (executableIdentity, error) {
		identity, err := pathIdentity(path)
		if err == nil && path == alias && !mutated {
			mutated = true
			future := time.Now().Add(time.Hour)
			if err := os.Chtimes(target, future, future); err != nil {
				t.Fatalf("mutate same-inode metadata: %v", err)
			}
		}
		return identity, err
	}
	if sameExecutableWith(context.Background(), alias, target, ops) {
		t.Fatal("same-file path fast path ignored metadata changed during revalidation")
	}
}

func TestSameExecutableStopsWhenDigestContextIsCancelled(t *testing.T) {
	root := t.TempDir()
	left, right := filepath.Join(root, "left"), filepath.Join(root, "right")
	for _, path := range []string{left, right} {
		if err := os.WriteFile(path, []byte("identical binary bytes"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ops := defaultExecutableCompareOps()
	realDigest := ops.digest
	ops.digest = func(ctx context.Context, file *os.File, size int64) ([sha256.Size]byte, error) {
		cancel()
		return realDigest(ctx, file, size)
	}
	if sameExecutableWith(ctx, left, right, ops) {
		t.Fatal("cancelled byte comparison was treated as equivalent")
	}
}
