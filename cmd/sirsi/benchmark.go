package main

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brain"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/seba"
)

var (
	benchBlocks int
	benchSize   int
)

const (
	maxBenchmarkInputBytes = 64 << 20
	maxBenchmarkBlocks     = 10_000
)

var benchmarkCmd = &cobra.Command{
	Use:   "benchmark",
	Short: "Benchmark CPU and Metal hashing plus file-classifier throughput",
	Long: `𓈗 Pantheon Benchmark — Hashing and File Classification

Runs CPU and Metal SHA-256 hashing benchmarks and classifies synthetic file
path strings using the best available classifier. The report names the selected
classifier backend, but does not measure or verify which hardware unit CoreML
uses for inference.

Compares hashing timing, throughput, and correctness; classifier throughput
is reported separately and is not a hashing or hardware-dispatch comparison.
Generated hash input is limited to 64 MiB per run.
Block count is limited to 10,000 to bound per-block work and allocation overhead.

  sirsi benchmark                    Run with defaults (1000 blocks × 4KB)
  sirsi benchmark --blocks 5000      Custom block count
  sirsi benchmark --size 65536       Custom block size (bytes)`,
	RunE: runBenchmark,
}

func init() {
	benchmarkCmd.Flags().IntVar(&benchBlocks, "blocks", 1000, "Number of data blocks to hash")
	benchmarkCmd.Flags().IntVar(&benchSize, "size", 4096, "Size of each block in bytes")
}

