package main

import (
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/completionproof/entryfixture"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// Exercise the actual MCP handler with real fixture ownership and a durable
// lease, rather than stopping at the shared proof helper. No registration
// loop, service, remote store, or live lane is started by this test.
func TestHandleCloseCompletionAuthorization(t *testing.T) {
	for _, tc := range entryfixture.Cases() {
		t.Run(tc.Name, func(t *testing.T) {
			f := entryfixture.New(t, tc)
			lease, err := f.Store.ClaimNext(entryfixture.Agent, 5*time.Minute)
			if err != nil || lease == nil {
				t.Fatalf("fixture claim: lease=%v err=%v", lease, err)
			}
			if lease.ItemID != f.ID {
				t.Fatalf("fixture claimed a different item: %q", lease.ItemID)
			}
			oldReg := reg
			reg = newRegistration()
			t.Cleanup(func() { reg = oldReg })
			reg.set("owned-fixture-thread", entryfixture.Agent)
			reg.recordClaim(f.ID, lease.Token, lease.Expires)
			before, err := f.Store.Get(f.ID)
			if err != nil {
				t.Fatal(err)
			}
			result, err := handleClose(map[string]interface{}{
				"id": f.ID, "proof": f.Proof, "result": "entry fixture completed",
			})
			if err != nil || result == nil {
				t.Fatalf("MCP transport result=%v err=%v", result, err)
			}
			var text strings.Builder
			for _, content := range result.Content {
				text.WriteString(content.Text)
			}
			if tc.Refusal != "" {
				if !result.IsError || !strings.Contains(text.String(), tc.Refusal) {
					t.Fatalf("want completion refusal %q, got %+v", tc.Refusal, result)
				}
				f.AssertUnchanged(t, before)
				if valid, err := f.Store.VerifyLease(f.ID, lease.Token); err != nil || !valid {
					t.Fatalf("proof refusal consumed the existing claim: valid=%v err=%v", valid, err)
				}
				f.AssertValidator(t)
				return
			}
			if result.IsError {
				t.Fatalf("completed proof refused: %s", text.String())
			}
			after, err := f.Store.Get(f.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Status != routerstore.StatusCompleted || after.Result != "entry fixture completed" || after.Closed == "" {
				t.Fatalf("actual leased completion did not commit: %+v", after)
			}
			f.AssertValidator(t)
		})
	}
}
