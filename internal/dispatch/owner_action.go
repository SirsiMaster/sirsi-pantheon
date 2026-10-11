package dispatch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// ItemRevision binds an operator's selection to the item they actually read.
func ItemRevision(it work.Item) string {
	b, _ := json.Marshal([]string{it.ID, it.From, it.To, it.Title, it.Status, it.Instructions, it.BlockedBy, it.AckedAt})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

type OwnerAction struct {
	Action    string `json:"action"`
	ItemID    string `json:"item_id"`
	Expected  string `json:"expected"`
	Recipient string `json:"recipient,omitempty"`
	Message   string `json:"message"`
}
type OwnerActionResult struct {
	ItemID     string `json:"item_id"`
	Action     string `json:"action"`
	ResponseID string `json:"response_id,omitempty"`
	Recipient  string `json:"recipient,omitempty"`
	Status     string `json:"status"`
}

// ActAsOwner is exclusively for a human-operated, authenticated owner surface.
// It is deliberately distinct from agent CloseItem/Reassign authority. The
// HTTP adapter fixes the actor, refuses foreign origins and binds a CSRF token.
// Reply does NOT close a request: delivery and owner disposition are separate.
func (f *Facade) ActAsOwner(req OwnerAction) (OwnerActionResult, error) {
	out := OwnerActionResult{ItemID: req.ItemID, Action: req.Action}
	if req.Action != "reply" && req.Action != "assign" && req.Action != "close" {
		return out, fmt.Errorf("unknown owner action")
	}
	if strings.TrimSpace(req.Message) == "" || len(req.Message) > 32768 {
		return out, fmt.Errorf("a message/disposition of 1–32768 bytes is required")
	}
	if err := f.ValidateAgent("owner", "owner"); err != nil {
		return out, err
	}
	it, err := f.getFromStore(req.ItemID)
	if err != nil {
		return out, err
	}
	if req.Expected == "" || ItemRevision(it) != req.Expected {
		return out, fmt.Errorf("item changed since it was read; refresh and review it before acting")
	}
	if it.Status != "open" {
		return out, fmt.Errorf("item is %s; claimed/closed work cannot be changed from this surface", it.Status)
	}
	audit := fmt.Sprintf("Owner %s via router console on item %s. surface=router-board actor=owner\n\n%s", req.Action, it.ID, strings.TrimSpace(req.Message))
	switch req.Action {
	case "reply":
		res, e := f.Send("owner", it.From, "OWNER RESPONSE: "+it.Title, "decision", audit)
		if e != nil {
			return out, e
		}
		out.ResponseID = res.ID
		out.Recipient = it.From
		sent, e := f.getFromStore(res.ID)
		if e != nil {
			return out, fmt.Errorf("reply %s sent; readback failed: %w", res.ID, e)
		}
		if sent.From != "owner" {
			return out, fmt.Errorf("reply identity readback mismatch")
		}
	case "assign":
		to, e := f.resolveRecipient(strings.TrimSpace(req.Recipient))
		if e != nil {
			return out, e
		}
		if e = f.ValidateAgent("recipient", to); e != nil {
			return out, e
		}
		// Atomic store guard rejects a concurrent claim, close or reassignment.
		if e = f.store.ReassignItem(it.ID, it.To, to, "\n\n---\n"+audit); e != nil {
			return out, e
		}
		f.store.NotifyAgent(to)
		out.Recipient = to
	case "close":
		// CloseItem's atomic status guard refuses a live worker claim. This records
		// a human disposition, not a fabricated completion proof or release bind.
		if e := f.store.CloseItem(it.ID, audit); e != nil {
			return out, e
		}
	}
	after, e := f.getFromStore(it.ID)
	if e != nil {
		return out, fmt.Errorf("action submitted; readback failed: %w", e)
	}
	out.Status = after.Status
	if req.Action == "close" && after.Status != "closed" {
		return out, fmt.Errorf("closure readback mismatch")
	}
	if req.Action == "assign" && after.To != out.Recipient {
		return out, fmt.Errorf("assignment submitted but recipient changed before readback; refresh")
	}
	return out, nil
}
