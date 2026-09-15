package router

// MachineID and SameMachine moved to internal/machineid (a dependency-free
// leaf package) so internal/routerstore — which internal/router already
// imports, so routerstore importing router directly would cycle — can use the
// exact same stable-machine-identity primitive instead of internal/routerstore
// keying its own token/session/thread-authority checks on the mutable
// os.Hostname() (2026-09-15: an M1 and an M5 fought over hostnames all night;
// this package already solved that class of bug in 2026-07 for the LOCAL
// reaper, PR #223 — routerstore just never adopted it). These two names, plus
// the private get/setMachineIDFn pair, are kept so internal/router's existing
// call sites and tests need no change.
import "github.com/SirsiMaster/sirsi-pantheon/internal/machineid"

// MachineID returns a stable per-machine identifier, or "" when the platform
// exposes none.
func MachineID() string { return machineid.MachineID() }

// SameMachine reports whether a thread record was written by THIS machine.
func SameMachine(recordMachineID, thisMachineID string) bool {
	return machineid.SameMachine(recordMachineID, thisMachineID)
}

func getMachineIDFn() func() string   { return machineid.GetProbe() }
func setMachineIDFn(fn func() string) { machineid.SetProbe(fn) }
