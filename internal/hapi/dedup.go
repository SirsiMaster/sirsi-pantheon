package hapi

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// DuplicateGroup represents a set of files with identical content.
type DuplicateGroup struct {
	Hash   string   `json:"hash"`
	Size   int64    `json:"size"`
	Files  []string `json:"files"`
	Wasted int64    `json:"wasted"` // (count - 1) * size
}

// DedupResult contains all duplicate file groups found.
type DedupResult struct {
	Groups      []DuplicateGroup `json:"groups"`
	TotalWasted int64            `json:"total_wasted"`
	TotalFiles  int              `json:"total_files"`
	Scanned     int              `json:"scanned"`
}

// FindDuplicates scans directories for duplicate files.
// Uses size-first filtering then SHA-256 comparison.
// minSize: minimum file size to consider (default 1 MB).
func FindDuplicates(dirs []string, minSize int64) (*DedupResult, error) {
	result := &DedupResult{}

	// Phase 1: Group files by size (fast filter)
	sizeMap := make(map[int64][]string)
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			size := info.Size()
			if size >= minSize {
				sizeMap[size] = append(sizeMap[size], path)
				result.Scanned++
			}
			return nil
		})
	}

	// Phase 2: Hash files that share the same size (parallel)
	hashMap := make(map[string][]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for size, files := range sizeMap {
		if len(files) < 2 {
			continue // No duplicates possible
		}
		_ = size
		for _, f := range files {
			wg.Add(1)
			go func(path string) {
				defer wg.Done()
				hash, err := hashFile(path)
				if err != nil {
					return
				}
				mu.Lock()
				hashMap[hash] = append(hashMap[hash], path)
				mu.Unlock()
			}(f)
		}
	}
	wg.Wait()

	// Phase 3: Build duplicate groups
	for hash, files := range hashMap {
		if len(files) < 2 {
			continue
		}
		info, err := os.Stat(files[0])
		if err != nil {
			continue
		}
		size := info.Size()
		wasted := size * int64(len(files)-1)

		result.Groups = append(result.Groups, DuplicateGroup{
			Hash:   hash[:16], // Truncate for display
			Size:   size,
			Files:  files,
			Wasted: wasted,
		})
		result.TotalWasted += wasted
		result.TotalFiles += len(files)
	}

	// Sort by wasted space descending
	sort.Slice(result.Groups, func(i, j int) bool {
		return result.Groups[i].Wasted > result.Groups[j].Wasted
	})

	return result, nil
}

// hashFile computes SHA-256 of a file.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
