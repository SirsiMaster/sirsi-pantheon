package ledger

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// Registry presence values for FabricLane.Registry.
const (
	RegistryDeclared   = "declared"
	RegistryUndeclared = "undeclared"
	registryAliasPfx   = "alias:"
)

var heartbeatAge = regexp.MustCompile(`last heartbeat (\S+) ago`)

// fabricReachFor joins one lane's registry presence with the PingLane result
// ReachFromRuntime already computed. A lane absent from the registry has no
// verdict: it is a retired name, either aliased to a successor or just left over.
func fabricReachFor(agent string, reg *router.Registry, reach map[string]router.PingResult, aliases map[string]string) FabricReach {
	if reg != nil {
		if _, ok := reg.Agents[agent]; ok {
			r := reach[agent] // zero value for services/humans: no verdict
			return FabricReach{Verdict: r.Verdict, Detail: r.Detail, Registry: RegistryDeclared}
		}
	}
	if succ, ok := aliases[agent]; ok && succ != "" {
		return FabricReach{Registry: registryAliasPfx + succ}
	}
	return FabricReach{Registry: RegistryUndeclared}
}

// FabricLaneReason turns the evidence already on the board into one plain-English
// sentence for a lane that is UNROUTABLE. Other states return "": the reason is
// an explanation, never a second classification, and it never changes State.
func FabricLaneReason(state string, r FabricReach, rt *routerstore.LaneOperationalState) string {
	if state != routerstore.LaneUnroutable {
		return ""
	}
	switch {
	case r.Registry == RegistryUndeclared:
		return "retired lane (not in registry); open rows left over"
	case strings.HasPrefix(r.Registry, registryAliasPfx):
		return "retired; forwards to " + strings.TrimPrefix(r.Registry, registryAliasPfx)
	}
	switch r.Verdict {
	case router.VerdictHeld:
		if strings.HasPrefix(r.Detail, "held: "+router.HoldAttended) || strings.HasPrefix(r.Detail, "held: "+router.HoldNoGrant) {
			return "waiting for an attended session (" + heartbeatSince(r.Detail) + ")"
		}
		return strings.TrimSpace(strings.SplitN(r.Detail, " (", 2)[0])
	case router.VerdictWatchOnly:
		return "no worker configured; needs a person's session"
	case router.VerdictUnstaffed:
		return "no worker path at all; needs a person's session"
	case router.VerdictAuthRequired:
		return "the lane's consumer cannot log in; needs a person to sign in"
	case router.VerdictUnreachable:
		return "its worker stopped reporting"
	case "PROVIDER_QUOTA":
		return "provider quota reached; waiting for it to clear"
	case router.VerdictLive, router.VerdictWakeable:
		if rt != nil && rt.UnresolvedWakeFailures > 0 {
			return fmt.Sprintf("%d wake failures since the lane last acknowledged work", rt.UnresolvedWakeFailures)
		}
		return "no automated wake path reaches this lane"
	}
	return "no reachability verdict available"
}

// heartbeatSince renders "none seen for 50h" from a PingLane detail, or
// "no session seen" when the detail names no heartbeat.
func heartbeatSince(detail string) string {
	if m := heartbeatAge.FindStringSubmatch(detail); m != nil {
		if d, err := time.ParseDuration(m[1]); err == nil {
			return "none seen for " + FormatAge(d.Seconds())
		}
	}
	return "no session seen"
}

// ReadLaneReach reads reachability for the given agents from the same registry,
// thread registry and alias table the board uses. Read-only.
func ReadLaneReach(repoRoot string, agents []string, now time.Time) (map[string]FabricReach, error) {
	root := filepath.Join(repoRoot, ".agents", "idea-router")
	reg, err := router.LoadRegistry(root)
	if err != nil {
		return nil, fmt.Errorf("load registry: %w", err)
	}
	if reg == nil {
		return nil, fmt.Errorf("registry unavailable")
	}
	threads, err := router.LoadThreadRegistry(root)
	if err != nil {
		return nil, fmt.Errorf("load threads: %w", err)
	}
	var aliases map[string]string
	if f, err := dispatch.Open(repoRoot); err == nil {
		aliases, _ = f.Aliases()
		f.Close()
	}
	reach := router.ReachFromRuntime(reg, threads, now)
	out := make(map[string]FabricReach, len(agents))
	for _, a := range agents {
		out[a] = fabricReachFor(a, reg, reach, aliases)
	}
	return out, nil
}
