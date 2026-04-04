// Package seba provides hardware detection and resource optimization.
// Named after the Egyptian god of the Nile's annual flood — the flow of resources.
package seba

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

// GPUType identifies the GPU/accelerator vendor.
type GPUType string

const (
	GPUAppleMetal GPUType = "apple_metal"
	GPUNVIDIA     GPUType = "nvidia_cuda"
	GPUAMD        GPUType = "amd_rocm"
	GPUIntel      GPUType = "intel"
	GPUNone       GPUType = "cpu_only"
)

// HardwareProfile contains the full hardware detection results.
type HardwareProfile struct {
	// CPU
	CPUCores int    `json:"cpu_cores"`
	CPUModel string `json:"cpu_model"`
	CPUArch  string `json:"cpu_arch"`

	// Memory
	TotalRAM int64 `json:"total_ram"`

	// GPU
	GPU GPUInfo `json:"gpu"`

	// Neural Engine (Apple only)
	NeuralEngine bool `json:"neural_engine"`

	// Platform
	OS     string `json:"os"`
	Kernel string `json:"kernel"`
}

// GPUInfo contains GPU/accelerator details.
type GPUInfo struct {
	Type        GPUType `json:"type"`
	Name        string  `json:"name"`
	VRAM        int64   `json:"vram_bytes,omitempty"`
	MetalFamily string  `json:"metal_family,omitempty"`
	CUDAVersion string  `json:"cuda_version,omitempty"`
	DriverVer   string  `json:"driver_version,omitempty"`
	Compute     string  `json:"compute_capability,omitempty"`
}

// DetectHardware performs full hardware detection for the current platform.
func DetectHardware() (*HardwareProfile, error) {
	profile := &HardwareProfile{
		CPUCores: runtime.NumCPU(),
		CPUArch:  runtime.GOARCH,
		OS:       runtime.GOOS,
	}

	switch runtime.GOOS {
	case "darwin":
		detectDarwinHardware(profile)
	case "linux":
		detectLinuxHardware(profile)
	}

	stele.Inscribe("hapi", stele.TypeHapiDetect, "", map[string]string{
		"cpu":  profile.CPUModel,
		"arch": profile.CPUArch,
		"ram":  fmt.Sprintf("%d", profile.TotalRAM),
	})
	return profile, nil
}

// detectDarwinHardware detects macOS hardware (Apple Silicon, Metal, Neural Engine).
// All system queries run concurrently on dedicated OS threads.
func detectDarwinHardware(p *HardwareProfile) {
	var wg sync.WaitGroup

	wg.Add(4)
	// CPU model
	go func() {
		defer wg.Done()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
			p.CPUModel = strings.TrimSpace(string(out))
		}
	}()

	// Total RAM
	go func() {
		defer wg.Done()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
			p.TotalRAM, _ = strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		}
	}()

	// Kernel
	go func() {
		defer wg.Done()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if out, err := exec.Command("uname", "-r").Output(); err == nil {
			p.Kernel = strings.TrimSpace(string(out))
		}
	}()

	// GPU via system_profiler
	go func() {
		defer wg.Done()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if out, err := exec.Command("system_profiler", "SPDisplaysDataType").Output(); err == nil {
			p.GPU = parseDisplayProfile(string(out))
		}
	}()

	wg.Wait()

	// Neural Engine detection (Apple Silicon M-series) — depends on CPU model
	if strings.Contains(p.CPUModel, "Apple M") {
		p.NeuralEngine = true
	}
}

// detectLinuxHardware detects Linux hardware (NVIDIA, AMD, Intel).
func detectLinuxHardware(p *HardwareProfile) {
	// CPU model
	if out, err := exec.Command("cat", "/proc/cpuinfo").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					p.CPUModel = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}

	// Total RAM
	if out, err := exec.Command("free", "-b").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "Mem:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					p.TotalRAM, _ = strconv.ParseInt(fields[1], 10, 64)
				}
			}
		}
	}

	// NVIDIA GPU
	if out, err := exec.Command("nvidia-smi",
		"--query-gpu=name,memory.total,driver_version,compute_cap",
		"--format=csv,noheader,nounits").Output(); err == nil {
		p.GPU = parseNvidiaSmi(string(out))
	} else {
		// AMD ROCm
		if out, err := exec.Command("rocm-smi", "--showproductname").Output(); err == nil {
			p.GPU = GPUInfo{
				Type: GPUAMD,
				Name: strings.TrimSpace(string(out)),
			}
		} else {
			p.GPU = GPUInfo{Type: GPUNone, Name: "CPU-only"}
		}
	}
}

// parseDisplayProfile extracts GPU info from macOS system_profiler output.
func parseDisplayProfile(output string) GPUInfo {
	info := GPUInfo{Type: GPUAppleMetal}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Chipset Model:") {
			info.Name = strings.TrimPrefix(line, "Chipset Model: ")
		}
		if strings.HasPrefix(line, "Metal Family:") || strings.HasPrefix(line, "Metal Support:") {
			val := strings.TrimPrefix(line, "Metal Family: ")
			val = strings.TrimPrefix(val, "Metal Support: ")
			info.MetalFamily = strings.TrimSpace(val)
		}
		if strings.Contains(line, "VRAM") || strings.Contains(line, "Total Number of Cores") {
			info.DriverVer = strings.TrimSpace(line)
		}
	}

	if info.Name == "" {
		info.Name = "Unknown GPU"
		info.Type = GPUNone
	}
	return info
}

// parseNvidiaSmi extracts GPU info from nvidia-smi output.
func parseNvidiaSmi(output string) GPUInfo {
	info := GPUInfo{Type: GPUNVIDIA}
	parts := strings.Split(strings.TrimSpace(output), ", ")
	if len(parts) >= 1 {
		info.Name = parts[0]
	}
	if len(parts) >= 2 {
		vram, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		info.VRAM = vram * 1024 * 1024 // MiB to bytes
	}
	if len(parts) >= 3 {
		info.DriverVer = parts[2]
	}
	if len(parts) >= 4 {
		info.Compute = parts[3]
	}
	return info
}

// FormatGPUType returns a human-readable GPU type.
func FormatGPUType(t GPUType) string {
	switch t {
	case GPUAppleMetal:
		return "Apple Metal"
	case GPUNVIDIA:
		return "NVIDIA CUDA"
	case GPUAMD:
		return "AMD ROCm"
	case GPUIntel:
		return "Intel"
	default:
		return "CPU-only"
	}
}

// FormatBytes formats bytes to human-readable.
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)
	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1f TB", float64(bytes)/float64(TB))
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
