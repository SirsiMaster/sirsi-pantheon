package main

import (
	"fmt"
	"sort"

	"github.com/SirsiMaster/sirsi-pantheon/internal/namespec"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// ADR-072 §5: the migration map. The doctor only SUGGESTS these; renaming a live
// lane is a separate, crash-safe verb (thread rename), never a doctor side effect.
var nameMigrationMap = map[string]string{
	"ra":                   "ra-router-m1",
	"claude-home":          "claude-home-m1",
	"sirsi-software-admin": "sirsi-governance-m5",
	"codex-apollo":         "codex-apollo-m5",
	"hermes":               "hermes-hermes-m1",
	"hermes-m5":            "hermes-hermes-m5",
	"codex-pantheon":       "codex-pantheon-m1",
}

// nonconformantNames returns the registered ids that do not parse as
// <agent>-<project>-<machine>[-<task>], sorted. Reads ids only (A35: it does not
// check the origin-pinned known-sets, so a conformant id is well-formed, not authorized).
func nonconformantNames(reg *router.Registry) []string {
	var out []string
	for id := range reg.Agents {
		if _, err := namespec.Parse(id); err != nil {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// reportNameGrammar prints the ADR-072 conformance line. Informational, not an
// issue: the migration (thread rename) is not built yet, so flagging it red would
// only add noise to every doctor run.
func reportNameGrammar(reg *router.Registry) {
	if reg == nil {
		return
	}
	bad := nonconformantNames(reg)
	if len(bad) == 0 {
		fmt.Printf("ℹ ADR-072: all %d registered ids are in <agent>-<project>-<machine>[-<task>] form.\n\n", len(reg.Agents))
		return
	}
	fmt.Printf("ℹ ADR-072: %d of %d registered ids are not in <agent>-<project>-<machine>[-<task>] form (report-only; rename verb pending):\n", len(bad), len(reg.Agents))
	for _, id := range bad {
		if to, ok := nameMigrationMap[id]; ok {
			fmt.Printf("    %s → %s (migration map)\n", id, to)
		} else {
			fmt.Printf("    %s (no migration-map entry yet)\n", id)
		}
	}
	fmt.Println()
}
