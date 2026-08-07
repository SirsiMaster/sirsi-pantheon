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

// walkFileBudget bounds a single estimator walk. Sync runs from the SessionEnd
// hook under a 60s timeout, so an unbounded walk does not degrade the estimate —
// it loses the whole sync, silently, on every session exit. An approximate count
// that completes beats an exact one that never returns.
const walkFileBudget = 200_000

// skipWalkDir reports whether a directory is outside project source and must be
// pruned from an estimator walk.
//
// Both estimators MUST route through walkSource. estimateTestCount previously
// pruned NOTHING while estimateLineCount pruned six directories — the same file,
// two walks, two different ideas of scope. That inconsistency is the bug: a sync
// rooted at $HOME (findRepoRoot walks UP to ~/.thoth, which exists) then walked
// the entire home tree — ~/Library, ~/Downloads, every node_modules and every
// git worktree — and could not finish inside the hook's 60s budget. Measured
// 2026-08-07: killed at 120s, never completed, on every home-rooted session.
func skipWalkDir(base string) bool {
	switch base {
	case "node_modules", "vendor", "dist", "out", "build", "target", "Pods":
		return true
	// macOS home directories that are never project source but are enormous.
	case "Library", "Applications", "Downloads", "Pictures", "Movies", "Music", "Documents":
		return true
	}
	// Any dotted directory: .git, .thoth, caches, tool state, worktrees. Pruning
	// by shape rather than by name means a new cache dir does not reopen this bug.
	return len(base) > 1 && strings.HasPrefix(base, ".")
}

// walkSource visits files under root, pruning non-source directories and
// stopping after walkFileBudget files. Bounded by construction.
//
// Uses WalkDir, not Walk: Walk lstats EVERY entry to build the os.FileInfo it
// passes, which on a large tree is one syscall per file whether the caller needs
// it or not. WalkDir hands back a DirEntry already produced by ReadDir, so the
// stat happens only when a caller asks for it — see the size lookup in
// estimateLineCount, which stats only the handful of source extensions it counts.
func walkSource(root string, visit func(path string, d os.DirEntry)) {
	seen := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && skipWalkDir(filepath.Base(path)) {
				return filepath.SkipDir
			}
			return nil
		}
		seen++
		if seen > walkFileBudget {
			return filepath.SkipAll
		}
		visit(path, d)
		return nil
	})
}

func estimateTestCount(root string) int {
	// Simple grep for "func Test" in Go files
	count := 0
	walkSource(root, func(path string, _ os.DirEntry) {
		if !strings.HasSuffix(path, "_test.go") {
			return
		}
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "func Test") {
				count++
			}
		}
	})
	return count
}

func estimateLineCount(root string) int {
	total := 0
	walkSource(root, func(path string, d os.DirEntry) {
		// Only count relevant source extensions. The stat lives HERE, behind the
		// extension check, so a tree of non-source files costs zero stat calls.
		switch filepath.Ext(path) {
		case ".go", ".ts", ".js", ".tsx", ".jsx", ".md", ".html", ".css", ".yaml", ".yml":
			if info, err := d.Info(); err == nil {
				total += int(info.Size())
			}
		}
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
