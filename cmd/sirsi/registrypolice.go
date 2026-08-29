package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// runRegistryPoliceDuty is the resident Go-native replacement for
// registry-police.sh. It preserves the old duty's bounded discovery, read-only
// process ledger, per-agent stranded check, and one-advisory-per-day policy.
// It neither launches watchers nor kills, renices, or steers processes.
func runRegistryPoliceDuty(routerRoot, repoRoot string) error {
	reg, loadErr := router.LoadRegistry(routerRoot)
	if loadErr != nil {
		return fmt.Errorf("registry police: load registry: %w", loadErr)
	}
	if _, err := reapDeadPIDThreads(routerRoot); err != nil {
		return fmt.Errorf("registry police: OS-truth sweep incomplete: %w", err)
	}
	_, actions, _, reconcileErr := reconcileDiscoveredProcs(routerRoot, reg, enumerateAgentProcs(localSurfaces(reg)))
	if reconcileErr != nil {
		return fmt.Errorf("registry police: discover: %w", reconcileErr)
	}
	if _, err := runThreadScout(routerRoot); err != nil {
		return fmt.Errorf("registry police: scout: %w", err)
	}
	stranded, strandedErr := router.RegistryPoliceStranded(routerRoot, router.DefaultLaunchctlChecker)
	if strandedErr != nil {
		return strandedErr
	}
	unmappable := 0
	for _, action := range actions {
		if action.Outcome == router.OutcomeUnmappable {
			unmappable++
		}
	}
	if unmappable+len(stranded) == 0 {
		return nil
	}

	now := time.Now().UTC()
	flag := filepath.Join(routerRoot, "police", ".last-alarm-"+now.Format("20060102"))
	if _, err := os.Stat(flag); err == nil {
		return nil // one advisory per UTC day, matching the legacy duty.
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("registry police: inspect advisory flag: %w", err)
	}
	body := registryPoliceAdvisory(now, unmappable, stranded)
	facade, openErr := dispatch.Open(repoRoot)
	if openErr != nil {
		return fmt.Errorf("registry police: open guarded dispatch: %w", openErr)
	}
	defer func() { _ = facade.Close() }()
	// Preserve the legacy CLI send contract: registry-police advisories are
	// ordinary addressed work items, not a newly retyped decision channel.
	if _, err := facade.Send("registry-police", "claude-pantheon", fmt.Sprintf("Registry police: %d A27 accountability issue(s)", unmappable+len(stranded)), "", body); err != nil {
		return fmt.Errorf("registry police: send advisory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(flag), 0o755); err != nil {
		return fmt.Errorf("registry police: create advisory directory: %w", err)
	}
	if err := os.WriteFile(flag, []byte(now.Format(time.RFC3339)+"\n"), 0o644); err != nil {
		return fmt.Errorf("registry police: record advisory: %w", err)
	}
	return nil
}

func registryPoliceAdvisory(now time.Time, unmappable int, stranded []router.StrandedAgent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Registry Police Alarm — %s\n\n", now.Format(time.RFC3339))
	b.WriteString("A27 two-tier accountability check found issues:\n\n")
	fmt.Fprintf(&b, "- **%d unmappable agent session(s)** — running agents outside a known repo have no agent identity or inbox. Register an explicit repo identity or relaunch from that repo.\n", unmappable)
	fmt.Fprintf(&b, "- **%d registered-but-not-looping agent(s)** — have open inbox items but zero armed watchers across their live threads and declared wake surfaces.\n", len(stranded))
	for _, agent := range stranded {
		fmt.Fprintf(&b, "  - `%s`: %d open item(s)\n", agent.AgentID, agent.OpenItems)
	}
	b.WriteString("\nRun `sirsi thread discover` and `sirsi thread list` to inspect. Registry police is read-only/advisory; no process was killed, reniced, or steered.\n")
	return b.String()
}
