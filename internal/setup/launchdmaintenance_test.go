package setup

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type maintenanceFixture struct {
	d          maintenanceOperations
	state      MaintenanceStatus
	plan       MaintenancePlan
	events     []string
	fail       string
	retirement string
	held       bool
	retireErr  error
}

func (f *maintenanceFixture) Inspect(context.Context, string) (ServiceInstance, error) {
	panic("ordinary refresh must not run")
}
func (f *maintenanceFixture) Refresh(context.Context, ServiceInstance) error {
	panic("ordinary refresh must not run")
}
func (f *maintenanceFixture) maintenanceOperations() maintenanceOperations { return f.d }

func fixtureMaintenancePlan() MaintenancePlan {
	p := MaintenancePlan{Profile: RootRelayMaintenanceProfile,
		Original:      ServiceInstance{Label: "ai.sirsi.router.relay", Target: "system/ai.sirsi.router.relay", Plist: "/Library/LaunchDaemons/ai.sirsi.router.relay.plist", PlistSHA256: strings.Repeat("1", 64), Program: "/usr/local/libexec/sirsi-pantheon/sirsi", ProgramSHA256: strings.Repeat("2", 64), PID: 123, Started: "fixture-old"},
		LegacyPayload: "/owned/retained/sirsi", LegacySHA256: strings.Repeat("3", 64), LoadedCDHash: strings.Repeat("4", 40), ReplacementCDHash: strings.Repeat("5", 40), KernelStart: "123.000001", LegacySource: "recognized-fixture-source", RootDevice: 7, RootInode: 8,
		WriterScope: "cooperative-publication-only; uncovered-writers-and-old-remote-outcomes-unknown"}
	p.SHA256 = maintenanceDigest(p)
	return p
}

func newMaintenanceFixture(t *testing.T) (*maintenanceFixture, ServiceController) {
	t.Helper()
	f := &maintenanceFixture{plan: fixtureMaintenancePlan(), retirement: "retired", held: true}
	effect := func(name string) error {
		f.events = append(f.events, name)
		if f.fail == name {
			return errors.New("injected " + name)
		}
		return nil
	}
	f.d = maintenanceOperations{
		preview: func(ctx context.Context, path string) (MaintenancePlan, error) {
			if path != f.plan.LegacyPayload {
				return MaintenancePlan{}, errors.New("wrong payload")
			}
			return f.plan, effect("preview")
		},
		lock: func(op func() error) error {
			if err := effect("lock"); err != nil {
				return err
			}
			return op()
		},
		load: func() (MaintenanceStatus, error) { return f.state, effect("load") },
		persist: func(version uint64, s MaintenanceStatus) error {
			if version != f.state.Revision || s.Revision != version+1 || !maintenanceTransition(f.state, s) {
				return errors.New("journal CAS/transition")
			}
			if err := effect("persist:" + s.Phase); err != nil {
				return err
			}
			f.state = s
			return nil
		},
		exclude: func(ctx context.Context, s MaintenanceStatus, op func() error) error {
			if err := effect("exclude"); err != nil {
				return err
			}
			defer func() { f.events = append(f.events, "unlock") }()
			return op()
		},
		revalidate: func(ctx context.Context, p MaintenancePlan, old bool) error {
			if p != f.plan {
				return errors.New("changed plan")
			}
			return effect("revalidate")
		},
		retire: func(ctx context.Context, p MaintenancePlan) error {
			if err := effect("retire"); err != nil {
				return err
			}
			return f.retireErr
		},
		retirement: func(ctx context.Context, p MaintenancePlan) (string, error) {
			if ctx.Err() != nil {
				return "unknown", ctx.Err()
			}
			return f.retirement, effect("retirement")
		},
		preserve:  func(ctx context.Context, s MaintenanceStatus) (int, error) { return 3, effect("preserve") },
		bootstrap: func(ctx context.Context, s MaintenanceStatus) error { return effect("bootstrap") },
		resume:    func(ctx context.Context, s MaintenanceStatus) error { return effect("resume") },
		ready: func(ctx context.Context, s MaintenanceStatus) (maintenanceReady, error) {
			p := f.plan.Original
			p.PID = 456
			p.Started = "fixture-new"
			return maintenanceReady{p, "generation-1", f.held}, effect("ready")
		},
		activate: func(ctx context.Context, s MaintenanceStatus, r maintenanceReady) error {
			if s.Generation != r.Generation || s.Successor != r.Instance {
				return errors.New("mixed generation")
			}
			if err := effect("activate"); err != nil {
				return err
			}
			f.held = false
			return nil
		},
		finish: func(ctx context.Context, s MaintenanceStatus) error {
			if f.held || s.Phase != "activated" {
				return errors.New("premature finish")
			}
			return effect("finish")
		},
	}
	c := ServiceController{Backend: f, Authorize: func(ctx context.Context, label string) error {
		if label != "ai.sirsi.router.relay" {
			return errors.New("wrong profile")
		}
		return effect("authorize")
	}}
	return f, c
}

