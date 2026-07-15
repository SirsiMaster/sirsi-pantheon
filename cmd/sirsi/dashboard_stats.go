package main

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

// sampleNodeCapacityFn is injectable so tests can supply a deterministic
// NodeCapacity without touching the host (Rule A16/A21).
var sampleNodeCapacityFn = guard.SampleNodeCapacity

// collectDashRAM gathers RAM metrics from the guard package's NodeCapacity
// self-model (ADR-031-B) — the SAME source the menubar/guard surfaces use
// (real system values, PR #123). This fills total_ram/used_ram/free_ram,
// which the old sysctl+vm_stat path never set, so GET /api/stats rendered
// live zeros next to a real ram_percent (data-honesty violation).
//
// used/free/percent all derive from ONE sample so the fields stay mutually
// consistent. "free" is the guard's canonical reclaimable definition
// (free + inactive + speculative pages).
func collectDashRAM(stats map[string]interface{}) {
	nc := sampleNodeCapacityFn()
	if nc.TotalRAM <= 0 {
		// Hardware detection failed — degrade to the legacy vm_stat percent
		// rather than inventing totals. RAM fields stay honest zeros.
		collectDashRAMLegacy(stats)
		return
	}

	used := nc.TotalRAM - nc.FreeRAM
	if used < 0 {
		used = 0
	}
	pct := float64(used) / float64(nc.TotalRAM) * 100

	stats["total_ram"] = nc.TotalRAM
	stats["used_ram"] = used
	stats["free_ram"] = nc.FreeRAM
	stats["ram_percent"] = pct
	classifyDashRAMPressure(stats, pct)
}

// classifyDashRAMPressure maps a used-RAM percentage onto the dashboard's
// pressure label + icon.
func classifyDashRAMPressure(stats map[string]interface{}, pct float64) {
	switch {
	case pct > 85:
		stats["ram_pressure"] = "high"
		stats["ram_icon"] = "🔴"
	case pct > 65:
		stats["ram_pressure"] = "medium"
		stats["ram_icon"] = "🟡"
	default:
		stats["ram_pressure"] = "low"
		stats["ram_icon"] = "🟢"
	}
}

// collectDashRAMLegacy is the pre-NodeCapacity fallback (sysctl + vm_stat).
// It only produces ram_percent — total/used/free remain zero, which the UI
// treats as "unknown" rather than rendering fabricated numbers.
func collectDashRAMLegacy(stats map[string]interface{}) {
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return
	}
	var total int64
	_, _ = fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &total)
	if total == 0 {
		return
	}

	vmOut, err := exec.Command("vm_stat").Output()
	if err != nil {
		return
	}

	var pageSize int64 = 16384
	var free, active, wired int64
	for _, line := range strings.Split(string(vmOut), "\n") {
		switch {
		case strings.Contains(line, "page size of"):
			_, _ = fmt.Sscanf(line, "Mach Virtual Memory Statistics: (page size of %d bytes)", &pageSize)
		case strings.Contains(line, "Pages free"):
			free = parseVMLine(line) * pageSize
		case strings.Contains(line, "Pages active"):
			active = parseVMLine(line) * pageSize
		case strings.Contains(line, "Pages wired"):
			wired = parseVMLine(line) * pageSize
		}
	}
	_ = free

	used := active + wired
	pct := float64(used) / float64(total) * 100

	stats["ram_percent"] = pct
	classifyDashRAMPressure(stats, pct)
}

func parseVMLine(line string) int64 {
	parts := strings.Split(line, ":")
	if len(parts) < 2 {
		return 0
	}
	val := strings.TrimSpace(parts[1])
	val = strings.TrimSuffix(val, ".")
	var v int64
	_, _ = fmt.Sscanf(val, "%d", &v)
	return v
}

// collectDashGit gathers git status metrics.
func collectDashGit(stats map[string]interface{}) {
	branchOut, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return
	}
	stats["git_branch"] = strings.TrimSpace(string(branchOut))

	statusOut, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return
	}
	status := strings.TrimSpace(string(statusOut))
	var count int
	if status != "" {
		count = len(strings.Split(status, "\n"))
	}
	stats["uncommitted_files"] = count

	switch {
	case count == 0:
		stats["osiris_risk"] = "none"
		stats["osiris_icon"] = "✅"
	case count <= 5:
		stats["osiris_risk"] = "low"
		stats["osiris_icon"] = "🟢"
	case count <= 15:
		stats["osiris_risk"] = "moderate"
		stats["osiris_icon"] = "🟡"
	default:
		stats["osiris_risk"] = "high"
		stats["osiris_icon"] = "🟠"
	}

	timeOut, err := exec.Command("git", "log", "-1", "--format=%aI").Output()
	if err != nil {
		return
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(timeOut))); err == nil {
		dur := time.Since(t)
		switch {
		case dur < time.Minute:
			stats["time_since_commit"] = fmt.Sprintf("%ds", int(dur.Seconds()))
		case dur < time.Hour:
			stats["time_since_commit"] = fmt.Sprintf("%dm", int(dur.Minutes()))
		case dur < 24*time.Hour:
			stats["time_since_commit"] = fmt.Sprintf("%dh", int(dur.Hours()))
		default:
			stats["time_since_commit"] = fmt.Sprintf("%dd", int(dur.Hours()/24))
		}
	}
}

// collectDashAccelerator detects CPU/accelerator type.
func collectDashAccelerator(stats map[string]interface{}) {
	out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
	if err != nil {
		return
	}
	brand := strings.TrimSpace(string(out))
	if strings.Contains(brand, "Apple") {
		stats["primary_accelerator"] = "ANE + Metal"
		stats["accel_icon"] = "⚡"
	} else if strings.Contains(brand, "Intel") {
		stats["primary_accelerator"] = "CPU (Intel)"
		stats["accel_icon"] = "💻"
	}
}

// collectDashDeities scans running processes for active deities.
func collectDashDeities(stats map[string]interface{}) {
	out, err := exec.Command("ps", "-eo", "comm").Output()
	if err != nil {
		return
	}

	procs := strings.ToLower(string(out))
	deities := map[string]string{
		"sirsi":          "☥ Sirsi",
		"anubis":         "𓁢 Anubis",
		"pantheon-agent": "🤖 Agent",
		"guard":          "🛡 Guard",
		"maat":           "🪶 Ma'at",
		"scarab":         "🪲 Scarab",
		"thoth":          "𓁟 Thoth",
	}

	var active []string
	for binary, label := range deities {
		if strings.Contains(procs, binary) {
			active = append(active, label)
		}
	}
	stats["active_deities"] = active
	stats["deity_count"] = len(active)
}
