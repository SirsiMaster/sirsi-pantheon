//go:build darwin

package servicedrain

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

func MaintenanceStart(pid int) (string, error) {
	k, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return "", err
	}
	if k == nil || int(k.Proc.P_pid) != pid || k.Proc.P_starttime.Sec <= 0 {
		return "", unix.ESRCH
	}
	return fmt.Sprintf("%d.%06d", k.Proc.P_starttime.Sec, k.Proc.P_starttime.Usec), nil
}

func MaintenanceEndpoint(uid, pid int) string {
	return filepath.Join("/private/tmp", "pantheon-service-"+strconv.Itoa(uid), "ai.sirsi.router.relay-"+strconv.Itoa(pid)+".sock")
}

// Absence is a named endpoint observation only, never a legacy drain proof.
func MaintenanceEndpointAbsent(uid, pid int) error {
	return maintenanceEndpointAbsentAt(MaintenanceEndpoint(uid, pid), uid)
}

func maintenanceEndpointAbsentAt(path string, uid int) error {
	parent, err := os.Lstat(filepath.Dir(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !parent.IsDir() || parent.Mode().Perm() != 0700 {
		return errors.New("service maintenance: endpoint parent is unknown")
	}
	var st unix.Stat_t
	if unix.Lstat(filepath.Dir(path), &st) != nil || st.Uid != uint32(uid) || st.Mode&07777 != 0700 {
		return errors.New("service maintenance: endpoint parent owner mismatch")
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return errors.New("service maintenance: modern or occupied endpoint; use its strict maintenance path")
	}
	return nil
}

func maintenancePeer(c *net.UnixConn) (int, int, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return 0, 0, err
	}
	var pid, uid int
	var probe error
	err = raw.Control(func(fd uintptr) {
		cred, e := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if e != nil {
			probe = e
			return
		}
		uid = int(cred.Uid)
		pid, probe = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
	})
	return pid, uid, errors.Join(err, probe)
}

type maintenanceCommand struct {
	Operation string           `json:"operation"`
	Expected  MaintenanceReady `json:"expected"`
}

type maintenanceServer struct {
	l       *net.UnixListener
	done    chan struct{}
	once    sync.Once
	parent  int
	leaf    string
	stat    unix.Stat_t
	mu      sync.Mutex
	clients map[*net.UnixConn]bool
	workers sync.WaitGroup
}

func (s *maintenanceServer) Close() error {
	var result error
	s.once.Do(func() {
		result = s.l.Close()
		<-s.done
		s.mu.Lock()
		for c := range s.clients {
			_ = c.Close()
		}
		s.mu.Unlock()
		s.workers.Wait()
		var named unix.Stat_t
		if unix.Fstatat(s.parent, s.leaf, &named, unix.AT_SYMLINK_NOFOLLOW) == nil && named.Dev == s.stat.Dev && named.Ino == s.stat.Ino && named.Mode&unix.S_IFMT == unix.S_IFSOCK {
			result = errors.Join(result, unix.Unlinkat(s.parent, s.leaf, 0))
		} else {
			result = errors.Join(result, errors.New("service maintenance: socket cleanup identity changed; retained debt"))
		}
		result = errors.Join(result, unix.Close(s.parent))
	})
	return result
}

func StartMaintenance(g *StartupMaintenance) (io.Closer, error) {
	status := g.Status()
	start, err := MaintenanceStart(os.Getpid())
	if err != nil || status.PID != os.Getpid() || status.Started != start {
		return nil, errors.New("service maintenance: endpoint must belong to this actual producer")
	}
	path := MaintenanceEndpoint(os.Geteuid(), os.Getpid())
	if err := os.Mkdir(filepath.Dir(path), 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	parent, err := unix.Open(filepath.Dir(path), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	var st unix.Stat_t
	if unix.Fstat(parent, &st) != nil || st.Uid != uint32(os.Geteuid()) || st.Mode&07777 != 0700 {
		_ = unix.Close(parent)
		return nil, errors.New("service maintenance: private endpoint directory required")
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		_ = unix.Close(parent)
		return nil, errors.New("service maintenance: occupied endpoint retained")
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		_ = unix.Close(parent)
		return nil, err
	}
	l.SetUnlinkOnClose(false)
	var socket unix.Stat_t
	var namedParent unix.Stat_t
	if unix.Fstatat(parent, filepath.Base(path), &socket, unix.AT_SYMLINK_NOFOLLOW) != nil || socket.Mode&unix.S_IFMT != unix.S_IFSOCK || socket.Uid != uint32(os.Geteuid()) || unix.Lstat(filepath.Dir(path), &namedParent) != nil || namedParent.Dev != st.Dev || namedParent.Ino != st.Ino {
		_ = l.Close()
		_ = unix.Close(parent)
		return nil, errors.New("service maintenance: endpoint publication unverified; retained cleanup debt")
	}
	s := &maintenanceServer{l: l, done: make(chan struct{}), parent: parent, leaf: filepath.Base(path), stat: socket, clients: make(map[*net.UnixConn]bool)}
	slots := make(chan struct{}, 4)
	go func() {
		defer close(s.done)
		for {
			c, err := l.AcceptUnix()
			if err != nil {
				return
			}
			select {
			case slots <- struct{}{}:
				s.mu.Lock()
				s.clients[c] = true
				s.mu.Unlock()
				s.workers.Add(1)
				go func() {
					defer func() {
						_ = c.Close()
						<-slots
						s.mu.Lock()
						delete(s.clients, c)
						s.mu.Unlock()
						s.workers.Done()
					}()
					_, uid, err := maintenancePeer(c)
					if err != nil || uid != 0 {
						return
					}
					_ = c.SetDeadline(time.Now().Add(5 * time.Second))
					if json.NewEncoder(c).Encode(g.Status()) != nil {
						return
					}
					raw, err := bufio.NewReaderSize(c, 8192).ReadSlice('\n')
					if err != nil {
						return
					}
					command, err := DecodeMaintenance[maintenanceCommand](raw)
					if err != nil || command.Operation != "activate" {
						return
					}
					after, err := g.Activate(command.Expected)
					if err == nil {
						_ = json.NewEncoder(c).Encode(after)
					}
				}()
			default:
				_ = c.Close()
			}
		}
	}()
	return s, nil
}

func ReadMaintenance(ctx context.Context, uid, pid int, started string, expected *MaintenanceReady) (MaintenanceReady, error) {
	var result MaintenanceReady
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return result, errors.New("service maintenance: native root controller required")
	}
	start, err := MaintenanceStart(pid)
	if err != nil || start != started {
		return result, errors.New("service maintenance: kernel instance changed")
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return result, errors.New("service maintenance: bounded controller context required")
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", MaintenanceEndpoint(uid, pid))
	if err != nil {
		return result, err
	}
	c := connection.(*net.UnixConn)
	defer c.Close()
	_ = c.SetDeadline(deadline)
	peerPID, peerUID, err := maintenancePeer(c)
	if err != nil || peerPID != pid || peerUID != uid {
		return result, errors.New("service maintenance: producer kernel peer mismatch")
	}
	reader := bufio.NewReaderSize(c, 8192)
	raw, err := reader.ReadSlice('\n')
	if err != nil {
		return result, err
	}
	result, err = DecodeMaintenance[MaintenanceReady](raw)
	if err != nil || result.Schema != 1 || result.PID != pid || result.Started != started || !validHex(result.Generation, 16) || result.Marker.Validate() != nil || result.Preserved < 0 ||
		!((result.Version == 1 && result.State == "maintenance_ready_not_drained") || (result.Version == 2 && result.State == "active_with_old_outcomes_unknown")) {
		return result, errors.New("service maintenance: invalid producer readiness")
	}
	if expected == nil {
		return result, nil
	}
	after := *expected
	after.State, after.Version = "active_with_old_outcomes_unknown", expected.Version+1
	if result == after {
		return result, nil // Explicit recovery of an activation whose reply was lost.
	}
	if result != *expected || result.State != "maintenance_ready_not_drained" {
		return result, errors.New("service maintenance: activation CAS changed")
	}
	if err := json.NewEncoder(c).Encode(maintenanceCommand{"activate", *expected}); err != nil {
		return result, err
	}
	raw, err = reader.ReadSlice('\n')
	if err != nil {
		return result, err
	}
	result, err = DecodeMaintenance[MaintenanceReady](raw)
	if err != nil || result != after {
		return result, errors.New("service maintenance: activation outcome unverified")
	}
	return result, nil
}
