package lifecycleevidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"strings"
	"testing"
)

type itemFixture map[string]routerstore.Item

func (r itemFixture) Get(id string) (routerstore.Item, error) {
	v, ok := r[id]
	if !ok {
		return v, fmt.Errorf("unknown item")
	}
	return v, nil
}
func receiptFixture(t *testing.T) string {
	t.Helper()
	policy := maat.QualificationPolicy{ID: "inert-policy", Threshold: .9, BoundaryThreshold: .98, MinimumPass: 1, MinimumChanges: 1}
	raw, _ := json.Marshal(policy)
	sum := sha256.Sum256(raw)
	r := maat.LifecycleReceipt{Schema: maat.LifecycleSchema, Kind: "policy", Profile: maat.SourceProfileIdentity{Producer: "producer", ArtifactSHA: strings.Repeat("a", 64), ModelVersion: "v1", PromptSHA: strings.Repeat("b", 64), RecipeSHA: strings.Repeat("c", 64), PolicySHA: hex.EncodeToString(sum[:])}, Repo: "fixture/repo", HeadSHA: strings.Repeat("a", 40), Policy: policy, RunID: "inert-owned-run", Samples: []maat.LifecycleSample{}, SelectedIDs: []string{}}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestCanonicalLifecycleGetIsCustodyNotAuthenticatedAcceptance(t *testing.T) {
	item := routerstore.Item{ID: "review", Type: "review", Status: "closed", From: "claimed-reviewer", Instructions: receiptFixture(t)}
	resolver := Router{Reader: itemFixture{"review": item}}
	v, err := resolver.ResolveLifecycle(context.Background(), "review")
	if err != nil {
		t.Fatal(err)
	}
	e := maat.QualificationEvent{Schema: maat.LifecycleSchema, ReceiptID: v.ID, ReceiptSHA: v.SHA, Profile: v.Receipt.Profile, HeadSHA: v.Receipt.HeadSHA}
	state, err := maat.ReplayQualification(context.Background(), v.Receipt.Profile, []maat.QualificationEvent{e}, resolver)
	if err != nil || state.Status != "unqualified" || state.Verified != 0 || !strings.Contains(state.Reason, "custody") {
		t.Fatalf("custody became acceptance: %+v %v", state, err)
	}
}

func TestCanonicalLifecycleRejectsResultBorrowingLegacyUnknownAndChangedBodies(t *testing.T) {
	good := routerstore.Item{ID: "review", Type: "decision", Status: "closed", From: "claimed-reviewer", Instructions: receiptFixture(t)}
	for _, defect := range []string{"unknown", "result-only", "open", "proposal", "empty-from", "wrong-id", "unknown-schema", "duplicate-key"} {
		t.Run(defect, func(t *testing.T) {
			item := good
			switch defect {
			case "unknown":
				item.ID = "absent"
			case "result-only":
				item.Result = item.Instructions
				item.Instructions = "legacy review prose"
			case "open":
				item.Status = "open"
			case "proposal":
				item.Type = "proposal"
			case "empty-from":
				item.From = ""
			case "wrong-id":
				item.ID = "other"
			case "unknown-schema":
				item.Instructions = strings.Replace(item.Instructions, maat.LifecycleSchema, "unknown", 1)
			case "duplicate-key":
				item.Instructions = strings.Replace(item.Instructions, `"kind":"policy"`, `"kind":"policy","kind":"policy"`, 1)
			}
			if _, err := (Router{Reader: itemFixture{"review": item}}).ResolveLifecycle(context.Background(), "review"); err == nil {
				t.Fatal("unsupported canonical receipt admitted")
			}
		})
	}
	first, err := (Router{Reader: itemFixture{"review": good}}).ResolveLifecycle(context.Background(), "review")
	if err != nil {
		t.Fatal(err)
	}
	good.Instructions += " \n"
	second, err := (Router{Reader: itemFixture{"review": good}}).ResolveLifecycle(context.Background(), "review")
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA == second.SHA {
		t.Fatal("raw receipt whitespace drift lost")
	}
}

func TestCanonicalLifecycleRawCustodyUsesExactInstructionsBytesOnly(t *testing.T) {
	rawInstructions := receiptFixture(t) + " \n"
	item := routerstore.Item{ID: "exact-review", Type: "review", Status: "closed", From: "claimed-sender",
		Instructions: rawInstructions, Result: receiptFixture(t)}
	resolver := Router{Reader: itemFixture{"exact-review": item}}
	retained, err := resolver.ResolveLifecycleRaw(context.Background(), "exact-review")
	if err != nil {
		t.Fatal(err)
	}
	if retained.ID != item.ID || retained.Issuer != item.From || !bytes.Equal(retained.RawInstructions, []byte(rawInstructions)) {
		t.Fatalf("raw custody was parsed/re-encoded or borrowed from Result: %+v", retained)
	}
	resolved, err := resolver.ResolveLifecycle(context.Background(), "exact-review")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(rawInstructions))
	if resolved.ID != item.ID || resolved.Issuer != item.From || resolved.SHA != hex.EncodeToString(sum[:]) || !strings.Contains(resolved.Reason, "custody") {
		t.Fatalf("exact canonical custody became acceptance or lost byte hash: %+v", resolved)
	}
	// Exercise exported replay semantics instead of reaching into maat's
	// private acceptance bit: exact custody must remain unqualified.
	event := maat.QualificationEvent{Schema: maat.LifecycleSchema, ReceiptID: resolved.ID, ReceiptSHA: resolved.SHA,
		Profile: resolved.Receipt.Profile, HeadSHA: resolved.Receipt.HeadSHA}
	state, err := maat.ReplayQualification(context.Background(), resolved.Receipt.Profile, []maat.QualificationEvent{event}, resolver)
	if err != nil || state.Status != "unqualified" || state.Declared != 1 || state.Verified != 0 || !strings.Contains(state.Reason, "custody") {
		t.Fatalf("exact custody replay became acceptance: %+v %v", state, err)
	}

	for _, item := range []routerstore.Item{
		{ID: "result-only", Type: "review", Status: "closed", From: "claimed-sender", Instructions: "legacy prose", Result: receiptFixture(t)},
		{ID: "trailing", Type: "review", Status: "closed", From: "claimed-sender", Instructions: receiptFixture(t) + " {}"},
	} {
		t.Run(item.ID, func(t *testing.T) {
			bad := Router{Reader: itemFixture{item.ID: item}}
			if _, err := bad.ResolveLifecycle(context.Background(), item.ID); err == nil {
				t.Fatal("legacy Result or trailing JSON was accepted as canonical source custody")
			}
		})
	}
}
