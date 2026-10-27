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
	for _, revision := range []string{
		"ba2a07bb464bbfd68808254a7d489769f122149b",
		"22944b2458f377a9f3e5e10c63c3442cb3ea83c3",
	} {
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
		{name: "wrong main module", info: maintenanceProfileBuildInfo("22944b2458f377a9f3e5e10c63c3442cb3ea83c3", "false", "example.invalid/other")},
		{name: "unknown revision", info: maintenanceProfileBuildInfo("unknown-revision", "false", "github.com/SirsiMaster/sirsi-pantheon")},
		{name: "clean unknown revision with valid commit shape", info: maintenanceProfileBuildInfo("7f17065c0cd934009eb271526efc0405f346190d", "false", "github.com/SirsiMaster/sirsi-pantheon")},
		{name: "dirty source", info: maintenanceProfileBuildInfo("22944b2458f377a9f3e5e10c63c3442cb3ea83c3", "true", "github.com/SirsiMaster/sirsi-pantheon")},
		{name: "missing revision", info: maintenanceProfileBuildInfo("<absent>", "false", "github.com/SirsiMaster/sirsi-pantheon")},
		{name: "missing modified setting", info: maintenanceProfileBuildInfo("22944b2458f377a9f3e5e10c63c3442cb3ea83c3", "<absent>", "github.com/SirsiMaster/sirsi-pantheon")},
		{name: "duplicate revision settings", info: &debug.BuildInfo{Main: debug.Module{Path: "github.com/SirsiMaster/sirsi-pantheon"}, Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "ba2a07bb464bbfd68808254a7d489769f122149b"},
			{Key: "vcs.revision", Value: "22944b2458f377a9f3e5e10c63c3442cb3ea83c3"},
			{Key: "vcs.modified", Value: "false"},
		}}},
		{name: "duplicate modified settings", info: &debug.BuildInfo{Main: debug.Module{Path: "github.com/SirsiMaster/sirsi-pantheon"}, Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "22944b2458f377a9f3e5e10c63c3442cb3ea83c3"},
			{Key: "vcs.modified", Value: "false"},
			{Key: "vcs.modified", Value: "false"},
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := maintenanceLegacySourceProfile(tc.info); err == nil || got != "" {
				t.Fatalf("invalid profile accepted as %q, error %v", got, err)
			}
		})
	}

	plan := fixtureMaintenancePlan()
	plan.LegacySource = "22944b2458f377a9f3e5e10c63c3442cb3ea83c3"
	plan.SHA256 = maintenanceDigest(plan)
	if err := plan.validate(); err != nil {
		t.Fatalf("plan with selected 121 source profile: %v", err)
	}
	plan.LegacySource = "ba2a07bb464bbfd68808254a7d489769f122149b"
	if err := plan.validate(); err == nil {
		t.Fatal("changing the selected source profile did not invalidate the maintenance digest")
	}
}
