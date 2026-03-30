// Package seba — runtime.go
//
// 𓇽 Phase 2 Runtime Mappers — Live System Probes
//
// These mappers collect live data from the current machine by wiring into
// existing deity modules (Guard, Hapi, Scarab) and system commands.
//
// Memory Pressure, CPU Topology, GPU Architecture, Network Topology,
// Process Memory Map, Disk I/O, Port Exposure, SSH Connections.
package seba

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/hapi"
)

func init() {
	registerMapper(DiagramMemoryPressure, "Memory Pressure Map", generateMemoryPressure)
	registerMapper(DiagramCPUTopology, "CPU Topology", generateCPUTopology)
	registerMapper(DiagramGPUArchitecture, "GPU/Accelerator Architecture", generateGPUArchitecture)
	registerMapper(DiagramProcessMap, "Process Memory Map", generateProcessMap)
	registerMapper(DiagramNetworkPorts, "Network Port Exposure", generateNetworkPorts)
	registerMapper(DiagramSSHConnections, "SSH Connection Map", generateSSHConnections)
	registerMapper(DiagramDiskUsage, "Disk Usage Map", generateDiskUsage)
	registerMapper(DiagramSystemOverview, "System Overview", generateSystemOverview)
}

// Phase 2 runtime diagram types
const (
	DiagramMemoryPressure  DiagramType = "memorypressure"
	DiagramCPUTopology     DiagramType = "cputopology"
	DiagramGPUArchitecture DiagramType = "gpuarch"
	DiagramProcessMap      DiagramType = "processmap"
	DiagramNetworkPorts    DiagramType = "networkports"
	DiagramSSHConnections  DiagramType = "sshmap"
	DiagramDiskUsage       DiagramType = "diskusage"
	DiagramSystemOverview  DiagramType = "systemoverview"
)

// ── Memory Pressure ─────────────────────────────────────────────────
// Uses guard.Audit() to get live RAM usage and process groups.

func generateMemoryPressure(_ string) (*DiagramResult, error) {
	audit, err := guard.Audit()
	if err != nil {
		return nil, fmt.Errorf("guard audit: %w", err)
	}

	totalGB := float64(audit.TotalRAM) / (1024 * 1024 * 1024)
	usedGB := float64(audit.UsedRAM) / (1024 * 1024 * 1024)
	freeGB := float64(audit.FreeRAM) / (1024 * 1024 * 1024)
	usedPct := 0.0
	if audit.TotalRAM > 0 {
		usedPct = float64(audit.UsedRAM) / float64(audit.TotalRAM) * 100
	}

	var sb strings.Builder
	sb.WriteString("graph TD\n")
	sb.WriteString(fmt.Sprintf("    RAM[\"💾 System RAM<br/>%.1f GB Total\"]\n", totalGB))
	sb.WriteString(fmt.Sprintf("    Used[\"🔴 Used: %.1f GB<br/>%.0f%%\"]\n", usedGB, usedPct))
	sb.WriteString(fmt.Sprintf("    Free[\"🟢 Free: %.1f GB<br/>%.0f%%\"]\n", freeGB, 100-usedPct))
	sb.WriteString("    RAM --> Used\n")
	sb.WriteString("    RAM --> Free\n\n")

	// Color code based on pressure
	if usedPct > 85 {
		sb.WriteString("    style Used fill:#E74C3C,stroke:#C0392B,color:#fff\n")
	} else if usedPct > 60 {
		sb.WriteString("    style Used fill:#F39C12,stroke:#E67E22,color:#000\n")
	} else {
		sb.WriteString("    style Used fill:#2ECC71,stroke:#27AE60,color:#000\n")
	}
	sb.WriteString("    style Free fill:#1ABC9C,stroke:#16A085,color:#000\n\n")

	// Process groups by RAM usage
	sb.WriteString("    subgraph groups[\"📊 Process Groups by RSS\"]\n")
	for i, g := range audit.Groups {
		if i >= 8 {
			break
		}
		groupMB := float64(g.TotalRSS) / (1024 * 1024)
		id := sanitizeMermaidID("grp_" + g.Name)
		sb.WriteString(fmt.Sprintf("        %s[\"%s<br/>%d procs · %.0f MB\"]\n", id, g.Name, g.TotalCount, groupMB))
	}
	sb.WriteString("    end\n")
	sb.WriteString("    Used --> groups\n")

	// Orphans
	if len(audit.Orphans) > 0 {
		orphanMB := float64(audit.OrphanRSS) / (1024 * 1024)
		sb.WriteString(fmt.Sprintf("\n    Orphans[\"⚠️ Orphans: %d<br/>%.0f MB\"]\n", audit.TotalOrphans, orphanMB))
		sb.WriteString("    Used --> Orphans\n")
		sb.WriteString("    style Orphans fill:#E74C3C,stroke:#C0392B,color:#fff\n")
	}

	return &DiagramResult{
		Type:    DiagramMemoryPressure,
		Title:   fmt.Sprintf("𓇽 Memory Pressure — %.1f/%.1f GB (%.0f%%)", usedGB, totalGB, usedPct),
		Mermaid: sb.String(),
	}, nil
}

