// Package guard provides RAM management and process auditing.
// Rule A1: NEVER kill a process without explicit user confirmation.
package guard

import (
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// ProcessInfo represents a running process with memory usage.
type ProcessInfo struct {
	PID        int
	Name       string
	Command    string
	RSS        int64 // Resident Set Size in bytes
	VSZ        int64 // Virtual memory size in bytes
	User       string
	CPUPercent float64
	Group      string // Grouping label (e.g., "node", "docker", "lsp")
}

// ProcessGroup aggregates processes by type.
type ProcessGroup struct {
	Name       string
	Processes  []ProcessInfo
	TotalRSS   int64
	TotalCount int
}

// AuditResult contains the full RAM audit.
type AuditResult struct {
	TotalRAM     int64
	UsedRAM      int64
	FreeRAM      int64
	Groups       []ProcessGroup
	Orphans      []ProcessInfo
	TotalOrphans int
	OrphanRSS    int64
}

// orphanPatterns defines known orphan process signatures.
var orphanPatterns = map[string]string{
	// Node.js orphans
	"node":                  "node",
	"npm":                   "node",
	"npx":                   "node",
	"tsx":                   "node",
	"ts-node":               "node",
	"esbuild":               "node",
	"vite":                  "node",
	"next-server":           "node",
	"webpack":               "node",
	"electron":              "electron",
	"electron helper":       "electron",
	"electron helper (gpu)": "electron",

	// LSP servers
	"typescript-language-server": "lsp",
	"tsserver":                   "lsp",
	"gopls":                      "lsp",
	"rust-analyzer":              "lsp",
	"pyright":                    "lsp",
	"pylsp":                      "lsp",
	"clangd":                     "lsp",
	"lua-language-server":        "lsp",
	"sourcekit-lsp":              "lsp",
	"vscode-json-language":       "lsp",
	"vscode-css-language":        "lsp",
	"vscode-html-language":       "lsp",

	// Docker
	"com.docker.backend":    "docker",
	"com.docker.extensions": "docker",
	"docker-compose":        "docker",
	"vpnkit":                "docker",

	// Build tools
	"gradle":  "build",
	"gradlew": "build",
	"maven":   "build",
	"cargo":   "build",
	"rustc":   "build",
	"cc1plus": "build",
	"clang":   "build",

	// AI/ML
	"ollama":       "ai",
	"ollama serve": "ai",
	"mlx_lm":       "ai",
}

// Audit scans all running processes and groups them by type.
func Audit() (*AuditResult, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, fmt.Errorf("guard: unsupported platform %s", runtime.GOOS)
	}

	result := &AuditResult{}

	// Get total/used/free RAM
	if err := getMemoryInfo(result); err != nil {
		return nil, fmt.Errorf("guard: memory info: %w", err)
	}

	// Get process list
	processes, err := getProcessList()
	if err != nil {
		return nil, fmt.Errorf("guard: process list: %w", err)
	}

	// Group processes
	groupMap := make(map[string]*ProcessGroup)
	for i := range processes {
		p := &processes[i]
		group := classifyProcess(p)
		p.Group = group

		if _, ok := groupMap[group]; !ok {
			groupMap[group] = &ProcessGroup{Name: group}
		}
		g := groupMap[group]
		g.Processes = append(g.Processes, *p)
		g.TotalRSS += p.RSS
		g.TotalCount++
	}

	// Convert map to sorted slice
	for _, g := range groupMap {
		result.Groups = append(result.Groups, *g)
	}
	sort.Slice(result.Groups, func(i, j int) bool {
		return result.Groups[i].TotalRSS > result.Groups[j].TotalRSS
	})

	// Identify orphans (processes with no parent terminal and high memory)
	result.Orphans = detectOrphans(processes)
	result.TotalOrphans = len(result.Orphans)
	for _, o := range result.Orphans {
		result.OrphanRSS += o.RSS
	}

	return result, nil
}

// classifyProcess determines which group a process belongs to.
func classifyProcess(p *ProcessInfo) string {
	name := strings.ToLower(p.Name)
	cmd := strings.ToLower(p.Command)

	// Check direct name match (pattern keys are already lowercase)
	for pattern, group := range orphanPatterns {
		if strings.Contains(name, pattern) {
			return group
		}
		if strings.Contains(cmd, pattern) {
			return group
		}
	}

	// Heuristics
	if strings.Contains(cmd, "language-server") || strings.Contains(cmd, "languageserver") {
		return "lsp"
	}
	if strings.Contains(name, "helper") && strings.Contains(cmd, ".app/") {
		return "app_helper"
	}

	return "other"
}

