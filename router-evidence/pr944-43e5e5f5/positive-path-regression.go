package dispatch

import (
 "testing"
 "time"
)

func TestPR944VerifiedLiveClaimCanCloseThroughFacade(t *testing.T) {
 f := testFacade(t)
 sent, err := f.Send("a", "b", "claimed work", "review", "review this")
 if err != nil { t.Fatal(err) }
 lease, err := f.Store().ClaimNext("b", time.Minute)
 if err != nil { t.Fatal(err) }
 if lease.ItemID != sent.ID { t.Fatal("unexpected claim") }
 live, err := f.Store().VerifyLease(lease.ItemID, lease.Token)
 if err != nil || !live { t.Fatalf("live verification: %v %v", live, err) }
 if err := f.CloseItem("b", lease.ItemID, "verified result"); err != nil {
  t.Fatalf("PR944 normal MCP close path refuses its valid live claim: %v", err)
 }
}
