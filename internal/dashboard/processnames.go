package dashboard

import (
	"path/filepath"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/deity"
)

// PantheonComponentLabels returns the canonical process inventory labels used
// by every dashboard producer. The returned map is fresh so callers cannot
// mutate the shared registry.
func PantheonComponentLabels() map[string]string {
	labels := map[string]string{
		"sirsi":          "☥ Sirsi",
		"sirsi-menubar":  "☥ Sirsi Menubar",
		"sirsi-agent":    "🤖 Agent",
		"pantheon-agent": "🤖 Agent",
		"guard":          "🛡 Guard",
	}
	for _, component := range deity.Roster {
		labels[component.Key] = component.Glyph + " " + component.Name
	}
	return labels
}

// ProcessLabelsFromCommandOutput matches known executable basenames exactly,
// preventing unrelated wrappers with similar names from appearing as
// Pantheon components. Duplicate processes produce one label.
func ProcessLabelsFromCommandOutput(output string, labels map[string]string) []string {
	seen := make(map[string]struct{}, len(labels))
	active := make([]string, 0, len(labels))
	for _, line := range strings.Split(output, "\n") {
		command := strings.TrimSpace(line)
		if command == "" {
			continue
		}
		name := strings.ToLower(filepath.Base(command))
		label, ok := labels[name]
		if !ok {
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		seen[name] = struct{}{}
		active = append(active, label)
	}
	return active
}
