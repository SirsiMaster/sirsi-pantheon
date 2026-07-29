package work

import "strings"

var ownerRecipients = []string{"user", "owner", "cylton", "sirsimaster", "cylton-collymore"}

// OwnerRecipients returns the canonical owner-escalation aliases.
func OwnerRecipients() []string {
	return append([]string(nil), ownerRecipients...)
}

// IsOwnerRecipient reports whether recipient names the durable human
// escalation lane. These aliases are intentionally not executable agents and
// therefore do not appear in agents.json.
func IsOwnerRecipient(recipient string) bool {
	normalized := strings.ToLower(strings.TrimSpace(recipient))
	for _, candidate := range ownerRecipients {
		if normalized == candidate {
			return true
		}
	}
	return false
}
