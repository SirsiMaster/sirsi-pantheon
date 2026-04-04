package thoth

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

// SyncOptions configures the auto-sync behavior.
type SyncOptions struct {
	RepoRoot   string
	UpdateDate bool
}

// Sync performs the auto-sync of project memory (memory.yaml).
// It discovers facts from the source code and updates the identity section.
func Sync(opts SyncOptions) error {
	repoRoot := opts.RepoRoot
	if repoRoot == "" {
		return fmt.Errorf("thoth sync: repo root required")
	}

	memoryPath := filepath.Join(repoRoot, ".thoth", "memory.yaml")
	data, err := os.ReadFile(memoryPath)
	if err != nil {
		return fmt.Errorf("thoth sync: fail to read memory: %w", err)
	}

	// Discover facts
	moduleCount := countSubdirs(filepath.Join(repoRoot, "internal"))
	binaryCount, binaryNames := listSubdirs(filepath.Join(repoRoot, "cmd"))
	testCount := estimateTestCount(repoRoot)
	lineCount := estimateLineCount(repoRoot)
	commandCount := estimateCommandCount(repoRoot)

	// Build the new Identity lines
	lines := strings.Split(string(data), "\n")
	newLines := make([]string, 0, len(lines))

	reBinary := regexp.MustCompile(`^binary_count:\s+\d+.*`)
	reModule := regexp.MustCompile(`^module_count:\s+\d+`)
	reTest := regexp.MustCompile(`^test_count:\s+\d+.*`)
	reLine := regexp.MustCompile(`^line_count:\s+.*`)
	reCommand := regexp.MustCompile(`^command_count:\s+\d+`)
	reDate := regexp.MustCompile(`^# Last updated:.*`)

	for _, line := range lines {
		updated := line
		switch {
		case reBinary.MatchString(line):
			updated = fmt.Sprintf("binary_count: %d (%s)", binaryCount, strings.Join(binaryNames, ", "))
		case reModule.MatchString(line):
			updated = fmt.Sprintf("module_count: %d", moduleCount)
		case reTest.MatchString(line):
			updated = fmt.Sprintf("test_count: %d+", testCount)
		case reLine.MatchString(line):
			updated = fmt.Sprintf("line_count: ~%s", formatNumber(lineCount))
		case reCommand.MatchString(line):
			updated = fmt.Sprintf("command_count: %d", commandCount)
		case opts.UpdateDate && reDate.MatchString(line):
			updated = fmt.Sprintf("# Last updated: %s", time.Now().Format("2006-01-02T15:04:05-07:00"))
		}
		newLines = append(newLines, updated)
	}

	if err := os.WriteFile(memoryPath, []byte(strings.Join(newLines, "\n")), 0o644); err != nil {
		return err
	}

	stele.Inscribe("thoth", stele.TypeThothSync, repoRoot, map[string]string{
		"modules": fmt.Sprintf("%d", moduleCount),
		"tests":   fmt.Sprintf("%d", testCount),
		"lines":   fmt.Sprintf("%d", lineCount),
	})
	return nil
}

func countSubdirs(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			count++
		}
	}
	return count
}

func listSubdirs(dir string) (int, []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return len(names), names
}

func estimateTestCount(root string) int {
	// Simple grep for "func Test" in Go files
	count := 0
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "func Test") {
				count++
			}
		}
		return nil
	})
	return count
}

func estimateLineCount(root string) int {
	total := 0
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "node_modules" || base == ".git" || base == "vendor" || base == "dist" || base == "out" || base == ".thoth" {
				return filepath.SkipDir
			}
			return nil
		}
		// Only count relevant source extensions
		ext := filepath.Ext(path)
		switch ext {
		case ".go", ".ts", ".js", ".tsx", ".jsx", ".md", ".html", ".css", ".yaml", ".yml":
			total += int(info.Size())
		}
		return nil
	})
	// 1 line per 50 bytes is very conservative for mixed code/MD
	return total / 65
}

func estimateCommandCount(root string) int {
	// Count cobra.Command definitions in cmd/
	count := 0
	_ = filepath.Walk(filepath.Join(root, "cmd"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "&cobra.Command") {
				count++
			}
		}
		return nil
	})
	return count
}

func formatNumber(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%d,%03d", n/1000, n%1000)
	}
	return fmt.Sprintf("%d", n)
}
