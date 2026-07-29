package main

// ADR-046 S2 — supervision moves into Go.
//
// The pre-S2 launchd path called syscall.Exec, replacing this process with
// CPython. After that instant there was no Go left on the machine: it could not
// wait for a port, guarantee a clean exit, observe the worker, or enforce
// anything. Three of the four load-bearing concerns in the serving path lived
// inside a GENERATED PYTHON STRING, which is the actual defect the port exists
// to fix — only decode is genuinely Python's job.
//
// ADR-046's boundary decision, taken on measurement rather than preference:
// MLX stays behind a PROCESS boundary and Go does not cgo-link libmlx. The
// crash that precipitated this work was 87,424 levels of C++ recursion inside
// mlx::core::detail::compile_dfs — a Go host calling the same C++ inherits the
// same unbounded recursion, on a measured 8.0 MB cgo stack (half of the 16 MB
// thread that already overflowed), and runtime.LockOSThread does not buy a
// bigger one. So the supervisor supervises; it never links the library.

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

// gemmaWorkerPidPath is the pid of the process actually SERVING.
//
// The pid-file contract (ADR-046): gemma-server.pid names the SUPERVISOR. That
// distinction broke a reader — a governed conduit step reads the pid file every
// 15 minutes and inspects that process's argv for --prompt-cache-bytes, which
// the worker carries and the supervisor does not, so it would have flipped to a
// permanent false negative and bounced a healthy broker every run. ADR-045 said
// "the supervised pid IS the serving process" and treated that as a fact about
// --stop; it was load-bearing for READERS too and nobody wrote it down, so it
// survived only as an implicit property and broke silently when the
// implementation changed.
func gemmaWorkerPidPath(home string) string {
	return filepath.Join(home, ".sirsi/gemma-worker.pid")
}

// gemmaWaitPortFree blocks until the port can be bound, or gives up.
//
// Ported from the generated Python shim, where it existed because a respawn
// that bound while the dying predecessor still held the port got Errno 48
// INSIDE mlx_lm.server, and the failed process wedged as a zombie with the
// model loaded (12.9 GB) and its main thread dead. This is supervision, not
// inference, so it belongs here.
//
// The only competitor for the port is our own dying predecessor, which never
// re-binds, so the probe→release→worker-bind window is benign.
func gemmaWaitPortFree(host string, port int, timeout time.Duration) error {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	deadline := time.Now().Add(timeout)
	for {
		l, err := net.Listen("tcp", addr)
		if err == nil {
			_ = l.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("port %d still held after %s — refusing to start a worker "+
				"that would die on bind", port, timeout)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// gemmaReadPid reads a pid file and reports whether that process is alive.
//
// Liveness is checked with signal 0 rather than trusting the file, because a
// pid file outlives the process it names and a stale one reads exactly like a
// healthy one — the same green-surface-over-a-dead-thing shape that has cost us
// real outages.
func gemmaReadPid(path string) (pid int, alive bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err = strconv.Atoi(string(trimSpaceBytes(b)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return pid, false
	}
	// syscall.Signal(0) is the liveness probe — it checks the process exists
	// without delivering anything. NOT Signal(nil): on darwin that returns an
	// error for a perfectly healthy process, so a LIVE broker reads as dead.
	// Caught by the test rather than in production, which is the point of it.
	return pid, p.Signal(syscall.Signal(0)) == nil
}

func trimSpaceBytes(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && (b[i] == ' ' || b[i] == '\n' || b[i] == '\r' || b[i] == '\t') {
		i++
	}
	for j > i && (b[j-1] == ' ' || b[j-1] == '\n' || b[j-1] == '\r' || b[j-1] == '\t') {
		j--
	}
	return b[i:j]
}

// gemmaSuperviseWorker runs the MLX worker as a child and stays resident as the
// launchd-supervised parent (ADR-046 S2).
//
// Contract, stated because ADR-045's equivalent was implicit and broke a reader:
//   - gemma-server.pid  = THIS process, the supervisor.
//   - gemma-worker.pid  = the child, the process actually serving.
//   - --stop kills the process group, so it still stops the whole tree.
//
// The supervisor must die WITH its child, never before it. ADR-046 named this
// explicitly: if the supervisor exits and leaves an orphan worker holding the
// model and the port, we have traded one failure mode for a worse one — an
// unsupervised 12 GB process nothing knows how to stop. So every exit path here
// terminates the group.
func gemmaSuperviseWorker(home, pyBin string, args []string) error {
	logf, err := os.Create(gemmaServerLogPath(home))
	if err != nil {
		return fmt.Errorf("opening server log: %w", err)
	}
	defer logf.Close()

	c := exec.Command(pyBin, args...)
	c.Stdout = logf
	c.Stderr = logf
	// Own process group: one signal reaches supervisor and worker together.
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := c.Start(); err != nil {
		return fmt.Errorf("starting mlx worker: %w", err)
	}

	supPid, workerPid := os.Getpid(), c.Process.Pid
	_ = os.WriteFile(gemmaPidPath(home), []byte(strconv.Itoa(supPid)), 0o644)
	_ = os.WriteFile(gemmaWorkerPidPath(home), []byte(strconv.Itoa(workerPid)), 0o644)
	_ = os.WriteFile(gemmaPortPath(home), []byte(strconv.Itoa(gemmaServePort)), 0o644)

	// Hapi governs the WORKER — it is the process that can balloon toward Jetsam,
	// and killing the supervisor would leave the ballooning process running.
	_ = guard.HapiRegisterGoverned(workerPid, "gemma-broker")

	// Forward termination to the whole group so launchd's stop is not merely a
	// stop of the supervisor.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		s := <-sigs
		_ = syscall.Kill(-workerPid, s.(syscall.Signal))
	}()

	err = c.Wait()
	_ = os.Remove(gemmaWorkerPidPath(home))

	// Reap anything the worker left in the group before we exit, so launchd's
	// restart never races an orphan still holding the port or the model.
	_ = syscall.Kill(-workerPid, syscall.SIGKILL)

	if err != nil {
		// Surface the worker's own failure rather than masking it as a supervisor
		// error: an MLX stack overflow must read as an MLX stack overflow.
		return fmt.Errorf("mlx worker exited: %w", err)
	}
	return nil
}