// ── CPU Topology ────────────────────────────────────────────────────
// Uses hapi.DetectHardware() + runtime info.

func generateCPUTopology(_ string) (*DiagramResult, error) {
	hw, err := hapi.DetectHardware()
	if err != nil {
		return nil, fmt.Errorf("hapi detect: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("graph TD\n")
	sb.WriteString(fmt.Sprintf("    CPU[\"🖥️ %s<br/>%s · %d cores\"]\n", hw.CPUModel, hw.CPUArch, hw.CPUCores))

	// Core breakdown
	sb.WriteString("    subgraph cores[\"CPU Cores\"]\n")
	pCores := hw.CPUCores // We'll estimate P vs E cores
	if hw.CPUCores >= 8 { // Apple Silicon typically has P+E split
		pCoreCount := hw.CPUCores / 2
		eCoreCount := hw.CPUCores - pCoreCount
		sb.WriteString(fmt.Sprintf("        pcores[\"⚡ P-Cores: %d<br/>High Performance\"]\n", pCoreCount))
		sb.WriteString(fmt.Sprintf("        ecores[\"🔋 E-Cores: %d<br/>Efficiency\"]\n", eCoreCount))
		_ = pCores
	} else {
		sb.WriteString(fmt.Sprintf("        allcores[\"%d Cores\"]\n", hw.CPUCores))
	}
	sb.WriteString("    end\n")
	sb.WriteString("    CPU --> cores\n\n")

	// Memory
	ramGB := float64(hw.TotalRAM) / (1024 * 1024 * 1024)
	sb.WriteString(fmt.Sprintf("    RAM[\"💾 RAM: %.0f GB\"]\n", ramGB))
	sb.WriteString("    CPU --> RAM\n")

	// Go runtime
	sb.WriteString(fmt.Sprintf("    GoRT[\"🐹 Go Runtime<br/>GOMAXPROCS=%d<br/>goroutines=%d\"]\n",
		runtime.GOMAXPROCS(0), runtime.NumGoroutine()))
	sb.WriteString("    CPU --> GoRT\n")

	// Kernel
	if hw.Kernel != "" {
		sb.WriteString(fmt.Sprintf("    Kernel[\"🔧 Kernel %s\"]\n", hw.Kernel))
		sb.WriteString("    CPU --> Kernel\n")
	}

	sb.WriteString("\n    style CPU fill:#4285F4,stroke:#3367D6,color:#fff\n")
	sb.WriteString("    style RAM fill:#1A1A5E,stroke:#C8A951,color:#C8A951\n")

	return &DiagramResult{
		Type:    DiagramCPUTopology,
		Title:   fmt.Sprintf("𓇽 CPU Topology — %s (%d cores)", hw.CPUModel, hw.CPUCores),
		Mermaid: sb.String(),
	}, nil
}

// ── GPU Architecture ────────────────────────────────────────────────
// Uses hapi.DetectHardware() for GPU/ANE info.

func generateGPUArchitecture(_ string) (*DiagramResult, error) {
	hw, err := hapi.DetectHardware()
	if err != nil {
		return nil, fmt.Errorf("hapi detect: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("graph TD\n")
	sb.WriteString(fmt.Sprintf("    SoC[\"🔲 %s<br/>%s\"]\n", hw.CPUModel, hw.OS))

	// GPU
	gpuLabel := hapi.FormatGPUType(hw.GPU.Type)
	sb.WriteString(fmt.Sprintf("    GPU[\"🎮 %s<br/>%s\"]\n", hw.GPU.Name, gpuLabel))
	sb.WriteString("    SoC --> GPU\n")

	if hw.GPU.MetalFamily != "" {
		sb.WriteString(fmt.Sprintf("    Metal[\"🔮 Metal: %s\"]\n", hw.GPU.MetalFamily))
		sb.WriteString("    GPU --> Metal\n")
	}
	if hw.GPU.VRAM > 0 {
		vramGB := float64(hw.GPU.VRAM) / (1024 * 1024 * 1024)
		sb.WriteString(fmt.Sprintf("    VRAM[\"💎 VRAM: %.0f GB\"]\n", vramGB))
		sb.WriteString("    GPU --> VRAM\n")
	}
	if hw.GPU.CUDAVersion != "" {
		sb.WriteString(fmt.Sprintf("    CUDA[\"🟢 CUDA %s\"]\n", hw.GPU.CUDAVersion))
		sb.WriteString("    GPU --> CUDA\n")
	}
	if hw.GPU.Compute != "" {
		sb.WriteString(fmt.Sprintf("    Compute[\"📐 Compute Cap: %s\"]\n", hw.GPU.Compute))
		sb.WriteString("    GPU --> Compute\n")
	}
	if hw.GPU.DriverVer != "" {
		sb.WriteString(fmt.Sprintf("    Driver[\"🔧 %s\"]\n", hw.GPU.DriverVer))
		sb.WriteString("    GPU --> Driver\n")
	}

	// Neural Engine
	if hw.NeuralEngine {
		sb.WriteString("    ANE[\"🧠 Apple Neural Engine<br/>16-core · ML Acceleration\"]\n")
		sb.WriteString("    SoC --> ANE\n")
		sb.WriteString("    style ANE fill:#9B59B6,stroke:#8E44AD,color:#fff\n")
	}

	// Color based on GPU type
	switch hw.GPU.Type {
	case hapi.GPUAppleMetal:
		sb.WriteString("    style GPU fill:#333333,stroke:#C8A951,color:#C8A951\n")
	case hapi.GPUNVIDIA:
		sb.WriteString("    style GPU fill:#76B900,stroke:#5A8F00,color:#000\n")
	case hapi.GPUAMD:
		sb.WriteString("    style GPU fill:#ED1C24,stroke:#B8171C,color:#fff\n")
	}
	sb.WriteString("    style SoC fill:#2C3E50,stroke:#1A252F,color:#fff\n")

	return &DiagramResult{
		Type:    DiagramGPUArchitecture,
		Title:   fmt.Sprintf("𓇽 GPU Architecture — %s (%s)", hw.GPU.Name, gpuLabel),
		Mermaid: sb.String(),
	}, nil
}

// ── Process Memory Map ──────────────────────────────────────────────
// Top processes by RSS with group classification.

func generateProcessMap(_ string) (*DiagramResult, error) {
	audit, err := guard.Audit()
	if err != nil {
		return nil, fmt.Errorf("guard audit: %w", err)
	}

	// Extract top 20 processes by RSS from all groups
	var allProcs []guard.ProcessInfo
	for _, g := range audit.Groups {
		allProcs = append(allProcs, g.Processes...)
	}
	sort.Slice(allProcs, func(i, j int) bool {
		return allProcs[i].RSS > allProcs[j].RSS
	})
	if len(allProcs) > 20 {
		allProcs = allProcs[:20]
	}

	var sb strings.Builder
	sb.WriteString("graph LR\n")

	for i, p := range allProcs {
		mb := float64(p.RSS) / (1024 * 1024)
		id := fmt.Sprintf("p%d", i)
		label := p.Name
		if len(label) > 25 {
			label = label[:22] + "..."
		}
		sb.WriteString(fmt.Sprintf("    %s[\"%s<br/>PID %d · %.0f MB · %.1f%% CPU\"]\n",
			id, label, p.PID, mb, p.CPUPercent))

		// Color by size
		if mb > 1024 {
			sb.WriteString(fmt.Sprintf("    style %s fill:#E74C3C,stroke:#C0392B,color:#fff\n", id))
		} else if mb > 256 {
			sb.WriteString(fmt.Sprintf("    style %s fill:#F39C12,stroke:#E67E22,color:#000\n", id))
		} else if mb > 64 {
			sb.WriteString(fmt.Sprintf("    style %s fill:#3498DB,stroke:#2980B9,color:#fff\n", id))
		}
	}

	return &DiagramResult{
		Type:    DiagramProcessMap,
		Title:   fmt.Sprintf("𓇽 Process Memory Map — Top %d by RSS", len(allProcs)),
		Mermaid: sb.String(),
	}, nil
}

// ── Network Port Exposure ───────────────────────────────────────────
// Scans listening ports via lsof.

func generateNetworkPorts(_ string) (*DiagramResult, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("lsof", "-iTCP", "-sTCP:LISTEN", "-nP", "-Fn")
	case "linux":
		cmd = exec.Command("ss", "-tlnp")
	default:
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("port scan: %w", err)
	}

	type listener struct {
		name string
		port string
		pid  string
	}

	var listeners []listener
	if runtime.GOOS == "darwin" {
		// Parse lsof output
		var currentName, currentPID string
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "p") {
				currentPID = line[1:]
			}
			if strings.HasPrefix(line, "c") {
				currentName = line[1:]
			}
			if strings.HasPrefix(line, "n") {
				addr := line[1:]
				if idx := strings.LastIndex(addr, ":"); idx >= 0 {
					port := addr[idx+1:]
					listeners = append(listeners, listener{
						name: currentName,
						port: port,
						pid:  currentPID,
					})
				}
			}
		}
	} else {
		// Parse ss output
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 5 && fields[0] == "LISTEN" {
				addr := fields[3]
				if idx := strings.LastIndex(addr, ":"); idx >= 0 {
					port := addr[idx+1:]
					procField := ""
					if len(fields) >= 6 {
						procField = fields[5]
					}
					listeners = append(listeners, listener{
						name: procField,
						port: port,
					})
				}
			}
		}
	}

	// Deduplicate by port
	portSet := map[string]listener{}
	for _, l := range listeners {
		if _, exists := portSet[l.port]; !exists {
			portSet[l.port] = l
		}
	}

	var sb strings.Builder
	sb.WriteString("graph LR\n")
	sb.WriteString(fmt.Sprintf("    Machine[\"🖥️ %s\"]\n\n", runtime.GOOS))

	i := 0
	for port, l := range portSet {
		if i >= 25 {
			break
		}
		id := fmt.Sprintf("port%d", i)
		label := l.name
		if label == "" {
			label = "unknown"
		}
		sb.WriteString(fmt.Sprintf("    %s[\":%s<br/>%s\"]\n", id, port, label))
		sb.WriteString(fmt.Sprintf("    Machine --> %s\n", id))

		// Color well-known ports
		portNum, _ := strconv.Atoi(port)
		switch {
		case portNum == 22:
			sb.WriteString(fmt.Sprintf("    style %s fill:#E74C3C,stroke:#C0392B,color:#fff\n", id))
		case portNum == 80 || portNum == 443:
			sb.WriteString(fmt.Sprintf("    style %s fill:#2ECC71,stroke:#27AE60,color:#000\n", id))
		case portNum >= 3000 && portNum <= 9999:
			sb.WriteString(fmt.Sprintf("    style %s fill:#3498DB,stroke:#2980B9,color:#fff\n", id))
		}
		i++
	}

	return &DiagramResult{
		Type:    DiagramNetworkPorts,
		Title:   fmt.Sprintf("𓇽 Network Ports — %d Listening", len(portSet)),
		Mermaid: sb.String(),
	}, nil
}

