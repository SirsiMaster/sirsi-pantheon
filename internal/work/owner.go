package work

import "strings"

var ownerRecipients = []string{
	"user",
	"owner",
	"cylton",
	"sirsimaster",
	"cylton-collymore",
}

// OwnerRecipients returns the reserved human-owner inbox aliases. These
// recipients are intentionally absent from agents.json because the owner queue
// has no executable watcher.
func OwnerRecipients() []string {
	out := make([]string, len(ownerRecipients))
	copy(out, ownerRecipients)
	return out
}

// IsOwnerRecipient reports whether to addresses the human owner rather than an
// executable agent.
func IsOwnerRecipient(to string) bool {
	to = strings.ToLower(strings.TrimSpace(to))
	for _, owner := range ownerRecipients {
		if to == owner {
			return true
		}
	}
	return false
}
