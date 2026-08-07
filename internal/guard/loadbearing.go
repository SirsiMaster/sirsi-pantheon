package guard

// loadbearing.go — recognition of LOAD-BEARING Pantheon infrastructure that must
// NOT be killed or starved while the system is working (PANTHEON_RULES A32,
// ADR-040). The canonical case is the local-model broker (`sirsi gemma serve`):
// it is the Tier-0 substrate the router, the reconcile, and gemma-the-builder all
// depend on. Killing it to "reclaim RAM" breaks Pantheon — the correct response
// to an oversized broker is to RIGHT-SIZE the Tier-0 model (swap to a smaller
// one), never to SIGKILL a serving process mid-work.
//
// This inverts the old posture where the broker registered itself as *governed*
// (consent-to-be-killed under pressure): the routine memory-reclaim paths
// (FindRunaway) now recognize the broker by its pidfile and skip it. A true
// last-resort emergency is a separate, explicit decision — never routine, and
// never something an agent does while working.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// loadBearingPidFiles are the pidfiles of infrastructure that must not be killed
// by routine reclaim. Relative to ~/.sirsi. The gemma broker + its worker are the
// Tier-0 substrate; add future load-bearing services here.
var loadBearingPidFiles = []string{
	"gemma-server.pid", // the warm local-model broker (`sirsi gemma serve`)
	"gemma-worker.pid", // the router-facing gemma worker
}

// ColimaVMReservation reports the memory reservation the running Colima VM was
// LAUNCHED with, in bytes, and whether that VM is currently up. Zero/false means
// no reservation could be established from the runtime record.
//
// Two deliberate choices, both of which were review findings on the first pass:
//
//   - The record is the LIMA-GENERATED `_lima/colima/lima.yaml`, not the
//     user-editable `~/.colima/default/colima.yaml`. The latter is DESIRED
//     config: an operator can edit it to any number at any time without the
//     running hypervisor changing at all, so reading it and calling the result
//     "the VM's ceiling" reports a wish as a measurement. The generated file is
//     written by Colima at launch, alongside the pidfile, and describes the VM
//     that is actually running.
//   - Liveness is bound to `_lima/colima/vz.pid` — the Apple-Virtualization
//     driver record for this instance — not inferred from a resource number. A
//     reservation read from a config whose VM is gone is not a reservation.
//
// Known and deliberate limitation: the vz.pid record identifies the Colima
// INSTANCE, while the process actually holding the RSS is the
// `com.apple.Virtualization.VirtualMachine` XPC service, which macOS reparents
// to launchd (ppid 1). There is therefore no process-tree link from the pidfile
// to the RSS-holding PID, and recognition is by class (an Apple-Virt VM, while
// this instance is up) rather than by instance identity. That asymmetry is why
// this is used ONLY to protect and to label, never to suppress: over-protecting
// a second unrelated VM from a routine kill is safe, whereas over-HIDING one
// would silence a real hog. Suppression is what the first pass got wrong.
func ColimaVMReservation() (int64, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, false
	}
	inst := filepath.Join(home, ".colima", "_lima", "colima")

	b, err := os.ReadFile(filepath.Join(inst, "vz.pid"))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || !pidAlive(pid) {
		return 0, false
	}

	y, err := os.ReadFile(filepath.Join(inst, "lima.yaml"))
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(y), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "memory:")
		if !ok {
			continue
		}
		n, err := parseMiB(strings.TrimSpace(rest))
		if err != nil || n <= 0 {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

// parseMiB parses the "10240MiB" form Lima writes into bytes. Only the units
// Lima actually emits are accepted — an unrecognized form yields an error and
// the caller falls through to reporting, never to a silent default.
func parseMiB(s string) (int64, error) {
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "GiB"):
		s, mult = strings.TrimSuffix(s, "GiB"), 1024*1024*1024
	case strings.HasSuffix(s, "MiB"):
		s, mult = strings.TrimSuffix(s, "MiB"), 1024*1024
	case strings.HasSuffix(s, "B"):
		s = strings.TrimSuffix(s, "B")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, err
	}
	return n * mult, nil
}

// LoadBearingPIDs returns the set of live PIDs Pantheon must not kill/starve
// while working. Dead PIDs are excluded (a stale pidfile never protects a reused
// PID — the PID-alive lesson).
func LoadBearingPIDs() map[int]bool {
	out := map[int]bool{}
	home, err := os.UserHomeDir()
	if err != nil {
		return out
	}
	for _, f := range loadBearingPidFiles {
		b, err := os.ReadFile(filepath.Join(home, ".sirsi", f))
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil || pid <= 0 {
			continue
		}
		if pidAlive(pid) {
			out[pid] = true
		}
	}
	return out
}

// IsLoadBearing reports whether a PID is protected load-bearing infrastructure.
// Every kill/suspend path MUST consult this before acting on a PID.
func IsLoadBearing(pid int) bool { return LoadBearingPIDs()[pid] }

// BrokerPID returns the live PID of the local-model broker (gemma-server.pid)
// only, or 0 if the pidfile is absent or the process is not alive. Use this
// instead of LoadBearingPIDs() when the operation is specific to the BROKER
// (e.g. reading mlx_active_bytes from /health) — LoadBearingPIDs() includes
// the worker PID too, and scoping a broker-only check to the full kill-
// protection set overstates the worker's footprint by ~27 GB (Rule A35).
func BrokerPID() int {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0
	}
	b, err := os.ReadFile(filepath.Join(home, ".sirsi", "gemma-server.pid"))
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 || !pidAlive(pid) {
		return 0
	}
	return pid
}

// pidAlive reports whether pid exists (signal 0 probes without delivering).
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}