// ── SSH Connection Map ──────────────────────────────────────────────
// Parses ~/.ssh/config for configured hosts.

func generateSSHConnections(_ string) (*DiagramResult, error) {
	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, ".ssh", "config")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("ssh config: %w", err)
	}

	type sshHost struct {
		name     string
		hostname string
		user     string
		port     string
	}

	var hosts []sshHost
	var current *sshHost

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		if strings.HasPrefix(strings.ToLower(line), "host ") && !strings.HasPrefix(strings.ToLower(line), "hostname") {
			if current != nil {
				hosts = append(hosts, *current)
			}
			name := strings.TrimSpace(strings.TrimPrefix(line, "Host "))
			name = strings.TrimSpace(strings.TrimPrefix(name, "host "))
			if name != "*" {
				current = &sshHost{name: name}
			} else {
				current = nil
			}
			continue
		}

		if current == nil {
			continue
		}

		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "hostname") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				current.hostname = parts[1]
			}
		}
		if strings.HasPrefix(lower, "user") && !strings.HasPrefix(lower, "userknownhostsfile") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				current.user = parts[1]
			}
		}
		if strings.HasPrefix(lower, "port") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				current.port = parts[1]
			}
		}
	}
	if current != nil {
		hosts = append(hosts, *current)
	}

	var sb strings.Builder
	sb.WriteString("graph LR\n")
	sb.WriteString("    Local[\"🖥️ Local Machine\"]\n\n")

	for i, h := range hosts {
		id := fmt.Sprintf("ssh%d", i)
		label := h.name
		if h.hostname != "" && h.hostname != h.name {
			label += "<br/>" + h.hostname
		}
		if h.user != "" {
			label += "<br/>user: " + h.user
		}
		port := "22"
		if h.port != "" {
			port = h.port
		}
		sb.WriteString(fmt.Sprintf("    %s[\"%s<br/>:%s\"]\n", id, label, port))
		sb.WriteString(fmt.Sprintf("    Local -->|SSH| %s\n", id))
		sb.WriteString(fmt.Sprintf("    style %s fill:#2C3E50,stroke:#1A252F,color:#C8A951\n", id))
	}

	if len(hosts) == 0 {
		sb.WriteString("    none[\"No SSH hosts configured\"]\n")
		sb.WriteString("    Local --> none\n")
	}

	return &DiagramResult{
		Type:    DiagramSSHConnections,
		Title:   fmt.Sprintf("𓇽 SSH Connection Map — %d Hosts", len(hosts)),
		Mermaid: sb.String(),
	}, nil
}

