package routerstore

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// RecipientDeliveryContract is optional and additive. Missing contracts retain
// legacy delivery. This field controls inbox admission, never authentication,
// registration, read/ack, or whether a runtime is alive.
type RecipientDeliveryContract struct {
	Version int    `json:"version"`
	Mode    string `json:"mode"` // inbox or actor-only; no automatic custodian routing
}

// ValidateRecipientDelivery reads the authoritative registry, not a sender's
// proposed delivery fields. An empty path preserves embedding/test compatibility.
func ValidateRecipientDelivery(path, to string) error {
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("recipient registry: %w", err)
	}
	var reg struct {
		Agents map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
			Wake        struct {
				Mechanism string `json:"mechanism"`
			} `json:"wake"`
			Contract *RecipientDeliveryContract `json:"delivery_contract,omitempty"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(b, &reg); err != nil {
		return fmt.Errorf("recipient registry: %w", err)
	}
	a, ok := reg.Agents[to]
	if !ok {
		return fmt.Errorf("undeclared recipient %q", to)
	}
	if a.Contract != nil {
		if a.Contract.Version != 1 {
			return fmt.Errorf("recipient %q unsupported delivery contract version %d", to, a.Contract.Version)
		}
		switch a.Contract.Mode {
		case "inbox":
			return nil
		case "actor-only":
			return fmt.Errorf("recipient %q is actor-only: no inbox endpoint; preserve source and route only to an explicitly declared custodian", to)
		default:
			return fmt.Errorf("recipient %q unsupported delivery mode %q", to, a.Contract.Mode)
		}
	}
	// Compatibility with the existing explicit Horus declaration. Neither wake
	// none nor an absent consumer alone makes a legacy mailbox non-deliverable.
	d := strings.ToLower(a.Description)
	if a.Type == "service" && a.Wake.Mechanism == "none" && strings.Contains(d, "actor-only") && strings.Contains(d, "never route inbox") {
		return fmt.Errorf("recipient %q is explicitly actor-only: no inbox endpoint; preserve source and use a declared custodian", to)
	}
	return nil
}
