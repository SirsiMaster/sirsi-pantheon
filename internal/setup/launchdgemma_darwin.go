//go:build darwin

package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/servicedrain"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const gemmaNativeProgram = "/Applications/Pantheon.app/Contents/MacOS/sirsi"

func gemmaPrivateDir() (string, error) {
	home, e := os.UserHomeDir()
	if e != nil {
		return "", e
	}
	dir := filepath.Join(home, ".sirsi", "gemma-native-lifecycle")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	real, e := filepath.EvalSymlinks(dir)
	if e != nil || real != dir {
		return "", errors.New("Gemma lifecycle: symlinked debt directory refused")
	}
	st, e := os.Lstat(dir)
	if e != nil {
		return "", e
	}
	u, ok := st.Sys().(*syscall.Stat_t)
	if !ok || !st.IsDir() || st.Mode().Perm() != 0700 || u.Uid != uint32(os.Getuid()) {
		return "", errors.New("Gemma lifecycle: private owned directory required")
	}
	return dir, nil
}
func gemmaPrivateRead(path string, v any) error {
	fd, e := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if errors.Is(e, unix.ENOENT) {
		return ErrGemmaNoDebt
	}
	if e != nil {
		return e
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	s, e := f.Stat()
	if e != nil {
		return e
	}
	u, ok := s.Sys().(*syscall.Stat_t)
	if !ok || !s.Mode().IsRegular() || s.Mode().Perm() != 0600 || u.Uid != uint32(os.Getuid()) || u.Nlink != 1 || s.Size() > 32768 {
		return errors.New("Gemma lifecycle: unsafe private record")
	}
	return json.NewDecoder(f).Decode(v)
}
func gemmaPrivateWrite(path string, v any) error {
	if s, e := os.Lstat(path); e == nil {
		u, ok := s.Sys().(*syscall.Stat_t)
		if !ok || !s.Mode().IsRegular() || s.Mode().Perm() != 0600 || u.Uid != uint32(os.Getuid()) || u.Nlink != 1 {
			return errors.New("Gemma lifecycle: substituted private record")
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".worker-state-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		e = json.NewEncoder(f).Encode(v)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if e = os.Rename(name, path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func gemmaAckPath(dir, thread string) (string, error) {
	if thread == "" || len(thread) > 160 || strings.ContainsAny(thread, "/\\.") {
		return "", errors.New("Gemma lifecycle: invalid exact worker thread")
	}
	return filepath.Join(dir, "pause-"+thread+".json"), nil
}
func inspectGemmaNative(ctx context.Context, thread, machine string) (GemmaWorkerPlan, error) {
	var p GemmaWorkerPlan
	if os.Getuid() != 501 || os.Geteuid() != 501 {
		return p, errors.New("Gemma lifecycle: exact native uid501 controller required")
	}
	home, e := os.UserHomeDir()
	if e != nil || home != "/Users/thekryptodragon" {
		return p, errors.New("Gemma lifecycle: native profile home mismatch")
	}
	plist := filepath.Join(home, "Library", "LaunchAgents", GemmaNativeWorkerLabel+".plist")
	raw, hash, e := serviceArtifactPolicy(plist, true, func(fd int) error {
		var st unix.Stat_t
		if err := unix.Fstat(fd, &st); err != nil {
			return err
		}
		if st.Uid != 501 || st.Gid != 20 || st.Mode&07777 != 0644 {
			return errors.New("Gemma lifecycle: selected plist owner/mode mismatch")
		}
		return nil
	})
	if e != nil {
		return p, e
	}
	out, e := serviceReadInput(ctx, raw, "/usr/bin/plutil", "-convert", "json", "-o", "-", "-")
	if e != nil {
		return p, e
	}
	var c struct {
		Label                string
		Program              string
		ProgramArguments     []string
		WorkingDirectory     string
		KeepAlive            struct{ SuccessfulExit *bool }
		RunAtLoad            bool
		ProcessType          string
		EnvironmentVariables map[string]string
	}
	if e = json.Unmarshal(out, &c); e != nil {
		return p, e
	}
	want := []string{gemmaNativeProgram, "gemma", "worker", "--profile", "m5", "--inference-profile", home + "/.sirsi/profiles/gemma-m5-r4-20261013.json"}
	if c.Label != GemmaNativeWorkerLabel || (c.Program != "" && c.Program != gemmaNativeProgram) || strings.Join(c.ProgramArguments, "\x00") != strings.Join(want, "\x00") || c.WorkingDirectory != home+"/Development/sirsi-pantheon" || c.KeepAlive.SuccessfulExit == nil || *c.KeepAlive.SuccessfulExit || !c.RunAtLoad || c.ProcessType != "Background" {
		return p, errors.New("Gemma lifecycle: plist outside exact worker-only profile")
	}
	for k := range c.EnvironmentVariables {
		if k != "SIRSI_AGENT_ID" && k != "SIRSI_ROUTER_AGENT" && k != "SIRSI_ROUTER_URL" {
			return p, errors.New("Gemma lifecycle: unexpected worker environment key")
		}
	}
	if c.EnvironmentVariables["SIRSI_AGENT_ID"] != "gemma-pantheon" || c.EnvironmentVariables["SIRSI_ROUTER_AGENT"] != "gemma-pantheon" {
		return p, errors.New("Gemma lifecycle: worker actor mismatch")
	}
	// Generic serviceLocation deliberately stays closed; this one typed adapter
	// alone observes the additional label and never invokes generic bootout.
	target := "gui/501/" + GemmaNativeWorkerLabel
	state, e := serviceRead(ctx, "/bin/launchctl", "print", target)
	if e != nil {
		return p, e
	}
	pid := 0
	for _, line := range strings.Split(string(state), "\n") {
		f := strings.Fields(line)
		if len(f) == 3 && f[0] == "pid" && f[1] == "=" {
			if pid != 0 {
				return p, errors.New("Gemma lifecycle: ambiguous selected job")
			}
			pid, e = strconv.Atoi(f[2])
			if e != nil {
				return p, e
			}
		}
	}
	start, e := servicedrain.MaintenanceStart(pid)
	if e != nil {
		return p, e
	}
	k, e := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if e != nil || k == nil || k.Proc.P_ppid != 1 {
		return p, errors.New("Gemma lifecycle: selected parent mismatch")
	}
	cd, e := maintenanceSignedImage(gemmaNativeProgram, pid)
	if e != nil {
		return p, e
	}
	programHash, e := serviceFileHash(gemmaNativeProgram)
	if e != nil {
		return p, e
	}
	started, e := serviceRead(ctx, "/bin/ps", "-p", strconv.Itoa(pid), "-o", "lstart=")
	if e != nil {
		return p, e
	}
	if again, e := servicedrain.MaintenanceStart(pid); e != nil || again != start {
		return p, errors.New("Gemma lifecycle: selected PID reused")
	}
	if again, e := serviceFileHash(plist); e != nil || again != hash {
		return p, errors.New("Gemma lifecycle: plist substituted")
	}
	p = GemmaWorkerPlan{Instance: ServiceInstance{Label: GemmaNativeWorkerLabel, Target: target, Plist: plist, PlistSHA256: hash, Program: gemmaNativeProgram, ProgramSHA256: programHash, PID: pid, Started: strings.TrimSpace(string(started))}, Thread: thread, Machine: machine, KernelStart: start, CDHash: cd, Parent: 1}
	p.Digest = gemmaPlanDigest(p)
	return p, nil
}

// Worker calls only after canonical CLOSED readback and its joined runOnce.
func WriteGemmaWorkerPauseAck(ctx context.Context, thread, machine string) error {
	p, e := inspectGemmaNative(ctx, thread, machine)
	if e != nil {
		return e
	}
	if p.Instance.PID != os.Getpid() {
		return errors.New("Gemma lifecycle: acknowledgment must be written by actual selected worker")
	}
	snapshot, e := serviceRead(ctx, "/bin/ps", "-axo", "pid=,ppid=")
	if e != nil {
		return e
	}
	if e = rootRelayChildrenClear(snapshot, os.Getpid()); e != nil {
		return e
	}
	dir, e := gemmaPrivateDir()
	if e != nil {
		return e
	}
	path, e := gemmaAckPath(dir, thread)
	if e != nil {
		return e
	}
	return gemmaPrivateWrite(path, GemmaWorkerPauseAck{Plan: p, At: time.Now().UTC()})
}
func NativeGemmaWorkerLifecycle(ctx context.Context, action, thread, owner, machine string, expected *GemmaWorkerPlan, authorize func(context.Context, string) error, barrier func(context.Context, routerstore.LaneClaimBarrierRequest) (routerstore.LaneClaimBarrierResult, error)) (GemmaWorkerPlan, error) {
	var p GemmaWorkerPlan
	if authorize == nil || barrier == nil {
		return p, errors.New("Gemma lifecycle: canonical registered SSA authority required")
	}
	if e := authorize(ctx, GemmaNativeWorkerLabel); e != nil {
		return p, e
	}
	if action == "plan" {
		p, e := inspectGemmaNative(ctx, thread, machine)
		if e != nil {
			return p, e
		}
		return p, validateGemmaWorkerPlan(p)
	}
	dir, e := gemmaPrivateDir()
	if e != nil {
		return p, e
	}
	debt := filepath.Join(dir, "controller.json")
	d := gemmaWorkerOperations{inspect: func(c context.Context, t string) (GemmaWorkerPlan, error) { return inspectGemmaNative(c, t, machine) }, barrier: barrier, now: time.Now, save: func(s gemmaWorkerDebt) error { return gemmaPrivateWrite(debt, s) }, load: func() (gemmaWorkerDebt, error) { var s gemmaWorkerDebt; e := gemmaPrivateRead(debt, &s); return s, e }, ack: func(t string) (GemmaWorkerPauseAck, error) {
		var a GemmaWorkerPauseAck
		path, e := gemmaAckPath(dir, t)
		if e == nil {
			e = gemmaPrivateRead(path, &a)
		}
		return a, e
	}, joined: func(p GemmaWorkerPlan) error {
		_, e := servicedrain.MaintenanceStart(p.Instance.PID)
		if !errors.Is(e, unix.ESRCH) {
			return errors.New("selected PID not proven absent (including reuse or probe outage)")
		}
		h, e := serviceFileHash(p.Instance.Plist)
		if e != nil || h != p.Instance.PlistSHA256 {
			return errors.New("plist changed before restart")
		}
		h, e = serviceFileHash(p.Instance.Program)
		if e != nil || h != p.Instance.ProgramSHA256 {
			return errors.New("program changed before restart")
		}
		cd, e := maintenanceSignedImage(p.Instance.Program, 0)
		if e != nil || cd != p.CDHash {
			return errors.New("installed signed image changed before restart")
		}
		return nil
	}, start: func(c context.Context) error {
		_, e := serviceRead(c, "/bin/launchctl", "kickstart", "gui/501/"+GemmaNativeWorkerLabel)
		return e
	}, wait: func(c context.Context) error {
		select {
		case <-c.Done():
			return c.Err()
		case <-time.After(200 * time.Millisecond):
			return nil
		}
	}}
	e = withServiceLock(filepath.Join(dir, "worker"), func() error {
		switch action {
		case "apply":
			if expected == nil {
				return errors.New("Gemma lifecycle: exact plan required")
			}
			return applyGemmaWorker(ctx, d, *expected, owner)
		case "recover":
			return recoverGemmaWorker(ctx, d, owner)
		default:
			return fmt.Errorf("Gemma lifecycle: unsupported typed action")
		}
	})
	return p, e
}