// ── Disk Usage ──────────────────────────────────────────────────────

func generateDiskUsage(_ string) (*DiagramResult, error) {
	out, err := exec.Command("df", "-h").Output()
	if err != nil {
		return nil, fmt.Errorf("df: %w", err)
	}

	type mountInfo struct {
		filesystem string
		size       string
		used       string
		avail      string
		usePct     string
		mountPoint string
	}

	var mounts []mountInfo
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 || fields[0] == "Filesystem" {
			continue
		}
		// Skip small/system mounts
		mp := fields[len(fields)-1]
		if strings.HasPrefix(mp, "/dev") && !strings.HasPrefix(mp, "/dev/disk") {
			continue
		}
		if strings.HasPrefix(mp, "/System") || strings.HasPrefix(mp, "/private/var/vm") {
			continue
		}
		if strings.HasPrefix(fields[0], "devfs") || strings.HasPrefix(fields[0], "map ") {
			continue
		}

		m := mountInfo{
			filesystem: fields[0],
			size:       fields[1],
			used:       fields[2],
			avail:      fields[3],
		}
		// Handle different df output formats
		if len(fields) >= 9 { // macOS format
			m.usePct = fields[4]
			m.mountPoint = fields[len(fields)-1]
		} else {
			m.usePct = fields[4]
			m.mountPoint = fields[5]
		}
		mounts = append(mounts, m)
	}

	var sb strings.Builder
	sb.WriteString("graph TD\n")

	for i, m := range mounts {
		if i >= 10 {
			break
		}
		id := fmt.Sprintf("disk%d", i)
		label := m.mountPoint
		if len(label) > 30 {
			label = "..." + label[len(label)-27:]
		}
		sb.WriteString(fmt.Sprintf("    %s[\"%s<br/>%s / %s (%s)\"]\n",
			id, label, m.used, m.size, m.usePct))

		// Color by usage
		pctStr := strings.TrimSuffix(m.usePct, "%")
		pct, _ := strconv.Atoi(pctStr)
		switch {
		case pct > 85:
			sb.WriteString(fmt.Sprintf("    style %s fill:#E74C3C,stroke:#C0392B,color:#fff\n", id))
		case pct > 60:
			sb.WriteString(fmt.Sprintf("    style %s fill:#F39C12,stroke:#E67E22,color:#000\n", id))
		default:
			sb.WriteString(fmt.Sprintf("    style %s fill:#2ECC71,stroke:#27AE60,color:#000\n", id))
		}
	}

	return &DiagramResult{
		Type:    DiagramDiskUsage,
		Title:   fmt.Sprintf("𓇽 Disk Usage — %d Volumes", len(mounts)),
		Mermaid: sb.String(),
	}, nil
}

