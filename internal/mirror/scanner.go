package mirror

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Scan walks the given directories and finds duplicate files.
// Uses a two-phase approach: group by size first (instant), then hash to confirm.
func Scan(opts ScanOptions) (*MirrorResult, error) {
	start := time.Now()

	if len(opts.Paths) == 0 {
		return nil, fmt.Errorf("no paths specified")
	}

	// Normalize protected dirs to absolute paths
	protectedAbs := make(map[string]bool)
	for _, d := range opts.ProtectDirs {
		abs, absErr := filepath.Abs(d)
		if absErr == nil {
			protectedAbs[abs] = true
		}
	}

	// Phase 1: Walk directories, collect files grouped by size
	sizeGroups := make(map[int64][]string)
	totalScanned := 0

	for _, root := range opts.Paths {
		absRoot, absErr := filepath.Abs(root)
		if absErr != nil {
			return nil, fmt.Errorf("resolve path %s: %w", root, absErr)
		}

		walkErr := filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip inaccessible files
			}
			if info.IsDir() {
				// Skip hidden directories (except the root)
				base := filepath.Base(path)
				if strings.HasPrefix(base, ".") && path != absRoot {
					return filepath.SkipDir
				}
				return nil
			}
			if !info.Mode().IsRegular() {
				return nil // Skip non-regular files
			}
			if !opts.FollowLinks && info.Mode()&os.ModeSymlink != 0 {
				return nil
			}

			size := info.Size()
			if size == 0 {
				return nil // Skip empty files
			}
			if opts.MinSize > 0 && size < opts.MinSize {
				return nil
			}
			if opts.MaxSize > 0 && size > opts.MaxSize {
				return nil
			}

			// Media type filter
			if opts.MediaFilter != "" {
				ext := strings.ToLower(filepath.Ext(path))
				if ClassifyMedia(ext) != opts.MediaFilter {
					return nil
				}
			}

			sizeGroups[size] = append(sizeGroups[size], path)
			totalScanned++
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("walk %s: %w", root, walkErr)
		}
	}

	// Phase 2: For size groups with multiple files, compute SHA-256
	// Only hash files that share the same size (massive speedup)
	hashGroups := make(map[string][]FileEntry)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8) // Limit concurrent I/O

	for size, paths := range sizeGroups {
		if len(paths) < 2 {
			continue // No duplicates possible if unique size
		}

		for _, p := range paths {
			wg.Add(1)
			sem <- struct{}{}
			go func(filePath string, fileSize int64) {
				defer wg.Done()
				defer func() { <-sem }()

				hash, hashErr := hashFileSHA256(filePath)
				if hashErr != nil {
					return // Skip unhashable files
				}

				ext := strings.ToLower(filepath.Ext(filePath))
				info, _ := os.Stat(filePath)
				var modTime time.Time
				if info != nil {
					modTime = info.ModTime()
				}

				entry := FileEntry{
					Path:      filePath,
					Size:      fileSize,
					ModTime:   modTime,
					SHA256:    hash,
					MediaType: ClassifyMedia(ext),
				}

				// Check if file is in a protected directory
				for protDir := range protectedAbs {
					if strings.HasPrefix(filePath, protDir) {
						entry.IsProtected = true
						break
					}
				}

				mu.Lock()
				hashGroups[hash] = append(hashGroups[hash], entry)
				mu.Unlock()
			}(p, size)
		}
	}
	wg.Wait()

	// Phase 3: Build duplicate groups
	var groups []DuplicateGroup
	totalDuplicates := 0
	var totalWaste int64
	groupID := 0

	for hash, files := range hashGroups {
		if len(files) < 2 {
			continue
		}

		// Sort files by recommendation priority
		sortByPriority(files)

		var waste int64
		for i := 1; i < len(files); i++ {
			waste += files[i].Size
		}

		group := DuplicateGroup{
			ID:          fmt.Sprintf("dup-%s", hash[:12]),
			Files:       files,
			MatchType:   MatchExact,
			Recommended: 0, // First file after sort = recommended keeper
			Confidence:  1.0,
			WasteBytes:  waste,
		}

		groups = append(groups, group)
		totalDuplicates += len(files) - 1
		totalWaste += waste
		groupID++
	}

	// Sort groups by waste (largest savings first)
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].WasteBytes > groups[j].WasteBytes
	})

	result := &MirrorResult{
		Groups:          groups,
		TotalScanned:    totalScanned,
		TotalDuplicates: totalDuplicates,
		TotalWasteBytes: totalWaste,
		UniqueFiles:     totalScanned - totalDuplicates,
		ScanDuration:    time.Since(start),
		DirsScanned:     opts.Paths,
	}

	return result, nil
}

// sortByPriority sorts files so the best keeper is first.
// Priority: protected > shallowest path > oldest > largest
func sortByPriority(files []FileEntry) {
	sort.SliceStable(files, func(i, j int) bool {
		a, b := files[i], files[j]

		// Protected files always win
		if a.IsProtected != b.IsProtected {
			return a.IsProtected
		}

		// Shallower path depth = more intentionally placed
		depthA := strings.Count(a.Path, string(filepath.Separator))
		depthB := strings.Count(b.Path, string(filepath.Separator))
		if depthA != depthB {
			return depthA < depthB
		}

		// Older files are likely originals
		if !a.ModTime.Equal(b.ModTime) {
			return a.ModTime.Before(b.ModTime)
		}

		// Larger file = less compressed = higher quality
		return a.Size > b.Size
	})
}

// hashFileSHA256 computes SHA-256 of a file.
// Reads the full file — callers should pre-filter by size to avoid unnecessary I/O.
func hashFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, copyErr := io.Copy(h, f); copyErr != nil {
		return "", copyErr
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// FormatBytes converts bytes to human-readable string.
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