// detectOrphans finds processes that are likely orphaned.
func detectOrphans(processes []ProcessInfo) []ProcessInfo {
	var orphans []ProcessInfo
	for _, p := range processes {
		if p.Group == "other" || p.Group == "app_helper" {
			continue
		}
		// A process using > 50 MB RSS is worth flagging
		if p.RSS > 50*1024*1024 {
			orphans = append(orphans, p)
		}
	}
	sort.Slice(orphans, func(i, j int) bool {
		return orphans[i].RSS > orphans[j].RSS
	})
	return orphans
}

// getMemoryInfo populates total/used/free RAM in the result.
func getMemoryInfo(result *AuditResult) error {
	switch runtime.GOOS {
	case "darwin":
		return getDarwinMemoryInfo(result)
	case "linux":
		return getLinuxMemoryInfo(result)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func getDarwinMemoryInfo(result *AuditResult) error {
	// Get total RAM from sysctl
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return err
	}
	total, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return err
	}
	result.TotalRAM = total

	// Get memory pressure from vm_stat
	out, err = exec.Command("vm_stat").Output()
	if err != nil {
		return err
	}

	pageSize := int64(16384) // Apple Silicon default
	var free, active, inactive, wired int64

	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "page size of") {
			parts := strings.Fields(line)
			for _, p := range parts {
				if v, err := strconv.ParseInt(p, 10, 64); err == nil && v > 0 {
					pageSize = v
				}
			}
		}
		if strings.Contains(line, "Pages free") {
			free = parseVMStatValue(line) * pageSize
		}
		if strings.Contains(line, "Pages active") {
			active = parseVMStatValue(line) * pageSize
		}
		if strings.Contains(line, "Pages inactive") {
			inactive = parseVMStatValue(line) * pageSize
		}
		if strings.Contains(line, "Pages wired") {
			wired = parseVMStatValue(line) * pageSize
		}
	}

	result.UsedRAM = active + wired
	result.FreeRAM = free + inactive
	_ = total // total is already set
	return nil
}

func getLinuxMemoryInfo(result *AuditResult) error {
	out, err := exec.Command("free", "-b").Output()
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Mem:") {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				result.TotalRAM, _ = strconv.ParseInt(fields[1], 10, 64)
				result.UsedRAM, _ = strconv.ParseInt(fields[2], 10, 64)
				result.FreeRAM, _ = strconv.ParseInt(fields[3], 10, 64)
			}
		}
	}
	return nil
}

func parseVMStatValue(line string) int64 {
	parts := strings.Split(line, ":")
	if len(parts) < 2 {
		return 0
	}
	valStr := strings.TrimSpace(parts[1])
	valStr = strings.TrimSuffix(valStr, ".")
	v, _ := strconv.ParseInt(valStr, 10, 64)
	return v
}

// getProcessList returns all running processes with memory info.
func getProcessList() ([]ProcessInfo, error) {
	out, err := exec.Command("ps", "-axo", "pid,rss,vsz,%cpu,user,comm").Output()
	if err != nil {
		return nil, err
	}

	var processes []ProcessInfo
	lines := strings.Split(string(out), "\n")

	for i, line := range lines {
		if i == 0 { // Skip header
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		pid, _ := strconv.Atoi(fields[0])
		rss, _ := strconv.ParseInt(fields[1], 10, 64)
		vsz, _ := strconv.ParseInt(fields[2], 10, 64)
		cpu, _ := strconv.ParseFloat(fields[3], 64)
		user := fields[4]
		comm := strings.Join(fields[5:], " ")

		// Extract just the binary name from path
		name := comm
		if idx := strings.LastIndex(comm, "/"); idx >= 0 {
			name = comm[idx+1:]
		}

		processes = append(processes, ProcessInfo{
			PID:        pid,
			Name:       name,
			Command:    comm,
			RSS:        rss * 1024, // ps reports RSS in KB
			VSZ:        vsz * 1024,
			CPUPercent: cpu,
			User:       user,
		})
	}

	return processes, nil
}

// FormatBytes formats bytes to human-readable string.
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