func countMaintenanceEvent(f *maintenanceFixture, event string) int {
	n := 0
	for _, e := range f.events {
		if e == event {
			n++
		}
	}
	return n
}

func TestLegacyMaintenanceControllerTransaction(t *testing.T) {
	f, c := newMaintenanceFixture(t)
	p, err := c.PreviewLegacyMaintenance(context.Background(), f.plan.LegacyPayload)
	if err != nil {
		t.Fatal(err)
	}
	s, err := c.ConfirmLegacyMaintenance(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Phase != "completed" || s.Preserved != 3 || s.OldOutcomes != "unknown_not_certified_drained" || s != f.state {
		t.Fatalf("incorrect durable result: %+v", s)
	}
	want := []string{"persist:prepared", "exclude", "revalidate", "persist:retirement_requested", "retire", "retirement", "persist:retired", "revalidate", "preserve", "persist:custody_preserved", "persist:bootstrap_requested", "bootstrap", "ready", "persist:activation_requested", "activate", "persist:activated", "finish", "persist:completed", "unlock"}
	var effects []string
	for _, e := range f.events {
		if e != "preview" && e != "load" && e != "authorize" && e != "lock" {
			effects = append(effects, e)
		}
	}
	if !reflect.DeepEqual(effects, want) {
		t.Fatalf("effect order %v", effects)
	}
	before := len(f.events)
	if _, err := c.RecoverLegacyMaintenance(context.Background(), s.ID, p.SHA256); err != nil {
		t.Fatal(err)
	}
	for _, e := range f.events[before:] {
		if e == "retire" || e == "bootstrap" || e == "activate" {
			t.Fatalf("completed history repeated %s", e)
		}
	}
}

func TestLegacyMaintenanceRetirementAmbiguity(t *testing.T) {
	for _, state := range []string{"original_alive", "pid_reused_or_changed", "outcome_unknown", "selected_job_still_loaded_or_changed"} {
		t.Run(state, func(t *testing.T) {
			f, c := newMaintenanceFixture(t)
			f.retirement = state
			f.retireErr = context.DeadlineExceeded
			s, err := c.ConfirmLegacyMaintenance(context.Background(), f.plan)
			if err == nil || !errors.Is(err, context.DeadlineExceeded) || s.Phase != "retirement_requested" {
				t.Fatalf("lost ambiguity: %+v %v", s, err)
			}
			if _, err = c.RecoverLegacyMaintenance(context.Background(), s.ID, s.Plan.SHA256); err == nil {
				t.Fatal("ambiguous recovery accepted")
			}
			if countMaintenanceEvent(f, "retire") != 1 || countMaintenanceEvent(f, "bootstrap") != 0 || countMaintenanceEvent(f, "activate") != 0 {
				t.Fatal(f.events)
			}
		})
	}
}

func TestLegacyMaintenanceCrashRecovery(t *testing.T) {
	for _, stop := range []string{"persist:retirement_requested", "persist:retired", "preserve", "persist:custody_preserved", "bootstrap", "ready", "activate", "persist:activated", "finish", "persist:completed"} {
		t.Run(stop, func(t *testing.T) {
			f, c := newMaintenanceFixture(t)
			f.fail = stop
			s, err := c.ConfirmLegacyMaintenance(context.Background(), f.plan)
			if err == nil {
				t.Fatal("fault accepted")
			}
			if s.ID == "" || s != f.state {
				t.Fatalf("returned nondurable phase: %+v durable %+v", s, f.state)
			}
			f.fail = ""
			recovered, err := c.RecoverLegacyMaintenance(context.Background(), s.ID, s.Plan.SHA256)
			if err != nil {
				t.Fatal(err)
			}
			if recovered.Phase != "completed" || countMaintenanceEvent(f, "retire") > 1 || countMaintenanceEvent(f, "activate") > 2 {
				t.Fatalf("unsafe recovery: %+v %v", recovered, f.events)
			}
		})
	}
}

func TestLegacyMaintenanceRefusalsBeforeMutation(t *testing.T) {
	for _, kind := range []string{"authorization", "digest", "profile", "preview", "exclude", "revalidate", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			f, c := newMaintenanceFixture(t)
			p := f.plan
			ctx := context.Background()
			switch kind {
			case "authorization":
				f.fail = "authorize"
			case "digest":
				p.SHA256 = strings.Repeat("0", 64)
			case "profile":
				p.Original.Label = "other"
				p.SHA256 = maintenanceDigest(p)
			case "preview":
				f.fail = "preview"
			case "exclude":
				f.fail = "exclude"
			case "revalidate":
				f.fail = "revalidate"
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if _, err := c.ConfirmLegacyMaintenance(ctx, p); err == nil {
				t.Fatal("invalid confirmation accepted")
			}
			if countMaintenanceEvent(f, "retire") != 0 || countMaintenanceEvent(f, "bootstrap") != 0 {
				t.Fatal(f.events)
			}
		})
	}
}

