package dispatch

import (
	"strings"
	"testing"
	"time"
)

func TestOwnerActionsReplyAssignClose(t *testing.T) {
	f := testFacade(t)
	t.Setenv("SIRSI_ROUTER_URL", "http://fixture.invalid") // direct fixture store, cutover read semantics
	sent, err := f.Send("a", "owner", "operator request", "decision", "original request")
	if err != nil {
		t.Fatal(err)
	}
	it, _ := f.getFromStore(sent.ID)
	reply, err := f.ActAsOwner(OwnerAction{Action: "reply", ItemID: it.ID, Expected: ItemRevision(it), Message: "Reviewed. Keep the request open."})
	if err != nil {
		t.Fatal(err)
	}
	response, _ := f.getFromStore(reply.ResponseID)
	if response.From != "owner" || response.To != "a" || !strings.Contains(response.Instructions, it.ID) {
		t.Fatalf("bad response: %+v", response)
	}
	it, _ = f.getFromStore(it.ID)
	if it.Status != "open" {
		t.Fatal("reply closed source")
	}
	_, err = f.ActAsOwner(OwnerAction{Action: "assign", ItemID: it.ID, Expected: ItemRevision(it), Recipient: "b", Message: "Please handle this"})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := f.getFromStore(it.ID)
	if after.ID != it.ID || after.To != "b" || !strings.Contains(after.Instructions, "Owner assign") {
		t.Fatal("history/identity lost")
	}
	// A stale close cannot silently act on the pre-assignment selection.
	if _, err = f.ActAsOwner(OwnerAction{Action: "close", ItemID: it.ID, Expected: ItemRevision(it), Message: "Old selection"}); err == nil {
		t.Fatal("stale revision accepted")
	}
	_, err = f.ActAsOwner(OwnerAction{Action: "close", ItemID: after.ID, Expected: ItemRevision(after), Message: "Owner retires this request; not release acceptance"})
	if err != nil {
		t.Fatal(err)
	}
	closed, _ := f.getFromStore(it.ID)
	if closed.Status != "closed" || !strings.Contains(closed.Result, "Owner close") {
		t.Fatal("owner closure not audited")
	}
}
func TestOwnerActionsRefuseClaimedAndBadInputs(t *testing.T) {
	f := testFacade(t)
	t.Setenv("SIRSI_ROUTER_URL", "http://fixture.invalid")
	res, _ := f.Send("a", "b", "claimed request", "", "work")
	_, err := f.store.ClaimNext("b", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	it, _ := f.getFromStore(res.ID)
	for _, action := range []string{"reply", "assign", "close"} {
		if _, err := f.ActAsOwner(OwnerAction{Action: action, ItemID: it.ID, Expected: ItemRevision(it), Recipient: "a", Message: "do not steal"}); err == nil {
			t.Fatal("claimed work changed")
		}
	}
	for _, req := range []OwnerAction{{Action: "execute"}, {Action: "close", Message: ""}, {Action: "reply", Message: strings.Repeat("x", 32769)}} {
		if _, err := f.ActAsOwner(req); err == nil {
			t.Fatal("bad action accepted")
		}
	}
}
