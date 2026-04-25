// Package guard provides RAM management and process auditing.
// Rule A1: NEVER kill a process without explicit user confirmation.
package guard

import (
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
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
	// NOTE: language_server_macos_arm is intentionally EXCLUDED — it is
	// Antigravity's core AI backend. Killing it crashes the IDE.
	// Use `sirsi guard --deprioritize lsp` to lower its priority instead (safe, reversible).
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

// Audit scans all running processes and groups them by type using the current platform.
func Audit() (*AuditResult, error) {
	return AuditWith(platform.Current())
}

// AuditWith scans all running processes using the provided platform (Rule A16).
// Memory info and process list are fetched concurrently on dedicated OS threads.
func AuditWith(p platform.Platform) (*AuditResult, error) {
	if p.Name() != "darwin" && p.Name() != "linux" && p.Name() != "mock" {
		return nil, fmt.Errorf("guard: unsupported platform %s", p.Name())
	}

	result := &AuditResult{}

	// Run memory and process queries in parallel on dedicated threads.
	var memErr error
	var processes []ProcessInfo
	var procErr error
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		memErr = getMemoryInfoWith(p, result)
	}()
	go func() {
		defer wg.Done()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		processes, procErr = getProcessListWith(p)
	}()
	wg.Wait()

	if memErr != nil {
		return nil, fmt.Errorf("guard: memory info: %w", memErr)
	}
	if procErr != nil {
		return nil, fmt.Errorf("guard: process list: %w", procErr)
	}

	// Group processes
	groupMap := make(map[string]*ProcessGroup)
	for i := range processes {
		proc := &processes[i]
		group := classifyProcess(proc)
		proc.Group = group

		if _, ok := groupMap[group]; !ok {
			groupMap[group] = &ProcessGroup{Name: group}
		}
		g := groupMap[group]
		g.Processes = append(g.Processes, *proc)
		g.TotalRSS += proc.RSS
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

// getMemoryInfoWith populates total/used/free RAM in the result using the provided platform.
func getMemoryInfoWith(p platform.Platform, result *AuditResult) error {
	platformName := p.Name()
	if platformName == "mock" {
		// Try to detect intended mock platform via env or defaults to darwin
		platformName = "darwin"
	}
	switch platformName {
	case "darwin":
		return getDarwinMemoryInfoWith(p, result)
	case "linux":
		return getLinuxMemoryInfoWith(p, result)
	default:
		return fmt.Errorf("unsupported platform: %s", platformName)
	}
}

func getDarwinMemoryInfoWith(p platform.Platform, result *AuditResult) error {
	// Get total RAM from sysctl
	out, err := p.Command("sysctl", "-n", "hw.memsize")
	if err != nil {
		return err
	}
	total, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return err
	}
	result.TotalRAM = total

	// Get memory pressure from vm_stat
	out, err = p.Command("vm_stat")
	if err != nil {
		return err
	}

	pageSize := int64(16384) // Apple Silicon default
	var free, active, inactive, wired int64

	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "page size of") {
			parts := strings.Fields(line)
			for _, part := range parts {
				if v, err := strconv.ParseInt(part, 10, 64); err == nil && v > 0 {
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
	return nil
}

func getLinuxMemoryInfoWith(p platform.Platform, result *AuditResult) error {
	out, err := p.Command("free", "-b")
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

// getProcessListWith returns all running processes with memory info using the provided platform.
func getProcessListWith(p platform.Platform) ([]ProcessInfo, error) {
	out, err := p.Command("ps", "-axo", "pid,rss,vsz,%cpu,user,comm")
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