func TestLegacyMaintenanceRecoveryIdentityAndContext(t *testing.T) {
	f, c := newMaintenanceFixture(t)
	f.fail = "bootstrap"
	s, err := c.ConfirmLegacyMaintenance(context.Background(), f.plan)
	if err == nil {
		t.Fatal("expected bootstrap error")
	}
	for _, pair := range [][2]string{{"other", s.Plan.SHA256}, {s.ID, "other"}} {
		if _, err := c.RecoverLegacyMaintenance(context.Background(), pair[0], pair[1]); err == nil {
			t.Fatal("mixed recovery accepted")
		}
	}
	f.fail = ""
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Cancellation after acquiring the physical barrier cannot cancel the
	// independent recovery of an already-retired selected job.
	previousExclude := f.d.exclude
	f.d.exclude = func(ctx context.Context, s MaintenanceStatus, op func() error) error {
		return previousExclude(ctx, s, func() error { cancel(); return op() })
	}
	f.d.ready = func(recovery context.Context, s MaintenanceStatus) (maintenanceReady, error) {
		if recovery.Err() != nil {
			t.Fatal("retired recovery inherited cancellation")
		}
		deadline, ok := recovery.Deadline()
		if !ok || time.Until(deadline) > 15*time.Second {
			t.Fatal("recovery not bounded")
		}
		p := s.Plan.Original
		p.PID = 456
		p.Started = "fixture-new"
		return maintenanceReady{p, "generation-1", true}, nil
	}
	if _, err := c.RecoverLegacyMaintenance(ctx, s.ID, s.Plan.SHA256); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyMaintenanceConfirmationPinsEveryIdentity(t *testing.T) {
	for _, field := range []string{"pid", "start", "kernel_start", "loaded_cdhash", "replacement_cdhash", "old_payload", "plist", "program", "root_device", "root_inode"} {
		t.Run(field, func(t *testing.T) {
			f, c := newMaintenanceFixture(t)
			approved := f.plan
			switch field {
			case "pid":
				f.plan.Original.PID++
			case "start":
				f.plan.Original.Started = "changed"
			case "kernel_start":
				f.plan.KernelStart = "changed"
			case "loaded_cdhash":
				f.plan.LoadedCDHash = strings.Repeat("6", 40)
			case "replacement_cdhash":
				f.plan.ReplacementCDHash = strings.Repeat("6", 40)
			case "old_payload":
				f.plan.LegacySHA256 = strings.Repeat("6", 64)
			case "plist":
				f.plan.Original.PlistSHA256 = strings.Repeat("6", 64)
			case "program":
				f.plan.Original.ProgramSHA256 = strings.Repeat("6", 64)
			case "root_device":
				f.plan.RootDevice++
			case "root_inode":
				f.plan.RootInode++
			}
			f.plan.SHA256 = maintenanceDigest(f.plan)
			if _, err := c.ConfirmLegacyMaintenance(context.Background(), approved); err == nil {
				t.Fatal("changed identity accepted")
			}
			if countMaintenanceEvent(f, "retire") != 0 || f.state.ID != "" {
				t.Fatalf("mutated before matching immutable preview: %v", f.events)
			}
		})
	}
}

func TestLegacyMaintenancePreparationFailureIsNotDurableSuccess(t *testing.T) {
	f, c := newMaintenanceFixture(t)
	f.fail = "persist:prepared"
	s, err := c.ConfirmLegacyMaintenance(context.Background(), f.plan)
	if err == nil || s.Phase != "preparation_outcome_unverified" || f.state.ID != "" || countMaintenanceEvent(f, "retire") != 0 {
		t.Fatalf("incorrect preparation outcome %+v %v", s, err)
	}
}
