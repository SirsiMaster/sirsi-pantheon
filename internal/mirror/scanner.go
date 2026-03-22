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
				return nil // Skip non-regular files (symlinks, devices, etc.)
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

	// Phase 2: Two-stage hashing to minimize disk I/O
	// Stage A: Hash first 4KB of each file (fast pre-filter).
	//          Two files with different 4KB prefixes cannot be duplicates.
	// Stage B: Full SHA-256 only for files whose 4KB prefix matched.
	//          This avoids reading entire large files unless necessary.
	const partialSize = 4096

	partialGroups := make(map[string][]string) // partial hash → file paths
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
			go func(filePath string) {
				defer wg.Done()
				defer func() { <-sem }()

				hash, hashErr := hashFilePartial(filePath, partialSize)
				if hashErr != nil {
					return
				}
				// Key includes size for extra safety
				key := fmt.Sprintf("%d:%s", size, hash)
				mu.Lock()
				partialGroups[key] = append(partialGroups[key], filePath)
				mu.Unlock()
			}(p)
		}
	}
	wg.Wait()

	// Stage B: Full hash only for files that passed the partial filter
	hashGroups := make(map[string][]FileEntry)

	for _, paths := range partialGroups {
		if len(paths) < 2 {
			continue // Partial hash was unique — no match possible
		}

		for _, p := range paths {
			wg.Add(1)
			sem <- struct{}{}
			go func(filePath string) {
				defer wg.Done()
				defer func() { <-sem }()

				hash, hashErr := hashFileSHA256(filePath)
				if hashErr != nil {
					return
				}

				ext := strings.ToLower(filepath.Ext(filePath))
				info, _ := os.Stat(filePath)
				var modTime time.Time
				var fileSize int64
				if info != nil {
					modTime = info.ModTime()
					fileSize = info.Size()
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
			}(p)
		}
	}
	wg.Wait()

	// Phase 3: Build duplicate groups
	var groups []DuplicateGroup
	totalDuplicates := 0
	var totalWaste int64

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

// hashFilePartial hashes the first n bytes AND last n bytes of a file.
// Two files that share the same format header (e.g., MP4, JPEG) but have
// different content will match on the first bytes but differ at the end.
// Reading 8KB total instead of the entire file to eliminate non-matches.
func hashFilePartial(path string, n int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()

	// Hash first n bytes
	if _, copyErr := io.CopyN(h, f, int64(n)); copyErr != nil && copyErr != io.EOF {
		return "", copyErr
	}

	// Hash last n bytes (seek from end)
	info, statErr := f.Stat()
	if statErr != nil {
		return "", statErr
	}
	if info.Size() > int64(n*2) {
		// File is large enough to have distinct head/tail regions
		if _, seekErr := f.Seek(-int64(n), io.SeekEnd); seekErr == nil {
			if _, copyErr := io.CopyN(h, f, int64(n)); copyErr != nil && copyErr != io.EOF {
				return "", copyErr
			}
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
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
