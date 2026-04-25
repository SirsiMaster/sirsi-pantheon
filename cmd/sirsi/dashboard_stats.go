package main

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// collectDashRAM gathers RAM metrics via sysctl + vm_stat.
func collectDashRAM(stats map[string]interface{}) {
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
