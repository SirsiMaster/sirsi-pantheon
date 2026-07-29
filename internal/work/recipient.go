package work

import "strings"

// IsOwnerRecipient reports whether recipient names the durable human
// escalation lane. These aliases are intentionally not executable agents and
// therefore do not appear in agents.json.
func IsOwnerRecipient(recipient string) bool {
	switch strings.ToLower(strings.TrimSpace(recipient)) {
	case "user", "owner", "cylton", "sirsimaster", "cylton-collymore":
		return true
	default:
		return false
	}
}
