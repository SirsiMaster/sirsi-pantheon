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
					_ = c.SetDeadline(time.Now().Add(RelayDrainTimeout + 5*time.Second))
					snapshot, err := g.Snapshot()
					if err != nil || json.NewEncoder(c).Encode(snapshot) != nil {
						return
					}
					raw, err := bufio.NewReaderSize(c, 8192).ReadSlice('\n')
					if err != nil {
						return
					}
					command, err := DecodeMaintenance[maintenanceCommand](raw)
					if err != nil {
						return
					}
					switch command.Operation {
					case "activate":
						after, opErr := g.Activate(command.Expected)
						if opErr == nil {
							_ = json.NewEncoder(c).Encode(after)
						}
					case "prepare-drain":
						ctx, cancel := context.WithTimeout(context.Background(), RelayDrainTimeout)
						after, opErr := g.PrepareDrain(ctx, command.Expected)
						cancel()
						if encodeErr := json.NewEncoder(c).Encode(after); encodeErr != nil && opErr == nil {
							_, _ = g.ResumeDrain(after)
						}
					case "resume-drain":
						after, opErr := g.ResumeDrain(command.Expected)
						if opErr == nil {
							_ = json.NewEncoder(c).Encode(after)
						}
					}
				}()
			default:
				_ = c.Close()
			}
		}
	}()
	return s, nil
}

func validMaintenanceReady(r MaintenanceReady, pid int, started string) bool {
	validState := (r.State == "maintenance_ready_not_drained" && r.Version >= 1) ||
		((r.State == "relay_active" || r.State == "active_with_old_outcomes_unknown") && r.Version >= 2) ||
		(r.State == "relay_drain_pending" && r.Version >= 3) ||
		(r.State == "relay_drain_acknowledged" && r.Version >= 4)
	return r.Schema == 1 && r.PID == pid && r.Started == started && validHex(r.Generation, 16) &&
		r.Marker.Validate() == nil && r.Preserved >= 0 && validState && r.Version >= 1
}

func maintenanceConnection(ctx context.Context, uid, pid int, started string) (*net.UnixConn, *bufio.Reader, error) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return nil, nil, errors.New("service maintenance: native root controller required")
	}
	start, err := MaintenanceStart(pid)
	if err != nil || start != started {
		return nil, nil, errors.New("service maintenance: kernel instance changed")
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, nil, errors.New("service maintenance: bounded controller context required")
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", MaintenanceEndpoint(uid, pid))
	if err != nil {
		return nil, nil, err
	}
	c := connection.(*net.UnixConn)
	_ = c.SetDeadline(deadline)
	peerPID, peerUID, err := maintenancePeer(c)
	if err != nil || peerPID != pid || peerUID != uid {
		_ = c.Close()
		return nil, nil, errors.New("service maintenance: producer kernel peer mismatch")
	}
	return c, bufio.NewReaderSize(c, 8192), nil
}

func readMaintenanceStatus(reader *bufio.Reader, pid int, started string) (MaintenanceReady, error) {
	raw, err := reader.ReadSlice('\n')
	if err != nil {
		return MaintenanceReady{}, err
	}
	result, err := DecodeMaintenance[MaintenanceReady](raw)
	if err != nil || !validMaintenanceReady(result, pid, started) {
		return result, errors.New("service maintenance: invalid producer readiness")
	}
	return result, nil
}

func ReadMaintenance(ctx context.Context, uid, pid int, started string, expected *MaintenanceReady) (MaintenanceReady, error) {
	var result MaintenanceReady
	c, reader, err := maintenanceConnection(ctx, uid, pid, started)
	if err != nil {
		return result, err
	}
	defer c.Close()
	result, err = readMaintenanceStatus(reader, pid, started)
	if err != nil {
		return result, err
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
	raw, err := reader.ReadSlice('\n')
	if err != nil {
		return result, err
	}
	result, err = DecodeMaintenance[MaintenanceReady](raw)
	if err != nil || result != after {
		return result, errors.New("service maintenance: activation outcome unverified")
	}
	return result, nil
}

func PrepareRelayDrain(ctx context.Context, uid, pid int, started string, expected MaintenanceReady) (MaintenanceReady, error) {
	var zero MaintenanceReady
	if (expected.State != "relay_active" && expected.State != "active_with_old_outcomes_unknown") || !validMaintenanceReady(expected, pid, started) {
		return zero, errors.New("service maintenance: active exact producer readiness required for drain")
	}
	c, reader, err := maintenanceConnection(ctx, uid, pid, started)
	if err != nil {
		return zero, err
	}
	defer c.Close()
	current, err := readMaintenanceStatus(reader, pid, started)
	if err != nil || current != expected {
		return current, errors.Join(errors.New("service maintenance: active generation changed before drain"), err)
	}
	if err := json.NewEncoder(c).Encode(maintenanceCommand{"prepare-drain", expected}); err != nil {
		return current, err
	}
	result, err := readMaintenanceStatus(reader, pid, started)
	if err != nil {
		return result, err
	}
	if result.State != "relay_drain_acknowledged" || result.Version != expected.Version+2 || !sameMaintenanceBinding(result, expected) {
		return result, errors.New("service maintenance: relay did not acknowledge bounded duty settlement")
	}
	return result, nil
}

func ResumeRelayAfterDrain(ctx context.Context, uid, pid int, started string, expected MaintenanceReady) (MaintenanceReady, error) {
	var zero MaintenanceReady
	if expected.State != "relay_drain_acknowledged" || !validMaintenanceReady(expected, pid, started) {
		return zero, errors.New("service maintenance: exact drain acknowledgment required to resume")
	}
	c, reader, err := maintenanceConnection(ctx, uid, pid, started)
	if err != nil {
		return zero, err
	}
	defer c.Close()
	current, err := readMaintenanceStatus(reader, pid, started)
	if err != nil {
		return current, err
	}
	if current.State == "relay_active" || current.State == "active_with_old_outcomes_unknown" {
		if current.Version == expected.Version+1 && sameMaintenanceBinding(current, expected) {
			return current, nil // Resume completed but its reply was lost.
		}
		return current, errors.New("service maintenance: producer changed after drain acknowledgment")
	}
	if current != expected {
		return current, errors.New("service maintenance: drain acknowledgment changed before resume")
	}
	if err := json.NewEncoder(c).Encode(maintenanceCommand{"resume-drain", expected}); err != nil {
		return current, err
	}
	result, err := readMaintenanceStatus(reader, pid, started)
	if err != nil {
		return result, err
	}
	if (result.State != "relay_active" && result.State != "active_with_old_outcomes_unknown") || result.Version != expected.Version+1 || !sameMaintenanceBinding(result, expected) {
		return result, errors.New("service maintenance: relay resume outcome unverified")
	}
	return result, nil
}

func sameMaintenanceBinding(a, b MaintenanceReady) bool {
	a.State, b.State = "", ""
	a.Version, b.Version = 0, 0
	return a == b
}
