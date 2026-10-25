package setup

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type appAdmissionFixture struct {
	f            *maintenanceFixture
	c            ServiceController
	state        MaintenanceAppAdmission
	prepared     MaintenanceStatus
	fault        string
	afterRunning func()
}

func newAppAdmissionFixture(t *testing.T) *appAdmissionFixture {
	f, _ := newMaintenanceFixture(t)
	a := &appAdmissionFixture{f: f}
	var mutex sync.Mutex
	f.d.lock = func(op func() error) error {
		if !mutex.TryLock() {
			return errors.New("selected service operation busy")
		}
		defer mutex.Unlock()
		return op()
	}
	f.d.appLoad = func() (MaintenanceAppAdmission, error) {
		if a.fault == "read" {
			return a.state, errors.New("namespace changed")
		}
		return a.state, nil
	}
	f.d.appSync = func(expected MaintenanceAppAdmission) error {
		if expected != a.state || a.fault == "sync" {
			return errors.New("admission durability unverified")
		}
		return nil
	}
	f.d.appPersist = func(old, next MaintenanceAppAdmission) error {
		if old != a.state {
			return errors.New("admission CAS")
		}
		if err := validAppStateTransition(old, next); err != nil {
			return err
		}
		if a.fault == "fence" && next.State == "fenced" {
			return errors.New("inert fence fsync failure")
		}
		a.state = next
		if next.State == "running" && a.afterRunning != nil {
			a.afterRunning()
		}
		if a.fault == "after-fence" && next.State == "fenced" {
			return errors.New("inert fence readback failure after durable write")
		}
		return nil
	}
	f.d.prepare = func(s MaintenanceStatus) error {
		a.prepared = s
		a.prepared.Revision++
		a.prepared.Phase = "prepared"
		f.events = append(f.events, "prepare")
		return nil
	}
	f.d.load = func() (MaintenanceStatus, error) {
		if a.prepared.ID != "" && a.prepared.ID != f.state.ID {
			return a.prepared, nil
		}
		return f.state, nil
	}
	f.d.loadRecovery = func(id, digest string) (MaintenanceStatus, error) {
		if f.state.ID != id && a.prepared.ID == id {
			f.state = a.prepared
		}
		return f.state, nil
	}
	f.d.pending = func(s MaintenanceStatus) (bool, error) { return a.prepared.ID == s.ID && s.ID != "", nil }
	f.d.clearPreparation = func(MaintenanceStatus) error { a.prepared = MaintenanceStatus{}; return nil }
	f.d.abortVerify = func(context.Context, MaintenanceStatus) error { return nil }
	f.d.abortFinish = func(context.Context, MaintenanceStatus) error { a.prepared = MaintenanceStatus{}; return nil }
	a.c = ServiceController{Backend: f, Authorize: func(context.Context, string) error { return nil }}
	return a
}
func (a *appAdmissionFixture) preview(t *testing.T) MaintenanceAppPreview {
	t.Helper()
	value, err := a.c.RunLegacyAppMaintenance(context.Background(), LegacyAppRequest{Profile: RootRelayMaintenanceProfile, EngineSHA256: strings.Repeat("a", 64), Operation: "preview", Legacy: a.f.plan.LegacyPayload})
	if err != nil {
		t.Fatal(err)
	}
	return value.(MaintenanceAppPreview)
}
func appMutation(p MaintenanceAppPreview) LegacyAppRequest {
	return LegacyAppRequest{Profile: RootRelayMaintenanceProfile, EngineSHA256: strings.Repeat("a", 64), Operation: "confirm", Legacy: p.Plan.LegacyPayload, PlanSHA256: p.Plan.SHA256, TicketJSON: p.TicketJSON}
}
func (a *appAdmissionFixture) resolve(p MaintenanceAppPreview) (MaintenanceAppResolution, error) {
	value, err := a.c.RunLegacyAppMaintenance(context.Background(), LegacyAppRequest{Profile: RootRelayMaintenanceProfile, EngineSHA256: strings.Repeat("a", 64), Operation: "resolve-operation", TicketJSON: p.TicketJSON})
	if value == nil {
		return MaintenanceAppResolution{}, err
	}
	return value.(MaintenanceAppResolution), err
}
func (a *appAdmissionFixture) effects(name string) int {
	n := 0
	for _, event := range a.f.events {
		if event == name {
			n++
		}
	}
	return n
}

