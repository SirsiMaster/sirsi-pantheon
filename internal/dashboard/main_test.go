package dashboard

import (
	"os"
	"testing"
)

// TestMain points os.TempDir at a private short directory so the singleton
// lock (platform.TryLock -> $TMPDIR/pantheon.dashboard.lock) never collides
// with a real dashboard running on the host. Short path: unix socket limit.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("/tmp", "dashtest")
	if err != nil {
		panic(err)
	}
	old, had := os.LookupEnv("TMPDIR")
	_ = os.Setenv("TMPDIR", dir)
	code := m.Run()
	if had {
		_ = os.Setenv("TMPDIR", old)
	} else {
		_ = os.Unsetenv("TMPDIR")
	}
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
