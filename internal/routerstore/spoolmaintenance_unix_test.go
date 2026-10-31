//go:build !windows

package routerstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/servicedrain"
)

func TestMaintenanceAdmissionRetainsPhysicalWriterBarrier(t *testing.T) {
	path := t.TempDir()
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writer, err := lockSpoolAdmission(context.Background(), root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	entered := false
	if err := WithSpoolQuiescence(ctx, path, func() error { entered = true; return nil }); !errors.Is(err, context.DeadlineExceeded) || entered {
		t.Fatalf("exclusive passed active publication: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	// A symlink alias resolves to the same directory lock, not a second store.
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	aliased, err := os.OpenRoot(alias)
	if err != nil {
		t.Fatal(err)
	}
	defer aliased.Close()
	if err := os.WriteFile(filepath.Join(path, servicedrain.MaintenanceMarkerName), []byte("unknown private transaction"), 0600); err != nil {
		t.Fatal(err)
	}
	if f, err := lockSpoolAdmission(context.Background(), aliased, false); err == nil {
		_ = f.Close()
		t.Fatal("pending/unknown transaction allowed new publication")
	}
	if err := WithSpoolQuiescence(context.Background(), path, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	// The lock cannot stop a legacy writer. This proves why maintenance status
	// must retain uncovered-writer uncertainty instead of certifying a drain.
	if err := WithSpoolQuiescence(context.Background(), path, func() error { return os.WriteFile(filepath.Join(path, "legacy-entry"), []byte("owned marker"), 0600) }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(path, "legacy-entry")); err != nil {
		t.Fatal(err)
	}
}