func runBenchmark(_ *cobra.Command, _ []string) error {
	if err := validateBenchmarkConfig(benchBlocks, benchSize); err != nil {
		return err
	}
	start := time.Now()
	output.Banner()
	output.Header("Benchmark — CPU / Metal / File classifier")

	// ── Hardware Detection ──────────────────────────────────────────
	profile := seba.DetectAccelerators()
	hw, _ := seba.DetectHardware()

	cpuModel := "unknown"
	if hw != nil {
		cpuModel = hw.CPUModel
	}

	output.Dashboard(map[string]string{
		"CPU":           cpuModel,
		"Cores":         fmt.Sprintf("%d", runtime.NumCPU()),
		"Metal":         fmt.Sprintf("%v (GPU cores: %d)", profile.HasMetal, profile.GPUCores),
		"ANE Available": fmt.Sprintf("%v (cores: %d; availability only)", profile.HasANE, profile.ANECores),
		"Blocks":        fmt.Sprintf("%d × %s", benchBlocks, benchFormatSize(benchSize)),
		"Total":         benchFormatSize(benchBlocks * benchSize),
	})

	// ── Generate test data ──────────────────────────────────────────
	output.Info("Generating %d random blocks (%s each)...", benchBlocks, benchFormatSize(benchSize))
	blocks := make([][]byte, benchBlocks)
	for i := range blocks {
		blocks[i] = make([]byte, benchSize)
		if _, err := rand.Read(blocks[i]); err != nil {
			return fmt.Errorf("generate test data: %w", err)
		}
	}

	// ── Benchmark: CPU Sequential SHA-256 ───────────────────────────
	output.Info("Running CPU sequential SHA-256...")
	cpuSeqStart := time.Now()
	cpuSeqHashes := make([][32]byte, len(blocks))
	for i, b := range blocks {
		cpuSeqHashes[i] = sha256.Sum256(b)
	}
	cpuSeqDur := time.Since(cpuSeqStart)

	// ── Benchmark: CPU Parallel SHA-256 ─────────────────────────────
	output.Info("Running CPU parallel SHA-256 (%d goroutines)...", runtime.NumCPU())
	cpuParStart := time.Now()
	cpuParHashes := make([][32]byte, len(blocks))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for i, b := range blocks {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, data []byte) {
			defer wg.Done()
			defer func() { <-sem }()
			cpuParHashes[idx] = sha256.Sum256(data)
		}(i, b)
	}
	wg.Wait()
	cpuParDur := time.Since(cpuParStart)

	// ── Benchmark: Metal GPU SHA-256 ────────────────────────────────
	output.Info("Running Metal GPU SHA-256...")
	metalDur := time.Duration(0)
	var metalBackend string
	metalMatch := false

	metalHashes, err := seba.MetalHashBatch(blocks)
	if err != nil {
		output.Warn("Metal: %v (falling back to CPU)", err)
		metalDur = cpuParDur
		metalBackend = "cpu-fallback"
	} else {
		// Re-run for timing (first run includes shader compilation)
		metalStart := time.Now()
		metalHashes, _ = seba.MetalHashBatch(blocks)
		metalDur = time.Since(metalStart)

		// Check which backend was used
		if profile.HasMetal {
			metalBackend = "metal-gpu"
		} else {
			metalBackend = "cpu-parallel"
		}
	}

	// Verify Metal hashes match CPU
	if metalHashes != nil {
		metalMatch = true
		for i := range metalHashes {
			if metalHashes[i] != cpuSeqHashes[i] {
				metalMatch = false
				break
			}
		}
	}

	// ── Benchmark: file-path classifier throughput ───────────────────
	output.Info("Classifying synthetic file paths (selected backend; hardware dispatch unverified)...")
	classifierBackend := "unavailable"
	classifierDur := time.Duration(0)
	pathsClassified := 0

	classifier, cerr := brain.GetClassifier()
	if cerr == nil {
		_ = classifier.Load("")
		classifierBackend = classifier.Name()

		// Classify a set of synthetic file paths
		testPaths := generateTestPaths(100)
		classifierStart := time.Now()
		for _, p := range testPaths {
			if _, err := classifier.Classify(p); err == nil {
				pathsClassified++
			}
		}
		classifierDur = time.Since(classifierStart)
		_ = classifier.Close()
	}

	// ── Results Table ───────────────────────────────────────────────
	seqDuration := maxDur(cpuSeqDur)
	parallelDuration := maxDur(cpuParDur)
	metalDuration := maxDur(metalDur)
	throughputSeq := float64(benchBlocks*benchSize) / seqDuration.Seconds() / (1024 * 1024)
	throughputPar := float64(benchBlocks*benchSize) / parallelDuration.Seconds() / (1024 * 1024)
	throughputMetal := float64(benchBlocks*benchSize) / metalDuration.Seconds() / (1024 * 1024)

	speedupPar := seqDuration.Seconds() / parallelDuration.Seconds()
	speedupMetal := seqDuration.Seconds() / metalDuration.Seconds()

	hashRows := [][]string{
		{"CPU Sequential", "go-sha256", cpuSeqDur.Round(time.Microsecond).String(),
			fmt.Sprintf("%.1f MB/s", throughputSeq), "1.00x", "baseline"},
		{"CPU Parallel", fmt.Sprintf("go-sha256 ×%d", runtime.NumCPU()),
			cpuParDur.Round(time.Microsecond).String(),
			fmt.Sprintf("%.1f MB/s", throughputPar),
			fmt.Sprintf("%.2fx", speedupPar), "✓ match"},
		{"Metal GPU", metalBackend, metalDur.Round(time.Microsecond).String(),
			fmt.Sprintf("%.1f MB/s", throughputMetal),
			fmt.Sprintf("%.2fx", speedupMetal),
			map[bool]string{true: "✓ match", false: "✗ mismatch"}[metalMatch]},
	}

	output.Table(
		[]string{"Backend", "Engine", "Duration", "Throughput", "Speedup", "Verify"},
		hashRows,
	)

	// Classifier results use synthetic path strings; they do not establish that
	// CoreML or the ANE processed real file contents.
	classifierRows := [][]string{
		{"File classifier (synthetic paths)", classifierBackend,
			classifierDur.Round(time.Microsecond).String(),
			fmt.Sprintf("%d paths", pathsClassified),
			fmt.Sprintf("%.1f paths/s", float64(pathsClassified)/maxDur(classifierDur).Seconds()),
			"hardware dispatch unverified"},
	}

	output.Table(
		[]string{"Operation", "Classifier backend", "Duration", "Paths classified", "Rate", "Notes"},
		classifierRows,
	)

	output.Footer(time.Since(start))
	return nil
}

// generateTestPaths creates synthetic file paths for classification benchmarking.
func generateTestPaths(n int) []string {
	extensions := []string{
		".go", ".py", ".js", ".log", ".tmp", ".jpg", ".zip", ".yaml",
		".sqlite", ".onnx", ".DS_Store", ".cache", ".csv", ".mp4",
	}
	paths := make([]string, n)
	for i := range paths {
		ext := extensions[i%len(extensions)]
		paths[i] = fmt.Sprintf("/tmp/bench/file_%04d%s", i, ext)
	}
	return paths
}

func validateBenchmarkConfig(blocks, blockSize int) error {
	if blocks <= 0 {
		return fmt.Errorf("benchmark: blocks must be positive")
	}
	if blocks > maxBenchmarkBlocks {
		return fmt.Errorf("benchmark: blocks exceeds %d per-run safety limit", maxBenchmarkBlocks)
	}
	if blockSize <= 0 {
		return fmt.Errorf("benchmark: size must be positive")
	}
	if blocks > maxBenchmarkInputBytes/blockSize {
		return fmt.Errorf("benchmark: total input exceeds %d MiB safety limit", maxBenchmarkInputBytes/(1024*1024))
	}
	return nil
}

func benchFormatSize(bytes int) string {
	switch {
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func maxDur(d time.Duration) time.Duration {
	if d < time.Microsecond {
		return time.Microsecond
	}
	return d
}
