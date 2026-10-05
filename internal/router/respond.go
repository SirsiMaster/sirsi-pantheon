package router

import (
	"fmt"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
)

// RespondToItem is the atomic request→response primitive (owner rule
// 2026-06-15: a request ALWAYS requires a response). A close-with-Result is
// audit-only — the sender is not notified; only a fresh inbound wakes them.
// This does both: notify the requester with a fresh type:decision inbound
// carrying the result, then close the item with the same result as an
// implicit ack. Shared by the CLI (`sirsi router respond`) and the MCP
// router_respond tool so both surfaces behave identically (ADR-036).
//
// Notify happens BEFORE close: the file/store era has no cross-row
// transaction, so one order has to be the survivable one on partial failure.
// Notifying first fails safe — the request stays OPEN until answered, so a
// retry is always available, and a retry is harmless because the store's
// idem_key dedupes an identical resend instead of double-notifying. Closing
// first could strand the requester if the notify step then failed.
func RespondToItem(f *dispatch.Facade, actor, id, title, result string) (notifyTo string, notifyDeduped bool, notifyID string, err error) {
	if result == "" {
		return "", false, "", fmt.Errorf("result is required: a response with no body answers nothing")
	}
	item, err := f.Get(id)
	if err != nil {
		return "", false, "", err
	}
	if item.From == "" {
		return "", false, "", fmt.Errorf("item %s has no from: — cannot notify the requester", id)
	}
	if err := f.ValidateAgent("acting agent", actor); err != nil {
		return "", false, "", err
	}

	if title == "" {
		t := item.Title
		if len(t) > 80 {
			t = t[:80]
		}
		title = "RESPONSE: " + t
	}
	body := fmt.Sprintf("RESPONSE to your request %q (your item %s, closed with this as the Result).\n\n%s",
		item.Title, id, result)

	res, err := f.Send(actor, item.From, title, "decision", body)
	if err != nil {
		return item.From, false, "", fmt.Errorf("notifying %s FAILED — %s left OPEN, nothing lost, rerun respond: %w", item.From, id, err)
	}

	// A respond close is by definition an acknowledgement — the notification
	// above IS the response — so it carries --ack semantics past the ADR-037
	// proof gate.
	if err := f.CloseItem(actor, id, result); err != nil {
		return item.From, res.Deduped, res.ID, fmt.Errorf("%s notified via %s but closing %s FAILED — rerun respond, the resend dedupes: %w", item.From, res.ID, id, err)
	}
	return item.From, res.Deduped, res.ID, nil
}
