package rules

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
)

// TestActiveDevDepsAreCaution locks the safety rule that active-project
// dependencies and dev dependency caches surface as CAUTION, never one-click
// "safe". node_modules deletion forces a reinstall before the project builds;
// the go/npm caches cost bandwidth + time to repopulate. Regression guard for
// the `sirsi clean` default set trying to trash active-project node_modules.
func TestActiveDevDepsAreCaution(t *testing.T) {
	t.Parallel()
	cases := map[string]jackal.ScanRule{
		"node_modules":     NewNodeModulesRule(),
		"go_mod_cache":     NewGoModCacheRule(),
		"npm_global_cache": NewNpmGlobalCacheRule(),
	}
	for name, rule := range cases {
		var sev jackal.Severity
		switch r := rule.(type) {
		case *findRule:
			sev = r.effectiveSeverity()
		case *baseScanRule:
			sev = r.effectiveSeverity()
		default:
			t.Fatalf("%s: unexpected rule type %T", name, rule)
		}
		if sev != jackal.SeverityCaution {
			t.Errorf("%s severity = %q, want caution (active deps must not be one-click safe)", name, sev)
		}
	}
}
