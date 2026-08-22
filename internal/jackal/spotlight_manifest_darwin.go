//go:build darwin

package jackal

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// SpotlightManifest consumes the index macOS already owns. It never recursively
// walks a directory; mdfind discovers indexed descendants and bounded lstat
// calls read size/type from those results.
type SpotlightManifest struct {
	mu    sync.Mutex
	sizes map[string]spotlightSize
}

type spotlightSize struct {
	bytes int64
	files int
}

func NewPlatformManifest() Manifest {
	if _, err := exec.LookPath("mdfind"); err != nil {
		return nil
	}
	return &SpotlightManifest{sizes: make(map[string]spotlightSize)}
}

func (m *SpotlightManifest) Exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func (m *SpotlightManifest) Glob(pattern string) []string {
	// Jackal's path rules use only exact paths or a single bounded child glob.
	// filepath.Glob does not recurse and therefore does not create a competing
	// filesystem index.
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	return matches
}

func (m *SpotlightManifest) DirSize(dir string) int64 {
	size, _ := m.DirSizeAndCount(dir)
	return size
}

func (m *SpotlightManifest) DirSizeAndCount(dir string) (int64, int) {
	dir = filepath.Clean(dir)
	m.mu.Lock()
	if cached, ok := m.sizes[dir]; ok {
		m.mu.Unlock()
		return cached.bytes, cached.files
	}
	m.mu.Unlock()

	paths := spotlightPaths(dir, "kMDItemFSSize >= 0")
	var total int64
	count := 0
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil || info.IsDir() {
			continue
		}
		total += info.Size()
		count++
	}

	m.mu.Lock()
	m.sizes[dir] = spotlightSize{bytes: total, files: count}
	m.mu.Unlock()
	return total, count
}

func (m *SpotlightManifest) FindDirsNamed(root, name string, maxDepth int) []string {
	query := fmt.Sprintf("kMDItemFSName == %qcd", name)
	paths := spotlightPaths(root, query)
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || filepath.Base(path) != name {
			continue
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		depth := strings.Count(rel, string(filepath.Separator)) + 1
		if depth <= maxDepth {
			result = append(result, path)
		}
	}
	sort.Strings(result)
	return result
}

func spotlightPaths(root, query string) []string {
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return nil
	}
	out, err := exec.Command("mdfind", "-0", "-onlyin", root, query).Output()
	if err != nil {
		return nil
	}
	items := bytes.Split(out, []byte{0})
	paths := make([]string, 0, len(items))
	for _, item := range items {
		if path := strings.TrimSpace(string(item)); path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}
