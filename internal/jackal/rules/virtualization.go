package rules

import "github.com/SirsiMaster/sirsi-anubis/internal/jackal"

// ═══════════════════════════════════════════
// VIRTUALIZATION — Parallels, VMware, UTM
// ═══════════════════════════════════════════

// NewParallelsFullRule performs the deep Parallels remnant scan.
// This is the rule that started it all — the original inspiration for Anubis.
func NewParallelsFullRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "parallels_full",
		displayName: "Parallels Desktop Remnants",
		category:    jackal.CategoryVirtualization,
		description: "Complete Parallels remnant scan — 12+ subsystem directories, ghost apps, package receipts",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Applications (Parallels)",
			"~/Library/Preferences/com.parallels.*",
			"~/Library/Group Containers/*parallels*",
			"~/Library/Containers/*parallels*",
			"~/Library/Saved Application State/com.parallels.*",
			"~/Library/Application Scripts/*parallels*",
			"~/Library/HTTPStorages/com.parallels.*",
			"~/Library/WebKit/com.parallels.*",
			"~/Library/Caches/com.parallels.*",
			"~/Library/Logs/parallels*",
			"~/Library/Logs/Parallels*",
		},
	}
}

// NewVMwareFusionRule scans for VMware Fusion remnants.
func NewVMwareFusionRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "vmware_fusion",
		displayName: "VMware Fusion Remnants",
		category:    jackal.CategoryVirtualization,
		description: "VMware Fusion preferences, logs, and support files",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Preferences/VMware Fusion",
			"~/Library/Logs/VMware",
			"~/Library/Caches/com.vmware.*",
			"~/Library/Application Support/VMware Fusion",
		},
	}
}

// NewUTMRule scans for UTM virtual machine remnants.
func NewUTMRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "utm",
		displayName: "UTM VM Remnants",
		category:    jackal.CategoryVirtualization,
		description: "UTM virtual machine data, snapshots, and caches",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Containers/com.utmapp.UTM",
			"~/Library/Group Containers/WDNLXAD4W8.com.utmapp.UTM",
		},
	}
}

// NewVirtualBoxRule scans for VirtualBox remnants.
func NewVirtualBoxRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "virtualbox",
		displayName: "VirtualBox Remnants",
		category:    jackal.CategoryVirtualization,
		description: "VirtualBox VMs, disk images, and configuration",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.config/VirtualBox",
			"~/VirtualBox VMs",
		},
	}
}
