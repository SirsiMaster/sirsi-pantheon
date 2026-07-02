package tui

import (
	"fmt"
	"strings"
)

// Shared formatting helpers used across screens.

// fmtBytes renders a byte count as a human-readable size (KB/MB/GB/TB), matching
// the CLI's seba.FormatBytes convention so the TUI and CLI show the same numbers.
func fmtBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), units[exp])
}

// shortPath abbreviates a home-anchored path for display: the home prefix
// becomes "~". It never truncates here — the table column budget does that with
// an ellipsis — it only shortens the common, noisy home prefix.
func shortPath(p, home string) string {
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// ago renders a compact "12s ago" style relative label. It is intentionally
// coarse; the exact timestamp lives in the detail view.
func agoLabel(seconds int64) string {
	switch {
	case seconds < 60:
		return fmt.Sprintf("%ds ago", seconds)
	case seconds < 3600:
		return fmt.Sprintf("%dm ago", seconds/60)
	case seconds < 86400:
		return fmt.Sprintf("%dh ago", seconds/3600)
	default:
		return fmt.Sprintf("%dd ago", seconds/86400)
	}
}
