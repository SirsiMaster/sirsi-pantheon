package machineid

import (
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

// A thread record is scoped to the machine that wrote it: the reaper may only
// judge a PID against its OWN OS process table (ADR-022). The original guard
// used the hostname for that scoping — but a hostname is mutable (a laptop
// reports different names on different networks: `Mac.lan`, `MacBook-Pro-2.local`,
// `Mac.hsd1.md.comcast.net` are one machine), so records written under an old
// name were treated as a foreign host and never reaped. That was the root cause
// of a 1d16h stranded inbox: dead worker PIDs stayed `active`, so the router
// believed a live consumer was reading items that no one could read.
//
// MachineID is the stable replacement: a per-machine identifier that survives
// hostname and network changes. Reaping keys on it (SameMachine), not on the
// hostname, which is now cosmetic.

var (
	machineIDMu sync.RWMutex
	machineIDFn = defaultMachineID
)

func getMachineIDFn() func() string {
	machineIDMu.RLock()
	defer machineIDMu.RUnlock()
	return machineIDFn
}

// setMachineIDFn installs a machine-id probe (tests). A21: guarded by the mutex.
func setMachineIDFn(fn func() string) {
	machineIDMu.Lock()
	defer machineIDMu.Unlock()
	machineIDFn = fn
}

// MachineID returns a stable per-machine identifier, or "" when the platform
// exposes none. "" means "unknown" — never a match — so an absent id can never
// cause a false cross-machine reap.
func MachineID() string { return getMachineIDFn()() }

// SameMachine reports whether a thread record was written by THIS machine and is
// therefore judgeable against this host's process table. A record carrying a
// DIFFERENT machine id is provably foreign and must be left alone. A record with
// NO machine id is a pre-migration local record (the registry is per-machine
// runtime state, never shared) and is treated as local — this is what un-breaks
// the historical strand across every past hostname at once.
//
// ponytail: local-registry assumption. If a router dir is ever deliberately
// synced across machines, stamp machine ids everywhere (RegisterThread already
// does going forward) so id-less records don't linger.
func SameMachine(recordMachineID, thisMachineID string) bool {
	if recordMachineID != "" && thisMachineID != "" {
		return recordMachineID == thisMachineID
	}
	return true
}

var uuidRe = regexp.MustCompile(`[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}`)

// shapeRe matches exactly what probeMachineID can return: a darwin
// IOPlatformUUID (uuidRe's dashed form) or a linux /etc/machine-id (32 bare
// hex chars, systemd's format — never dashed).
var shapeRe = regexp.MustCompile(`^(?:[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}|[0-9A-Fa-f]{32})$`)

// LooksLikeMachineID reports whether s has the shape MachineID() produces on
// some platform, as opposed to a legacy hostname string ("Mac", "M1.local",
// "MacBook-Pro-2.local" never match).
func LooksLikeMachineID(s string) bool { return shapeRe.MatchString(s) }

// InTransition reports whether a and b differ ONLY because one has migrated
// from a legacy hostname to a machine id and the other has not yet — exactly
// one of the pair is machine-id-shaped. internal/routerstore uses this to let
// a pre-migration record or token (stamped with a hostname, back when that
// was the only identity this fabric had) be adopted by a session that has
// since started claiming a machine id instead, WITHOUT weakening the
// cross-host check itself: two DIFFERENT hostnames ("m1" vs "m5") are not in
// transition and must still be refused exactly as before — every live token
// is hostname-shaped right now, so a blanket "any legacy value matches
// anything" rule would have disabled cross-host impersonation protection for
// the fabric's current, actual regime, not just eased a future migration.
// Two machine ids, or two hostnames, are compared exactly either way; this
// only ever widens the ONE case where a single host's own identity crosses
// from one shape to the other.
func InTransition(a, b string) bool { return LooksLikeMachineID(a) != LooksLikeMachineID(b) }

var (
	machineIDCacheMu sync.Mutex
	machineIDCache   string
	machineIDDone    bool
)

// defaultMachineID probes the OS once and caches: the id never changes within a
// process, and the macOS probe shells out to ioreg.
func defaultMachineID() string {
	machineIDCacheMu.Lock()
	defer machineIDCacheMu.Unlock()
	if machineIDDone {
		return machineIDCache
	}
	machineIDDone = true
	machineIDCache = probeMachineID()
	return machineIDCache
}

func probeMachineID() string {
	switch runtime.GOOS {
	case "darwin":
		// IOPlatformUUID — stable across renames, reinstalls, and networks.
		out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
		if err != nil {
			return ""
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "IOPlatformUUID") {
				return uuidRe.FindString(line)
			}
		}
		return ""
	case "linux":
		for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
			if b, err := os.ReadFile(p); err == nil {
				if id := strings.TrimSpace(string(b)); id != "" {
					return id
				}
			}
		}
		return ""
	default:
		return "" // unknown platform → hostname-independent scoping falls back to "local"
	}
}

// GetProbe and SetProbe are the exported test-injection pair (A21: guarded by
// the same mutex as the package-private machineIDFn). internal/router re-wraps
// these under its own historical setMachineIDFn/getMachineIDFn names so its
// existing tests need no change; internal/routerstore calls these directly.
func GetProbe() func() string   { return getMachineIDFn() }
func SetProbe(fn func() string) { setMachineIDFn(fn) }
