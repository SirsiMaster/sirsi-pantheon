package servicedrain

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestMaintenanceIdentityRejectsMalformedAuthority(t *testing.T) {
	valid := fixtureMarker()
	if err := valid.Validate(); err != nil {
		t.Fatalf("fixture marker rejected: %v", err)
	}
	cases := []struct {
		name   string
		change func(*MaintenanceMarker)
	}{
		{"schema", func(m *MaintenanceMarker) { m.Schema = 2 }},
		{"short transaction", func(m *MaintenanceMarker) { m.ID = "01" }},
		{"uppercase digest", func(m *MaintenanceMarker) { m.PlanSHA256 = strings.ToUpper(m.PlanSHA256) }},
		{"zero root inode", func(m *MaintenanceMarker) { m.RootInode = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			marker := valid
			tc.change(&marker)
			if err := marker.Validate(); err == nil {
				t.Fatal("invalid transaction/root binding accepted")
			}
		})
	}
	if _, err := NewStartupMaintenance(1, "", valid, 0, func(MaintenanceReady) error { return nil }, nil); err == nil {
		t.Fatal("invalid startup identity accepted")
	}
	if _, err := NewStartupMaintenance(2, "fixture", valid, -1, func(MaintenanceReady) error { return nil }, nil); err == nil {
		t.Fatal("negative custody count accepted")
	}
	if _, err := NewStartupMaintenance(2, "fixture", valid, 0, nil, nil); err == nil {
		t.Fatal("missing continuity verifier accepted")
	}
}

func TestMaintenanceSnapshotReobservesCustodyAndKeepsFailuresClosed(t *testing.T) {
	observed := 4
	observeErr := error(nil)
	g, err := NewStartupMaintenance(123, "fixture-start", fixtureMarker(), 3, func(MaintenanceReady) error { return nil }, func() (int, error) {
		return observed, observeErr
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := g.Snapshot()
	if err != nil || first.Preserved != 4 || first.Version != 2 {
		t.Fatalf("updated custody snapshot = %+v, %v", first, err)
	}
	observeErr = errors.New("fixture custody read failed")
	failed, err := g.Snapshot()
	if !errors.Is(err, observeErr) || failed != first || g.Status() != first {
		t.Fatalf("failed observation changed authority: %+v, %v", failed, err)
	}
	observeErr = nil
	observed = MaxCustodyEnvelopes + 1
	if got, err := g.Snapshot(); err == nil || got != first || g.Status() != first {
		t.Fatalf("out-of-range observation changed authority: %+v, %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled wait = %v", err)
	}
	if g.Status() != first {
		t.Fatal("cancellation opened startup gate")
	}
}

func TestMaintenanceActivationVerifierFailureDoesNotReleaseWaiters(t *testing.T) {
	verifyErr := errors.New("fixture root continuity failure")
	g, err := NewStartupMaintenance(123, "fixture-start", fixtureMarker(), 5, func(MaintenanceReady) error { return verifyErr }, nil)
	if err != nil {
		t.Fatal(err)
	}
	before := g.Status()
	if after, err := g.Activate(before); !errors.Is(err, verifyErr) || after != before || g.Status() != before {
		t.Fatalf("failed continuity check changed state: %+v, %v", after, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	select {
	case <-g.started:
		t.Fatal("failed continuity check closed the activation channel")
	default:
	}
	cancel()
	if err := g.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait released after failed verification: %v", err)
	}
}
