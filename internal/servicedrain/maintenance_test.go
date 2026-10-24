package servicedrain

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func fixtureMarker() MaintenanceMarker {
	return MaintenanceMarker{1, strings.Repeat("1", 32), strings.Repeat("2", 64), 3, 4}
}

func TestStartupMaintenanceRequiresExactExplicitActivation(t *testing.T) {
	g, err := NewStartupMaintenance(123, "fixture-start", fixtureMarker(), 7, func() error { return nil })
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
	g, err := NewStartupMaintenance(123, "fixture-start", fixtureMarker(), 2, func() error {
		if changed {
			return errors.New("owned root substituted")
		}
		return nil
	})
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
