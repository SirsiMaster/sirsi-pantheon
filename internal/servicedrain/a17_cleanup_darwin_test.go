//go:build darwin

package servicedrain

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func shortOwnedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/private/tmp", "sd-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

func fixtureMaintenanceServer(t *testing.T) (*maintenanceServer, string) {
	t.Helper()
	dir := shortOwnedTempDir(t)
	path := filepath.Join(dir, "relay.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	parent, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	var socket unix.Stat_t
	if err := unix.Fstatat(parent, filepath.Base(path), &socket, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		_ = listener.Close()
		_ = unix.Close(parent)
		t.Fatal(err)
	}
	done := make(chan struct{})
	close(done)
	return &maintenanceServer{l: listener, done: done, parent: parent, leaf: filepath.Base(path), stat: socket, clients: make(map[*net.UnixConn]bool)}, path
}

func TestMaintenanceServerCloseRemovesOwnedSocket(t *testing.T) {
	server, path := fixtureMaintenanceServer(t)
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned socket remains after close: %v", err)
	}
}

func TestMaintenanceServerCloseRetainsReplacedPath(t *testing.T) {
	server, path := fixtureMaintenanceServer(t)
	if err := server.l.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	const replacement = "inert replacement fixture"
	if err := os.WriteFile(path, []byte(replacement), 0600); err != nil {
		t.Fatal(err)
	}
	if err := server.Close(); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("replacement cleanup result = %v, want retained cleanup debt", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != replacement {
		t.Fatalf("replacement path was removed or altered: %q, %v", got, err)
	}
}

func TestMaintenanceKernelIdentityAndRootControllerGate(t *testing.T) {
	if got := MaintenanceEndpoint(42, 99); got != "/private/tmp/pantheon-service-42/ai.sirsi.router.relay-99.sock" {
		t.Fatalf("maintenance endpoint = %q", got)
	}
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		if _, err := ReadMaintenance(context.Background(), os.Getuid(), -1, "fixture", nil); err == nil || !strings.Contains(err.Error(), "native root controller required") {
			t.Fatalf("unprivileged controller result = %v", err)
		}
	}
}

func TestMaintenancePeerReadsOnlyConnectedFixtureCredentials(t *testing.T) {
	path := filepath.Join(shortOwnedTempDir(t), "peer.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan *net.UnixConn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := listener.AcceptUnix()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()
	client, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var serverConn *net.UnixConn
	select {
	case serverConn = <-accepted:
	case err := <-acceptErr:
		t.Fatal(err)
	}
	defer serverConn.Close()
	pid, uid, err := maintenancePeer(serverConn)
	if err != nil || pid != os.Getpid() || uid != os.Getuid() {
		t.Fatalf("fixture peer = pid %d uid %d err %v", pid, uid, err)
	}
}