func TestLegacyAppAdmissionTruthMatrix(t *testing.T) {
	for _, name := range []string{"oldA-does-not-resolve-B", "empty-before-journal-late-original", "running-lock-busy", "completed-reply-lost", "prepare-before-journal", "fence-reply-lost", "fence-write-or-readback-fails", "delayed-preview-superseded", "changed-binding", "concurrent-confirms", "agent-instance-changed", "cancel-after-running-intent", "old-detail-evicted"} {
		t.Run(name, func(t *testing.T) {
			a := newAppAdmissionFixture(t)
			p := a.preview(t)
			ctx := context.Background()
			switch name {
			case "oldA-does-not-resolve-B":
				old := a.f.plan
				old.LegacyPayload += "-older"
				old.SHA256 = maintenanceDigest(old)
				a.f.state = MaintenanceStatus{Schema: 1, ID: strings.Repeat("b", 32), Revision: 1, Plan: old, Phase: "completed"}
				view, err := a.c.RunLegacyAppMaintenance(ctx, LegacyAppRequest{Profile: RootRelayMaintenanceProfile, EngineSHA256: strings.Repeat("a", 64), Operation: "status"})
				if err != nil || view.(MaintenanceAppView).Status.ID != a.f.state.ID || a.state.Ticket != p.Ticket {
					t.Fatal(view, err)
				}
				result, err := a.resolve(p)
				if err != nil || result.Status.ID != "" || result.PastOutcome != "never-admitted" {
					t.Fatal(result, err)
				}
			case "empty-before-journal-late-original":
				result, err := a.resolve(p)
				if err != nil || !result.SafeForNewPreview {
					t.Fatal(result, err)
				}
				if _, err := a.c.RunLegacyAppMaintenance(ctx, appMutation(p)); err == nil {
					t.Fatal("late original admitted")
				}
				if a.effects("prepare")+a.effects("retire") != 0 {
					t.Fatal(a.f.events)
				}
				fresh := a.preview(t)
				if fresh.Ticket.Generation <= p.Ticket.Generation || fresh.Ticket.ID == p.Ticket.ID {
					t.Fatal(fresh)
				}
			case "running-lock-busy", "concurrent-confirms":
				entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
				var once sync.Once
				original := a.f.d.prepare
				a.f.d.prepare = func(s MaintenanceStatus) error { close(entered); <-release; return original(s) }
				joined := false
				t.Cleanup(func() {
					once.Do(func() { close(release) })
					if !joined {
						select {
						case <-done:
						case <-time.After(2 * time.Second):
							t.Error("owned controller not joined")
						}
					}
				})
				go func() { _, err := a.c.RunLegacyAppMaintenance(ctx, appMutation(p)); done <- err }()
				select {
				case <-entered:
				case <-time.After(2 * time.Second):
					t.Fatal("intent not entered")
				}
				if name == "running-lock-busy" {
					if _, err := a.resolve(p); err == nil {
						t.Fatal("busy lock reported safe")
					}
				} else {
					if _, err := a.c.RunLegacyAppMaintenance(ctx, appMutation(p)); err == nil {
						t.Fatal("duplicate concurrent original accepted")
					}
				}
				once.Do(func() { close(release) })
				select {
				case err := <-done:
					joined = true
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("owned controller not terminal")
				}
				if a.effects("retire") != 1 {
					t.Fatal(a.f.events)
				}
			case "completed-reply-lost":
				if _, err := a.c.RunLegacyAppMaintenance(ctx, appMutation(p)); err != nil {
					t.Fatal(err)
				}
				result, err := a.resolve(p)
				if err != nil || result.Disposition != "matching-terminal" || result.Status.ID != p.Ticket.Transaction {
					t.Fatal(result, err)
				}
				if _, err := a.c.RunLegacyAppMaintenance(ctx, appMutation(p)); err != nil {
					t.Fatal(err)
				}
				if a.effects("retire") != 1 {
					t.Fatal("replayed completed operation", a.f.events)
				}
			case "prepare-before-journal":
				a.state.State = "running"
				a.prepared = MaintenanceStatus{Schema: 1, ID: p.Ticket.Transaction, Revision: 1, Plan: p.Plan, Phase: "prepared", OldOutcomes: "unknown_not_certified_drained"}
				result, err := a.resolve(p)
				if err != nil || result.Disposition != "fenced-interrupted-needs-recovery" || result.SafeForNewPreview || result.PastOutcome == "never-admitted" || result.Status.ID != p.Ticket.Transaction {
					t.Fatal(result, err)
				}
				request := LegacyAppRequest{Profile: RootRelayMaintenanceProfile, EngineSHA256: strings.Repeat("a", 64), Operation: "prepare-operation", TargetOperation: "abort", Transaction: p.Ticket.Transaction, PlanSHA256: p.Plan.SHA256}
				prepared, err := a.c.RunLegacyAppMaintenance(ctx, request)
				if err != nil {
					t.Fatal(err)
				}
				q := prepared.(MaintenanceAppPreview)
				request.Operation = "abort"
				request.TargetOperation = ""
				request.TicketJSON = q.TicketJSON
				if _, err := a.c.RunLegacyAppMaintenance(ctx, request); err != nil {
					t.Fatal(err)
				}
				if a.f.state.Phase != "aborted" || a.effects("retire") != 0 {
					t.Fatal(a.f.state, a.f.events)
				}
			case "fence-reply-lost":
				if _, err := a.resolve(p); err != nil {
					t.Fatal(err)
				}
				high := a.state.HighWater
				result, err := a.resolve(p)
				if err != nil || !result.SafeForNewPreview || a.state.HighWater != high {
					t.Fatal(result, err)
				}
			case "fence-write-or-readback-fails":
				for _, fault := range []string{"fence", "after-fence", "read", "sync"} {
					b := newAppAdmissionFixture(t)
					q := b.preview(t)
					b.fault = fault
					result, err := b.resolve(q)
					if err == nil || result.SafeForNewPreview {
						t.Fatal(fault, result, err)
					}
					b.fault = ""
					result, err = b.resolve(q)
					if err != nil || !result.SafeForNewPreview {
						t.Fatal(fault, result, err)
					}
					if _, err := b.c.RunLegacyAppMaintenance(ctx, appMutation(q)); err == nil {
						t.Fatal("failed/lost fence allowed late original")
					}
				}
			case "delayed-preview-superseded":
				_ = a.preview(t)
				if _, err := a.c.RunLegacyAppMaintenance(ctx, appMutation(p)); err == nil || a.effects("prepare") != 0 {
					t.Fatal("stale preview revived")
				}
			case "changed-binding":
				for _, field := range []string{"plan", "verb", "transaction", "restore", "alias"} {
					request := appMutation(p)
					ticket := p.Ticket
					switch field {
					case "plan":
						ticket.Plan.LegacyPayload += "-other"
						ticket.Plan.SHA256 = maintenanceDigest(ticket.Plan)
					case "verb":
						ticket.Operation = "recover"
					case "transaction":
						ticket.Transaction = strings.Repeat("f", 32)
					case "restore":
						ticket.RestoreSHA256 = strings.Repeat("e", 64)
					}
					raw, _ := json.Marshal(ticket)
					request.TicketJSON = string(raw)
					if field == "alias" {
						request.TicketJSON = strings.Replace(request.TicketJSON, `"generation":`, `"Generation":`, 1)
					}
					if _, err := a.c.RunLegacyAppMaintenance(ctx, request); err == nil {
						t.Fatal("changed ticket admitted", field)
					}
				}
				if a.effects("prepare") != 0 {
					t.Fatal(a.f.events)
				}
			case "agent-instance-changed":
				a.f.plan.Original.PID++
				a.f.plan.SHA256 = maintenanceDigest(a.f.plan)
				if _, err := a.c.RunLegacyAppMaintenance(ctx, appMutation(p)); err == nil || a.effects("prepare") != 0 {
					t.Fatal("changed instance admitted")
				}
			case "cancel-after-running-intent":
				request, cancel := context.WithCancel(ctx)
				defer cancel()
				a.afterRunning = cancel
				if _, err := a.c.RunLegacyAppMaintenance(request, appMutation(p)); !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				if a.effects("prepare") != 0 {
					t.Fatal("canceled intent prepared")
				}
				result, err := a.resolve(p)
				if err != nil || !result.SafeForNewPreview || result.PastOutcome == "never-admitted" {
					t.Fatal(result, err)
				}
			case "old-detail-evicted":
				if _, err := a.resolve(p); err != nil {
					t.Fatal(err)
				}
				_ = a.preview(t)
				result, err := a.resolve(p)
				if err != nil || result.Disposition != "future-fenced-past-detail-not-retained" || result.PastOutcome != "unknown" {
					t.Fatal(result, err)
				}
				if _, err := a.c.RunLegacyAppMaintenance(ctx, appMutation(p)); err == nil {
					t.Fatal("old generation revived")
				}
			}
		})
	}
}
