// Package vitals provides lightweight system stats collection shared
// across TUI, menubar, and dashboard surfaces.
package vitals

import (
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProcInfo holds info about a running process.
type ProcInfo struct {
	Name       string
	CPUPercent float64
	MemMB      float64
	PID        int
}

// Snapshot holds a point-in-time collection of system vitals.
type Snapshot struct {
	RAMPercent  float64
	RAMPressure string // "low", "medium", "high"
	RAMIcon     string
	RAMTotalGB  float64 // total RAM in GB
	RAMUsedGB   float64 // used RAM in GB

	GitBranch   string
	Uncommitted int
	LastCommit  string // human-friendly relative time ("2m", "1h", etc.)

	Accelerator string // short label ("M5 Max", "Intel", etc.)

	// CPU
	CPUPercent float64    // overall CPU usage 0-100
	CPUCores   []float64  // per-core usage percentages
	CPULoadAvg [3]float64 // 1m, 5m, 15m

	// Top processes
	TopProcs []ProcInfo // top 5 by CPU

	// Network
	NetDownBps float64 // bytes/sec download
	NetUpBps   float64 // bytes/sec upload

	// Disk
	DiskPercent float64 // disk usage 0-100
	DiskUsedGB  float64
	DiskTotalGB float64
	DiskFreeGB  float64

	// Uptime
	UptimeStr string // human-friendly "3d 12h"

	// Machine info
	ModelName string // e.g. "MacBook Pro"
	OSVersion string // e.g. "macOS 15.5"
}

// Network state for delta calculations
var (
	netMu       sync.Mutex
	prevNetDown int64
	prevNetUp   int64
	prevNetTime time.Time
)

// runCommand executes a command and returns its stdout. It is a
// package-level var so tests can feed canned output instead of shelling out.
var runCommand = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// Collect gathers a fresh vitals snapshot. Designed to be fast (< 200ms).
func Collect() Snapshot {
	var s Snapshot
	collectRAM(&s)
	collectGit(&s)
	collectAccelerator(&s)
	collectCPU(&s)
	collectTopProcs(&s)
	collectNetwork(&s)
	collectDisk(&s)
	collectLoadAvg(&s)
	collectUptime(&s)
	collectMachineInfo(&s)
	return s
}

func collectRAM(s *Snapshot) {
	out, err := runCommand("sysctl", "-n", "hw.memsize")
	if err != nil {
		return
	}
	var total int64
	_, _ = fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &total)
	if total == 0 {
		return
	}

	vmOut, err := runCommand("vm_stat")
	if err != nil {
		return
	}

	var active, wired int64
	for _, line := range strings.Split(string(vmOut), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		val := strings.TrimSpace(strings.TrimSuffix(parts[1], "."))
		var n int64
		_, _ = fmt.Sscanf(val, "%d", &n)
		n *= 4096 // pages to bytes (macOS default page size)
		switch {
		case strings.Contains(parts[0], "Pages active"):
			active = n
		case strings.Contains(parts[0], "Pages wired"):
			wired = n
		}
	}

	used := active + wired
	s.RAMPercent = float64(used) / float64(total) * 100
	s.RAMTotalGB = float64(total) / (1024 * 1024 * 1024)
	s.RAMUsedGB = float64(used) / (1024 * 1024 * 1024)

	switch {
	case s.RAMPercent > 85:
		s.RAMPressure = "high"
		s.RAMIcon = "🔴"
	case s.RAMPercent > 65:
		s.RAMPressure = "medium"
		s.RAMIcon = "🟡"
	default:
		s.RAMPressure = "low"
		s.RAMIcon = "🟢"
	}
}

func collectGit(s *Snapshot) {
	if out, err := runCommand("git", "branch", "--show-current"); err == nil {
		s.GitBranch = strings.TrimSpace(string(out))
	}
	if out, err := runCommand("git", "status", "--porcelain"); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) == 1 && lines[0] == "" {
			s.Uncommitted = 0
		} else {
			s.Uncommitted = len(lines)
		}
	}
	if out, err := runCommand("git", "log", "-1", "--format=%cr"); err == nil {
		s.LastCommit = strings.TrimSpace(string(out))
	}
}

func collectAccelerator(s *Snapshot) {
	out, err := runCommand("sysctl", "-n", "machdep.cpu.brand_string")
	if err != nil {
		return
	}
	brand := strings.TrimSpace(string(out))
	s.Accelerator = brand
	// Shorten Apple Silicon to just the chip name
	if strings.Contains(brand, "Apple") {
		parts := strings.Fields(brand)
		if len(parts) >= 2 {
			s.Accelerator = strings.Join(parts[1:], " ") // "M5 Max"
		}
	}
}

func collectCPU(s *Snapshot) {
	// Get overall CPU usage from top -l 1
	out, err := runCommand("top", "-l", "1", "-n", "0", "-s", "0")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "CPU usage:") {
			// "CPU usage: 12.34% user, 5.67% sys, 81.99% idle"
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "idle" && i > 0 {
					idleStr := strings.TrimSuffix(parts[i-1], "%")
					idle, parseErr := strconv.ParseFloat(idleStr, 64)
					if parseErr == nil {
						s.CPUPercent = 100 - idle
					}
				}
			}
			break
		}
	}

	// Per-core: approximate from core count
	coreOut, err := runCommand("sysctl", "-n", "hw.logicalcpu")
	if err != nil {
		return
	}
	cores, err := strconv.Atoi(strings.TrimSpace(string(coreOut)))
	if err != nil || cores <= 0 {
		return
	}
	avgPerCore := s.CPUPercent // approximate: spread overall across cores
	s.CPUCores = make([]float64, cores)
	for i := range s.CPUCores {
		s.CPUCores[i] = avgPerCore
	}
}

