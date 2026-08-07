package platform

import "strings"

// ParseDisabledLabels extracts the labels marked disabled in
// `launchctl print-disabled gui/<uid>` output. Format is one entry per line:
//
//	"label.name" => disabled
//	"label.name2" => enabled
//
// A single entry may hold SEVERAL labels space-joined inside one pair of
// quotes, e.g.
//
//	"ai.sirsi.triage ai.sirsi.pantheon ai.sirsi.gemma-broker" => disabled
//
// launchd stores whatever argument it was handed, so an unquoted "$@" in a
// caller writes all its labels as one key. Read verbatim, that key matches no
// real label and every service inside it reports as NOT disabled — an
// owner-quarantined job then looks revivable. Splitting the key on whitespace
// is what makes the quarantine visible.
//
// Returns labels unfiltered and in file order; callers apply their own prefix
// filtering.
func ParseDisabledLabels(output string) []string {
	var found []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasSuffix(line, "=> disabled") {
			continue
		}
		start := strings.Index(line, `"`)
		end := strings.LastIndex(line, `"`)
		if start < 0 || end <= start {
			continue
		}
		found = append(found, strings.Fields(line[start+1:end])...)
	}
	return found
}

// ManagedLabel reports whether a launchd label is one Sirsi manages.
func ManagedLabel(label string) bool {
	return strings.HasPrefix(label, "ai.sirsi.") || strings.HasPrefix(label, "actions.runner.")
}
