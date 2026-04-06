// Package output — suggestions.go
//
// Context-aware inline prediction engine for the Pantheon TUI.
// Generates fish-shell-style ghost text suggestions based on what the user
// has typed so far. Suggestions are prefix-matched by the bubbletea textinput
// component and rendered as dim text after the cursor.
//
// Tier 1: History matches (most recent first)
// Tier 2: Command tree completions (deity → subcommand → flags)
// Tier 3: Deity names and top-level aliases
// Tier 4: Intent keywords (natural language discovery)
package output

import (
	"sort"
	"strings"
)

// deityCommands describes the subcommands and flags available for a deity.
type deityCommands struct {
	Subcommands []string
	Flags       map[string][]string // subcommand → flags
}

// commandTree is the static Pantheon command registry.
// Mirrors the cobra tree in cmd/pantheon/ without importing it.
var commandTree = map[string]deityCommands{
	"ra": {
		Subcommands: []string{"health", "test", "lint", "task", "broadcast", "nightly", "status", "pipeline", "deploy", "kill", "collect", "watch"},
		Flags: map[string][]string{
			"deploy": {"--scope", "--iterm2", "--wait", "--record", "--dry-run"},
		},
	},
	"thoth": {
		Subcommands: []string{"init", "sync", "brain", "compact"},
		Flags: map[string][]string{
			"compact": {"--summary", "--max-age", "--max-keep"},
			"sync":    {"--since", "--dry-run"},
			"init":    {"--yes", "--name", "--language", "--version"},
			"brain":   {"--update", "--remove"},
		},
	},
	"anubis": {
		Subcommands: []string{"weigh", "judge", "ka", "mirror", "guard", "apps"},
		Flags: map[string][]string{
			"judge": {"--dry-run", "--confirm"},
			"ka":    {"--sudo"},
			"apps":  {"--ghosts", "--unused", "--size", "--uninstall", "--complete", "--window", "--yes", "--json"},
		},
	},
	"maat": {
		Subcommands: []string{"audit", "scales", "heal", "pulse"},
		Flags: map[string][]string{
			"audit":  {"--sudo", "--skip-test"},
			"scales": {"--fix"},
			"heal":   {"--fix", "--full"},
			"pulse":  {"--skip-test", "--json"},
		},
	},
	"seshat": {
		Subcommands: []string{"ingest", "export", "notebooklm", "list", "adapters", "profiles", "open", "auth", "sync", "mcp"},
		Flags: map[string][]string{
			"ingest":     {"--source", "--since", "--export", "--profile", "--all-profiles"},
			"notebooklm": {"--profile"},
			"open":       {"--profile", "--url"},
		},
	},
	"hapi": {
		Subcommands: []string{"scan", "profile", "compute"},
		Flags: map[string][]string{
			"compute": {"--tokenize"},
		},
	},
	"seba": {
		Subcommands: []string{"scan", "book", "fleet", "diagram"},
		Flags: map[string][]string{
			"book":    {"--output"},
			"fleet":   {"--containers", "--confirm-network", "--subnet"},
			"diagram": {"--type", "--html"},
		},
	},
	"net": {
		Subcommands: []string{"status", "align"},
	},
	"isis": {
		Subcommands: []string{"network", "heal"},
		Flags: map[string][]string{
			"network": {"--fix", "--rollback", "--json"},
			"heal":    {"--fix", "--full"},
		},
	},
	"osiris": {Subcommands: []string{}},
}

// topLevelCommands are available at the root (no deity prefix needed).
var topLevelCommands = []string{
	"scan", "ghosts", "dedup", "guard", "doctor", "mcp", "version", "help",
	"ra", "net", "thoth", "maat", "isis", "seshat",
	"anubis", "hapi", "seba", "osiris",
}

// buildSuggestions returns an ordered list of completion candidates for the
// current input. The textinput component prefix-matches these against the
// full input value, so each suggestion is a complete command string.
//
// Priority: history → command tree → top-level → intent keywords.
func buildSuggestions(input string, history []string) []string {
	if input == "" {
		return nil // textinput won't match on empty anyway
	}

	lower := strings.ToLower(input)
	tokens := strings.Fields(lower)
	hasTrailingSpace := len(input) > 0 && input[len(input)-1] == ' '

	var suggestions []string
	seen := make(map[string]bool)

	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			suggestions = append(suggestions, s)
		}
	}

	// Tier 1: History (most recent first, prefix-matched)
	for i := len(history) - 1; i >= 0; i-- {
		if strings.HasPrefix(strings.ToLower(history[i]), lower) {
			add(history[i])
		}
	}

	// Determine context
	switch {
	case len(tokens) == 0:
		// Shouldn't happen (input != ""), but be safe
		return suggestions

	case len(tokens) == 1 && !hasTrailingSpace:
		// Completing first word → all top-level commands
		for _, cmd := range topLevelCommands {
			add(cmd)
		}
		// Intent keywords for discovery
		for _, kws := range intentKeywords {
			for _, kw := range kws {
				add(kw)
			}
		}

	case len(tokens) >= 1:
		deity := tokens[0]
		dc, isDeity := commandTree[deity]

		if isDeity && (len(tokens) == 1 && hasTrailingSpace || len(tokens) == 2 && !hasTrailingSpace) {
			// Completing subcommand for this deity
			for _, sub := range dc.Subcommands {
				add(deity + " " + sub)
			}
		} else if isDeity && len(tokens) >= 2 {
			sub := tokens[1]
			// Completing flags for deity + subcommand
			if flags, ok := dc.Flags[sub]; ok {
				prefix := deity + " " + sub
				for _, flag := range flags {
					add(prefix + " " + flag)
				}
			}
		}
	}

	return suggestions
}

// deduplicateHistory extracts unique command strings from history entries,
// preserving the original casing, most recent occurrence wins.
func deduplicateHistory(history []historyEntry) []string {
	seen := make(map[string]bool)
	var result []string
	// Walk backward so most recent wins
	for i := len(history) - 1; i >= 0; i-- {
		cmd := history[i].command
		lower := strings.ToLower(cmd)
		if cmd != "" && !seen[lower] {
			seen[lower] = true
			result = append(result, cmd)
		}
	}
	// Reverse so most recent is last (natural history order)
	sort.SliceStable(result, func(i, j int) bool { return i > j })
	return result
}