func collectTopProcs(s *Snapshot) {
	// ps -Ao %cpu,rss,pid,comm -r — sorted by CPU descending
	out, err := runCommand("ps", "-Ao", "%cpu,rss,pid,comm", "-r")
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	count := 0
	for _, line := range lines {
		if count >= 5 {
			break
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		cpu, err1 := strconv.ParseFloat(fields[0], 64)
		rssKB, err2 := strconv.ParseInt(fields[1], 10, 64)
		pid, err3 := strconv.Atoi(fields[2])
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		if cpu <= 0 {
			continue
		}
		// Get just the binary name from the full path
		name := fields[3]
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		// Skip kernel processes
		if name == "kernel_task" || name == "launchd" || name == "WindowServer" {
			continue
		}
		s.TopProcs = append(s.TopProcs, ProcInfo{
			Name:       name,
			CPUPercent: cpu,
			MemMB:      float64(rssKB) / 1024,
			PID:        pid,
		})
		count++
	}
}

func collectNetwork(s *Snapshot) {
	out, err := runCommand("netstat", "-ib")
	if err != nil {
		return
	}
	var totalIn, totalOut int64
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		// Look for en0 (primary interface)
		if !strings.HasPrefix(fields[0], "en0") {
			continue
		}
		// netstat -ib columns: Name Mtu Network Address Ipkts Ierrs Ibytes Opkts Oerrs Obytes
		ibytes, err1 := strconv.ParseInt(fields[6], 10, 64)
		obytes, err2 := strconv.ParseInt(fields[9], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		totalIn += ibytes
		totalOut += obytes
	}

	netMu.Lock()
	defer netMu.Unlock()

	now := time.Now()
	if !prevNetTime.IsZero() && totalIn > 0 {
		dt := now.Sub(prevNetTime).Seconds()
		if dt > 0 {
			s.NetDownBps = float64(totalIn-prevNetDown) / dt
			s.NetUpBps = float64(totalOut-prevNetUp) / dt
			// Clamp negatives (interface reset)
			if s.NetDownBps < 0 {
				s.NetDownBps = 0
			}
			if s.NetUpBps < 0 {
				s.NetUpBps = 0
			}
		}
	}
	prevNetDown = totalIn
	prevNetUp = totalOut
	prevNetTime = now
}

func collectDisk(s *Snapshot) {
	out, err := runCommand("df", "-g", "/")
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return
	}
	total, _ := strconv.ParseFloat(fields[1], 64)
	used, _ := strconv.ParseFloat(fields[2], 64)
	avail, _ := strconv.ParseFloat(fields[3], 64)
	s.DiskTotalGB = total
	s.DiskUsedGB = used
	s.DiskFreeGB = avail
	if total > 0 {
		s.DiskPercent = used / total * 100
	}
}

func collectLoadAvg(s *Snapshot) {
	out, err := runCommand("sysctl", "-n", "vm.loadavg")
	if err != nil {
		return
	}
	// Output: "{ 1.23 4.56 7.89 }"
	raw := strings.TrimSpace(string(out))
	raw = strings.Trim(raw, "{}")
	parts := strings.Fields(raw)
	if len(parts) >= 3 {
		s.CPULoadAvg[0], _ = strconv.ParseFloat(parts[0], 64)
		s.CPULoadAvg[1], _ = strconv.ParseFloat(parts[1], 64)
		s.CPULoadAvg[2], _ = strconv.ParseFloat(parts[2], 64)
	}
}

func collectUptime(s *Snapshot) {
	out, err := runCommand("sysctl", "-n", "kern.boottime")
	if err != nil {
		return
	}
	// Output: "{ sec = 1234567890, usec = 123456 } Thu Jan  1 00:00:00 1970"
	raw := string(out)
	if idx := strings.Index(raw, "sec = "); idx >= 0 {
		secStr := raw[idx+6:]
		if end := strings.Index(secStr, ","); end >= 0 {
			secStr = secStr[:end]
		}
		sec, err := strconv.ParseInt(strings.TrimSpace(secStr), 10, 64)
		if err != nil {
			return
		}
		bootTime := time.Unix(sec, 0)
		uptime := time.Since(bootTime)
		days := int(uptime.Hours()) / 24
		hours := int(uptime.Hours()) % 24
		if days > 0 {
			s.UptimeStr = fmt.Sprintf("%dd %dh", days, hours)
		} else if hours > 0 {
			s.UptimeStr = fmt.Sprintf("%dh %dm", hours, int(uptime.Minutes())%60)
		} else {
			s.UptimeStr = fmt.Sprintf("%dm", int(uptime.Minutes()))
		}
	}
}

func collectMachineInfo(s *Snapshot) {
	if out, err := runCommand("sysctl", "-n", "hw.model"); err == nil {
		model := strings.TrimSpace(string(out))
		// Map model identifiers to friendly names
		switch {
		case strings.Contains(model, "MacBookPro"):
			s.ModelName = "MacBook Pro"
		case strings.Contains(model, "MacBookAir"):
			s.ModelName = "MacBook Air"
		case strings.Contains(model, "Macmini"):
			s.ModelName = "Mac mini"
		case strings.Contains(model, "MacPro"):
			s.ModelName = "Mac Pro"
		case strings.Contains(model, "iMac"):
			s.ModelName = "iMac"
		default:
			s.ModelName = model
		}
	}
	if out, err := runCommand("sw_vers", "-productVersion"); err == nil {
		s.OSVersion = "macOS " + strings.TrimSpace(string(out))
	}
}

// SortProcsByCPU sorts processes by CPU descending (for display).
func SortProcsByCPU(procs []ProcInfo) {
	sort.Slice(procs, func(i, j int) bool {
		return procs[i].CPUPercent > procs[j].CPUPercent
	})
}
