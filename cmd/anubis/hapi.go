package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-anubis/internal/hapi"
	"github.com/SirsiMaster/sirsi-anubis/internal/output"
)

var (
	hapiGPU       bool
	hapiDedup     bool
	hapiSnapshots bool
	hapiDedupDirs []string
	hapiMinSize   int64
)

var hapiCmd = &cobra.Command{
	Use:   "hapi",
	Short: "🌊 Optimize resources — GPU detection, dedup, snapshot management",
	Long: `🌊 Hapi — The Flow of Resources

Named after the Egyptian god of the Nile's annual flood.
Hapi manages the flow of compute, storage, and memory resources.

  anubis hapi                  Full resource audit (hardware + storage)
  anubis hapi --gpu            GPU/accelerator detection only
  anubis hapi --dedup          Find duplicate files
  anubis hapi --snapshots      List APFS/Time Machine snapshots

GPU Detection:
  Apple Metal/MLX (Neural Engine), NVIDIA CUDA, AMD ROCm, Intel`,
	Run: runHapi,
}

func init() {
	hapiCmd.Flags().BoolVar(&hapiGPU, "gpu", false, "Show GPU/accelerator detection only")
	hapiCmd.Flags().BoolVar(&hapiDedup, "dedup", false, "Find duplicate files")
	hapiCmd.Flags().BoolVar(&hapiSnapshots, "snapshots", false, "List APFS/Time Machine snapshots")
	hapiCmd.Flags().StringSliceVar(&hapiDedupDirs, "dirs", nil, "Directories to scan for duplicates (default: ~/Downloads, ~/Desktop)")
	hapiCmd.Flags().Int64Var(&hapiMinSize, "min-size", 1024*1024, "Minimum file size for dedup in bytes (default: 1MB)")
}

func runHapi(cmd *cobra.Command, args []string) {
	// Specific sub-modes
	if hapiGPU {
		runHapiGPU()
		return
	}
	if hapiDedup {
		runHapiDedup()
		return
	}
	if hapiSnapshots {
		runHapiSnapshots()
		return
	}

	// Default: full resource audit
	runHapiGPU()
	fmt.Println()
	runHapiSnapshots()
}

func runHapiGPU() {
	output.Header("🌊 Hapi — Hardware Detection")
	fmt.Println()

	profile, err := hapi.DetectHardware()
	if err != nil {
		output.Error(fmt.Sprintf("Hardware detection failed: %v", err))
		os.Exit(1)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(profile)
		return
	}

	// CPU
	output.Info(fmt.Sprintf("CPU:           %s", profile.CPUModel))
	output.Info(fmt.Sprintf("Cores:         %d", profile.CPUCores))
	output.Info(fmt.Sprintf("Architecture:  %s", profile.CPUArch))
	output.Info(fmt.Sprintf("RAM:           %s", hapi.FormatBytes(profile.TotalRAM)))
	fmt.Println()

	// GPU
	gpu := profile.GPU
	output.Info(fmt.Sprintf("GPU:           %s", gpu.Name))
	output.Info(fmt.Sprintf("Type:          %s", hapi.FormatGPUType(gpu.Type)))
	if gpu.MetalFamily != "" {
		output.Info(fmt.Sprintf("Metal:         %s", gpu.MetalFamily))
	}
	if gpu.VRAM > 0 {
		output.Info(fmt.Sprintf("VRAM:          %s", hapi.FormatBytes(gpu.VRAM)))
	}
	if gpu.CUDAVersion != "" {
		output.Info(fmt.Sprintf("CUDA:          %s", gpu.CUDAVersion))
	}
	if gpu.Compute != "" {
		output.Info(fmt.Sprintf("Compute:       %s", gpu.Compute))
	}
	if gpu.DriverVer != "" {
		output.Info(fmt.Sprintf("Driver:        %s", gpu.DriverVer))
	}
	fmt.Println()

	// Neural Engine
	if profile.NeuralEngine {
		output.Info("Neural Engine: ✅ Available (Apple Silicon)")
		output.Info("               Ready for on-device ML inference")
	} else {
		output.Info("Neural Engine: ❌ Not available")
	}

	// Recommendations
	fmt.Println()
	switch gpu.Type {
	case hapi.GPUAppleMetal:
		output.Info("💡 Recommendation: Use CoreML/MLX for on-device inference")
		output.Info("   Unified memory eliminates GPU↔CPU transfer overhead")
	case hapi.GPUNVIDIA:
		output.Info("💡 Recommendation: Use CUDA for GPU-accelerated workloads")
		if gpu.VRAM > 0 {
			output.Info(fmt.Sprintf("   Available VRAM: %s", hapi.FormatBytes(gpu.VRAM)))
		}
	case hapi.GPUAMD:
		output.Info("💡 Recommendation: Use ROCm for AMD GPU acceleration")
	default:
		output.Info("💡 CPU-only detected — consider ONNX Runtime for optimized inference")
	}
}

func runHapiDedup() {
	output.Header("🌊 Hapi — Duplicate File Detection")
	fmt.Println()

	dirs := hapiDedupDirs
	if len(dirs) == 0 {
		home, _ := os.UserHomeDir()
		dirs = []string{
			filepath.Join(home, "Downloads"),
			filepath.Join(home, "Desktop"),
		}
	}

	output.Info(fmt.Sprintf("Scanning %d directories (min size: %s)...", len(dirs), hapi.FormatBytes(hapiMinSize)))
	fmt.Println()

	result, err := hapi.FindDuplicates(dirs, hapiMinSize)
	if err != nil {
		output.Error(fmt.Sprintf("Dedup scan failed: %v", err))
		os.Exit(1)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
		return
	}

	output.Info(fmt.Sprintf("Files scanned:  %d", result.Scanned))
	output.Info(fmt.Sprintf("Duplicate sets: %d", len(result.Groups)))
	output.Info(fmt.Sprintf("Wasted space:   %s", hapi.FormatBytes(result.TotalWasted)))
	fmt.Println()

	if len(result.Groups) == 0 {
		output.Info("✅ No duplicate files found")
		return
	}

	limit := 10
	if len(result.Groups) < limit {
		limit = len(result.Groups)
	}

	for i, g := range result.Groups[:limit] {
		output.Warn(fmt.Sprintf("  Set %d — %s × %d copies (wasted: %s)",
			i+1, hapi.FormatBytes(g.Size), len(g.Files), hapi.FormatBytes(g.Wasted)))
		for _, f := range g.Files {
			fmt.Printf("    📄 %s\n", f)
		}
		fmt.Println()
	}

	if len(result.Groups) > limit {
		output.Info(fmt.Sprintf("  ... and %d more duplicate sets", len(result.Groups)-limit))
	}
}

func runHapiSnapshots() {
	output.Header("🌊 Hapi — APFS Snapshots")
	fmt.Println()

	result, err := hapi.ListSnapshots()
	if err != nil {
		output.Warn(fmt.Sprintf("Snapshot listing failed: %v", err))
		return
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
		return
	}

	if result.Total == 0 {
		output.Info("✅ No local Time Machine snapshots found")
		return
	}

	output.Info(fmt.Sprintf("Found %d local snapshots:", result.Total))
	fmt.Println()

	for _, s := range result.Snapshots {
		date := s.Date
		if date == "" {
			date = "unknown"
		}
		fmt.Printf("    📸 %s  (%s)\n", s.Name, date)
	}
	fmt.Println()
	output.Info("To prune old snapshots: sudo tmutil deletelocalsnapshots <date>")
}
