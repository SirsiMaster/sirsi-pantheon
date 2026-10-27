//go:build darwin

package setup

import (
	"runtime/debug"
	"testing"
)

func maintenanceProfileBuildInfo(revision, modified, module string) *debug.BuildInfo {
	info := &debug.BuildInfo{Main: debug.Module{Path: module}}
	if revision != "<absent>" {
		info.Settings = append(info.Settings, debug.BuildSetting{Key: "vcs.revision", Value: revision})
	}
	if modified != "<absent>" {
		info.Settings = append(info.Settings, debug.BuildSetting{Key: "vcs.modified", Value: modified})
	}
	return info
}

func TestMaintenanceLegacySourceProfilesAndDigestBinding(t *testing.T) {
	for _, revision := range []string{maintenanceLegacySourceCurrent, maintenanceLegacySource121} {
		got, err := maintenanceLegacySourceProfile(maintenanceProfileBuildInfo(revision, "false", "github.com/SirsiMaster/sirsi-pantheon"))
		if err != nil || got != revision {
			t.Fatalf("profile %q selected as %q, error %v", revision, got, err)
		}
	}

	for _, tc := range []struct {
		name string
		info *debug.BuildInfo
	}{
		{name: "nil build info"},
		{name: "wrong main module", info: maintenanceProfileBuildInfo(maintenanceLegacySource121, "false", "example.invalid/other")},
		{name: "unknown revision", info: maintenanceProfileBuildInfo("unknown-revision", "false", "github.com/SirsiMaster/sirsi-pantheon")},
		{name: "dirty source", info: maintenanceProfileBuildInfo(maintenanceLegacySource121, "true", "github.com/SirsiMaster/sirsi-pantheon")},
		{name: "missing revision", info: maintenanceProfileBuildInfo("<absent>", "false", "github.com/SirsiMaster/sirsi-pantheon")},
		{name: "missing modified setting", info: maintenanceProfileBuildInfo(maintenanceLegacySource121, "<absent>", "github.com/SirsiMaster/sirsi-pantheon")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := maintenanceLegacySourceProfile(tc.info); err == nil || got != "" {
				t.Fatalf("invalid profile accepted as %q, error %v", got, err)
			}
		})
	}

	plan := fixtureMaintenancePlan()
	plan.LegacySource = maintenanceLegacySource121
	plan.SHA256 = maintenanceDigest(plan)
	if err := plan.validate(); err != nil {
		t.Fatalf("plan with selected 121 source profile: %v", err)
	}
	plan.LegacySource = maintenanceLegacySourceCurrent
	if err := plan.validate(); err == nil {
		t.Fatal("changing the selected source profile did not invalidate the maintenance digest")
	}
}
