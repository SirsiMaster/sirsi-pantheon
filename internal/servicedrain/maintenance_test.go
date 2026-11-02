package servicedrain

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func fixtureMarker() MaintenanceMarker {
	return MaintenanceMarker{1, strings.Repeat("1", 32), strings.Repeat("a2", 32), 3, 4}
}

func TestActiveRelayDrainIsBoundedExactAndResumable(t *testing.T) {
	var paused bool
	hold := true
	release := make(chan struct{})
	drainResult := error(nil)
	drain := func(ctx context.Context) error {
		if !paused {
			t.Fatal("drain callback ran before the same-process pause")
		}
		if hold {
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if drainResult != nil {
			return drainResult
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	resume := func() { paused = false }
	g, err := NewActiveMaintenance(123, "active-start", 3, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.SetDrainHandlers(func(ctx context.Context) error {
		paused = true
		return drain(ctx)
	}, resume, func(expected MaintenanceReady) error {
		if expected.State != "relay_active" || expected.PID != 123 || expected.Started != "active-start" {
			return errors.New("active producer generation changed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	active := g.Status()
	if active.State != "relay_active" || active.PID != 123 || active.Started != "active-start" {
		t.Fatalf("active control lost producer binding: %+v", active)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := g.PrepareDrain(ctx, active); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed-out duty was acknowledged: %v", err)
	}
	if g.Status().State != "relay_active" || paused {
		t.Fatal("bounded refusal did not reopen the live producer")
	}
	hold = false
	drainResult = errors.New("local outcome custody unsettled")
	settlementCtx, settle := context.WithTimeout(context.Background(), time.Second)
	defer settle()
	if _, err := g.PrepareDrain(settlementCtx, g.Status()); err == nil {
		t.Fatal("local settlement debt acknowledged")
	}
	if paused || g.Status().State != "relay_active" {
		t.Fatal("settlement debt did not resume the producer")
	}
	drainResult = nil
	ctx2, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	ack, err := g.PrepareDrain(ctx2, g.Status())
	if err != nil || ack.State != "relay_drain_acknowledged" || !paused {
		t.Fatalf("exact settled drain was not acknowledged: %+v %v", ack, err)
	}
	wrong := ack
	wrong.Generation = strings.Repeat("f", 32)
	if _, err := g.ResumeDrain(wrong); err == nil || !paused {
		t.Fatal("foreign generation resumed the selected relay")
	}
	activeAgain, err := g.ResumeDrain(ack)
	if err != nil || activeAgain.State != "relay_active" || paused {
		t.Fatalf("exact acknowledgment did not resume: %+v %v", activeAgain, err)
	}
}

func TestStartupMaintenanceRequiresExactExplicitActivation(t *testing.T) {
	g, err := NewStartupMaintenance(123, "fixture-start", fixtureMarker(), 7, func(MaintenanceReady) error { return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	expected := g.Status()
	for _, field := range []string{"transaction", "root", "generation", "version", "pid", "start", "custody"} {
		t.Run(field, func(t *testing.T) {
			wrong := expected
			switch field {
			case "transaction":
				wrong.Marker.ID = strings.Repeat("3", 32)
			case "root":
				wrong.Marker.RootInode++
			case "generation":
				wrong.Generation = strings.Repeat("4", 32)
			case "version":
				wrong.Version++
			case "pid":
				wrong.PID++
			case "start":
				wrong.Started = "other"
			case "custody":
				wrong.Preserved = 0
			}
			if _, err := g.Activate(wrong); err == nil {
				t.Fatal("mixed activation accepted")
			}
			if g.Status() != expected {
				t.Fatal("refusal mutated gate")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if g.Status() != expected {
		t.Fatal("cancel/timer released startup")
	}
	after, err := g.Activate(expected)
	if err != nil {
		t.Fatal(err)
	}
	if after.Preserved != 7 || after.Version != 2 || after.State != "active_with_old_outcomes_unknown" {
		t.Fatal(after)
	}
	if err := g.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Activate(expected); err == nil {
		t.Fatal("stale activation repeated")
	}
}

func TestMaintenanceCanonicalWireIsClosed(t *testing.T) {
	m := fixtureMarker()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if got, err := DecodeMaintenance[MaintenanceMarker](raw); err != nil || got != m {
		t.Fatalf("%+v %v", got, err)
	}
	for _, bad := range []string{
		strings.Replace(string(raw), `"schema":1`, `"schema":1,"schema":1`, 1),
		strings.Replace(string(raw), `"schema":1`, `"Schema":1`, 1),
		strings.Replace(string(raw), `"schema":1,`, "", 1),
		strings.TrimSuffix(string(raw), "\n"), string(raw) + "{}\n", strings.Repeat(" ", 8193),
	} {
		if _, err := DecodeMaintenance[MaintenanceMarker]([]byte(bad)); err == nil {
			t.Fatalf("accepted noncanonical wire %q", bad)
		}
	}
}

func TestStartupMaintenanceRechecksContinuityBeforeActivation(t *testing.T) {
	changed := true
	g, err := NewStartupMaintenance(123, "fixture-start", fixtureMarker(), 2, func(MaintenanceReady) error {
		if changed {
			return errors.New("owned root substituted")
		}
		return nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := g.Status()
	if got, err := g.Activate(want); err == nil || got != want {
		t.Fatal("changed scope activated", got, err)
	}
	if g.Status() != want {
		t.Fatal("failed final continuity check opened gate")
	}
	changed = false
	if _, err := g.Activate(want); err != nil {
		t.Fatal(err)
	}
}
