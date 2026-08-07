// Package rtk implements an output filter that reduces AI context window consumption
// by stripping ANSI escapes, deduplicating repeated lines, collapsing blank runs,
// and truncating oversized output with tail preservation.
//
// Subsumes the external RTK (Rust Token Killer) tool as native Go inside Pantheon.
package rtk

import (
	"strings"
)

// FilterConfig controls RTK output filter behavior.
type FilterConfig struct {
	StripANSI     bool // Remove ANSI escape sequences.
	Dedup         bool // Collapse consecutive identical lines.
	DedupWindow   int  // Number of recent lines to track for dedup (default 32).
	MaxLines      int  // Truncate output after N lines (0 = unlimited).
	MaxBytes      int  // Truncate output after N bytes (0 = unlimited).
	TailLines     int  // When truncating by MaxLines, keep last N lines as tail context.
	CollapseBlank bool // Collapse runs of blank lines to a single blank.
}

// FilterResult holds the filtered output and reduction statistics.
type FilterResult struct {
	Output        string  // The filtered text.
	OriginalBytes int     // Input size in bytes.
	FilteredBytes int     // Output size in bytes.
	LinesRemoved  int     // Total lines removed (dedup + truncation + blank collapse).
	DupsCollapsed int     // Lines removed specifically by deduplication.
	Truncated     bool    // Whether MaxLines or MaxBytes truncation was applied.
	Ratio         float64 // FilteredBytes / OriginalBytes (lower is better).
}

// Filter applies RTK transformations to raw tool output.
type Filter struct {
	cfg FilterConfig
}

// DefaultConfig returns a sensible default for AI context optimization.
// DefaultMaxBytes caps tool output sent to the AI context at 128 KB.
// Raw transcript writes are not intercepted by RTK (that is the Claude Code
// binary's responsibility), but every call through RTK.Apply respects this
// limit so the AI context never receives multi-megabyte tool outputs.
// R11: transcript-file growth requires Claude Code changes; this gates the
// AI-context surface that RTK owns.
const DefaultMaxBytes = 128 << 10 // 128 KB

func DefaultConfig() FilterConfig {
	return FilterConfig{
		StripANSI:     true,
		Dedup:         true,
		DedupWindow:   32,
		MaxLines:      0,
		MaxBytes:      DefaultMaxBytes,
		TailLines:     20,
		CollapseBlank: true,
	}
}

// New creates a new RTK filter with the given configuration.
func New(cfg FilterConfig) *Filter {
	return &Filter{cfg: cfg}
}

// Apply filters the given raw output and returns the result with statistics.
func (f *Filter) Apply(raw string) FilterResult {
	originalBytes := len(raw)
	if originalBytes == 0 {
		return FilterResult{Ratio: 1.0}
	}

	text := raw

	// Step 1: Strip ANSI escape sequences.
	if f.cfg.StripANSI {
		text = StripANSI(text)
	}

	lines := strings.Split(text, "\n")
	totalRemoved := 0
	dupsCollapsed := 0

	// Step 2: Deduplicate consecutive identical lines.
	if f.cfg.Dedup {
		ring := newDedupRing(f.cfg.DedupWindow)
		filtered := make([]string, 0, len(lines))
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			// Don't dedup blank lines (blank collapsing handles those).
			if trimmed == "" || !ring.seen(trimmed) {
				filtered = append(filtered, line)
			} else {
				dupsCollapsed++
			}
		}
		lines = filtered
	}

	// Step 3: Collapse runs of blank lines.
	if f.cfg.CollapseBlank {
		collapsed := make([]string, 0, len(lines))
		prevBlank := false
		for _, line := range lines {
			blank := strings.TrimSpace(line) == ""
			if blank && prevBlank {
				totalRemoved++
				continue
			}
			collapsed = append(collapsed, line)
			prevBlank = blank
		}
		lines = collapsed
	}

	// Step 4: Truncate by MaxLines with tail preservation.
	truncated := false
	if f.cfg.MaxLines > 0 && len(lines) > f.cfg.MaxLines {
		lines, totalRemoved = truncateWithTail(lines, f.cfg.MaxLines, f.cfg.TailLines, totalRemoved)
		truncated = true
	}

	totalRemoved += dupsCollapsed
	output := strings.Join(lines, "\n")

	// Step 5: Truncate by MaxBytes.
	if f.cfg.MaxBytes > 0 && len(output) > f.cfg.MaxBytes {
		output = output[:f.cfg.MaxBytes]
		truncated = true
	}

	filteredBytes := len(output)
	ratio := 1.0
	if originalBytes > 0 {
		ratio = float64(filteredBytes) / float64(originalBytes)
	}

	return FilterResult{
		Output:        output,
		OriginalBytes: originalBytes,
		FilteredBytes: filteredBytes,
		LinesRemoved:  totalRemoved,
		DupsCollapsed: dupsCollapsed,
		Truncated:     truncated,
		Ratio:         ratio,
	}
}

// truncateWithTail keeps the first (maxLines - tailLines) lines and the last tailLines lines,
// inserting a marker showing how many lines were omitted.
func truncateWithTail(lines []string, maxLines, tailLines, removed int) ([]string, int) {
	if tailLines <= 0 || tailLines >= maxLines {
		// No tail — simple truncation.
		removed += len(lines) - maxLines
		return lines[:maxLines], removed
	}

	headCount := maxLines - tailLines - 1 // -1 for the omission marker
	if headCount < 1 {
		headCount = 1
	}

	omitted := len(lines) - headCount - tailLines
	if omitted <= 0 {
		return lines, removed
	}

	result := make([]string, 0, headCount+1+tailLines)
	result = append(result, lines[:headCount]...)
	result = append(result, strings.Repeat("─", 40))
	result = append(result, lines[len(lines)-tailLines:]...)
	return result, removed + omitted
}
