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
	profiles := []string{
		maintenanceLegacySourceCurrent,
		maintenanceLegacySource121,
		maintenanceLegacySource129,
		maintenanceLegacySource137,
	}
	for _, revision := range profiles {
		t.Run(revision, func(t *testing.T) {
			got, err := maintenanceLegacySourceProfile(maintenanceProfileBuildInfo(revision, "false", "github.com/SirsiMaster/sirsi-pantheon"))
			if err != nil || got != revision {
				t.Fatalf("profile %q selected as %q, error %v", revision, got, err)
			}
		})
	}

	for _, revision := range []string{maintenanceLegacySource129, maintenanceLegacySource137} {
		for _, tc := range []struct {
			name     string
			modified string
			settings []debug.BuildSetting
		}{
			{name: "dirty source", modified: "true"},
			{name: "missing revision", modified: "false", settings: []debug.BuildSetting{
				{Key: "vcs.modified", Value: "false"},
			}},
			{name: "missing modified setting", modified: "<absent>"},
			{name: "duplicate revision settings", modified: "false", settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: revision},
				{Key: "vcs.revision", Value: revision},
				{Key: "vcs.modified", Value: "false"},
			}},
			{name: "duplicate modified settings", modified: "false", settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: revision},
				{Key: "vcs.modified", Value: "false"},
				{Key: "vcs.modified", Value: "false"},
			}},
		} {
			t.Run(revision+"/"+tc.name, func(t *testing.T) {
				info := maintenanceProfileBuildInfo(revision, tc.modified, "github.com/SirsiMaster/sirsi-pantheon")
				if tc.settings != nil {
					info.Settings = tc.settings
				}
				if got, err := maintenanceLegacySourceProfile(info); err == nil || got != "" {
					t.Fatalf("invalid profile accepted as %q, error %v", got, err)
				}
			})
		}
	}

	for _, tc := range []struct {
		name string
		info *debug.BuildInfo
	}{
		{name: "nil build info"},
		{name: "wrong main module", info: maintenanceProfileBuildInfo("22944b2458f377a9f3e5e10c63c3442cb3ea83c3", "false", "example.invalid/other")},
		{name: "wrong main module 129", info: maintenanceProfileBuildInfo(maintenanceLegacySource129, "false", "example.invalid/other")},
		{name: "wrong main module 137", info: maintenanceProfileBuildInfo(maintenanceLegacySource137, "false", "example.invalid/other")},
		{name: "unknown revision", info: maintenanceProfileBuildInfo("unknown-revision", "false", "github.com/SirsiMaster/sirsi-pantheon")},
		{name: "clean unknown revision with valid commit shape", info: maintenanceProfileBuildInfo("7f17065c0cd934009eb271526efc0405f346190d", "false", "github.com/SirsiMaster/sirsi-pantheon")},
		{name: "version label 129", info: maintenanceProfileBuildInfo("v0.24.129", "false", "github.com/SirsiMaster/sirsi-pantheon")},
		{name: "version label 137", info: maintenanceProfileBuildInfo("v0.24.137", "false", "github.com/SirsiMaster/sirsi-pantheon")},
		{name: "nearby 129 revision", info: maintenanceProfileBuildInfo("0fd8463011ee2ebae12494895669b0e7b20b9dde", "false", "github.com/SirsiMaster/sirsi-pantheon")},
		{name: "nearby 137 revision", info: maintenanceProfileBuildInfo("cb59d56f849235eb80f5e4b10514fc89467fb8ae", "false", "github.com/SirsiMaster/sirsi-pantheon")},
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

	for _, revision := range profiles {
		plan := fixtureMaintenancePlan()
		plan.LegacySource = revision
		plan.SHA256 = maintenanceDigest(plan)
		if err := plan.validate(); err != nil {
			t.Fatalf("plan with selected %q source profile: %v", revision, err)
		}
		for _, replacement := range profiles {
			if replacement == revision {
				continue
			}
			substituted := plan
			substituted.LegacySource = replacement
			if err := substituted.validate(); err == nil {
				t.Fatalf("substituting source %q for %q did not invalidate the maintenance digest", replacement, revision)
			}
		}
	}
}