// ── System Overview ─────────────────────────────────────────────────
// Combines CPU + GPU + RAM + Disk into one diagram. The "master map".

func generateSystemOverview(_ string) (*DiagramResult, error) {
	hw, err := hapi.DetectHardware()
	if err != nil {
		return nil, fmt.Errorf("hapi detect: %w", err)
	}

	audit, err := guard.Audit()
	if err != nil {
		return nil, fmt.Errorf("guard audit: %w", err)
	}

	ramGB := float64(hw.TotalRAM) / (1024 * 1024 * 1024)
	usedGB := float64(audit.UsedRAM) / (1024 * 1024 * 1024)
	usedPct := 0.0
	if audit.TotalRAM > 0 {
		usedPct = float64(audit.UsedRAM) / float64(audit.TotalRAM) * 100
	}

	var sb strings.Builder
	sb.WriteString("graph TD\n")

	// Machine
	sb.WriteString(fmt.Sprintf("    Machine[\"🏛️ %s<br/>%s\"]\n", hw.CPUModel, hw.OS))

	// CPU
	sb.WriteString(fmt.Sprintf("    CPU[\"🖥️ CPU<br/>%d cores · %s\"]\n", hw.CPUCores, hw.CPUArch))
	sb.WriteString("    Machine --> CPU\n")

	// RAM
	sb.WriteString(fmt.Sprintf("    RAM[\"💾 RAM<br/>%.0f GB total · %.0f GB used (%.0f%%)\"]\n", ramGB, usedGB, usedPct))
	sb.WriteString("    Machine --> RAM\n")

	// GPU
	gpuLabel := hapi.FormatGPUType(hw.GPU.Type)
	sb.WriteString(fmt.Sprintf("    GPU[\"🎮 %s<br/>%s\"]\n", hw.GPU.Name, gpuLabel))
	sb.WriteString("    Machine --> GPU\n")

	// Neural Engine
	if hw.NeuralEngine {
		sb.WriteString("    ANE[\"🧠 Neural Engine<br/>ML Acceleration\"]\n")
		sb.WriteString("    Machine --> ANE\n")
	}

	// Go runtime
	sb.WriteString(fmt.Sprintf("    GoRT[\"🐹 Go Runtime<br/>%d goroutines\"]\n", runtime.NumGoroutine()))
	sb.WriteString("    CPU --> GoRT\n")

	// Process summary
	totalProcs := 0
	for _, g := range audit.Groups {
		totalProcs += g.TotalCount
	}
	sb.WriteString(fmt.Sprintf("    Procs[\"⚡ %d Processes<br/>%d groups\"]\n", totalProcs, len(audit.Groups)))
	sb.WriteString("    RAM --> Procs\n")

	// Orphans
	if audit.TotalOrphans > 0 {
		orphanMB := float64(audit.OrphanRSS) / (1024 * 1024)
		sb.WriteString(fmt.Sprintf("    Orphans[\"⚠️ %d Orphans<br/>%.0f MB\"]\n", audit.TotalOrphans, orphanMB))
		sb.WriteString("    Procs --> Orphans\n")
		sb.WriteString("    style Orphans fill:#E74C3C,stroke:#C0392B,color:#fff\n")
	}

	// Styling
	sb.WriteString("\n    style Machine fill:#2C3E50,stroke:#1A252F,color:#fff\n")
	sb.WriteString("    style CPU fill:#4285F4,stroke:#3367D6,color:#fff\n")
	if usedPct > 85 {
		sb.WriteString("    style RAM fill:#E74C3C,stroke:#C0392B,color:#fff\n")
	} else {
		sb.WriteString("    style RAM fill:#1A1A5E,stroke:#C8A951,color:#C8A951\n")
	}
	switch hw.GPU.Type {
	case hapi.GPUAppleMetal:
		sb.WriteString("    style GPU fill:#333333,stroke:#C8A951,color:#C8A951\n")
	case hapi.GPUNVIDIA:
		sb.WriteString("    style GPU fill:#76B900,stroke:#5A8F00,color:#000\n")
	}

	return &DiagramResult{
		Type:    DiagramSystemOverview,
		Title:   fmt.Sprintf("𓇽 System Overview — %s · %d cores · %.0f GB", hw.CPUModel, hw.CPUCores, ramGB),
		Mermaid: sb.String(),
	}, nil
}
