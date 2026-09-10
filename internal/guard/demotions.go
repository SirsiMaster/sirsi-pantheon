package guard

// demotions.go — ADR-064 §3: what Isis demoted (or held) recently, read back
// from the stele so `sirsi report` can show it and a "why is X slow" has an
// answer without grepping JSON.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

// DemotionSummary counts stele auto_renice / auto_renice_held rows newer than
// since, by process name. Zero-value when the stele is absent or unreadable
// (the report must never fail because the ledger is missing).
type DemotionSummary struct {
	Reniced map[string]int
	Held    map[string]int
	Since   time.Time
}

func (d DemotionSummary) Empty() bool { return len(d.Reniced) == 0 && len(d.Held) == 0 }

// RecentDemotions reads at most maxBytes from the tail of the stele.
func RecentDemotions(since time.Time, maxBytes int64) DemotionSummary {
	d := DemotionSummary{Reniced: map[string]int{}, Held: map[string]int{}, Since: since}
	entries, err := stele.TailByType(stele.TypeGuardAlert, maxBytes)
	if err != nil {
		return d
	}
	for _, e := range entries {
		ts, err := time.Parse(time.RFC3339, e.TS)
		if err != nil || ts.Before(since) {
			continue
		}
		name := e.Data["name"]
		switch e.Data["action"] {
		case "auto_renice":
			d.Reniced[name]++
		case "auto_renice_held":
			d.Held[name]++
		}
	}
	return d
}

// String renders one owner-readable block; "" when nothing happened.
func (d DemotionSummary) String() string {
	if d.Empty() {
		return ""
	}
	var sb strings.Builder
	win := time.Since(d.Since).Round(time.Hour)
	fmt.Fprintf(&sb, "𓁵 Isis auto-renice, last %s:", win)
	if len(d.Reniced) > 0 {
		sb.WriteString(" demoted " + joinCounts(d.Reniced))
	}
	if len(d.Held) > 0 {
		sb.WriteString(" · held " + joinCounts(d.Held))
	}
	sb.WriteString(" · undo: sirsi guard undo <pid>")
	return sb.String()
}

func joinCounts(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return m[keys[i]] > m[keys[j]] || (m[keys[i]] == m[keys[j]] && keys[i] < keys[j]) })
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s×%d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}
