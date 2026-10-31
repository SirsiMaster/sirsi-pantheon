//go:build darwin && cgo

package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/servicedrain"
	"golang.org/x/sys/unix"
)

func maintenancePhysicalFixture(t *testing.T) (string, *os.Root, MaintenancePlan) {
	t.Helper()
	path, f := maintenanceOwnedDirectory(t)
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		t.Fatal(err)
	}
	p := fixtureMaintenancePlan()
	p.RootDevice = uint64(st.Dev)
	p.RootInode = st.Ino
	p.SHA256 = maintenanceDigest(p)
	return path, root, p
}

func TestMaintenancePreparationCrashIsVisibleToRealStartupReader(t *testing.T) {
	for _, window := range []string{"private_stage_before_commit", "before_private_journal", "after_private_prepared", "during_marker_staging"} {
		t.Run(window, func(t *testing.T) {
			path, root, p := maintenancePhysicalFixture(t)
			s := MaintenanceStatus{Schema: 1, ID: strings.Repeat("1", 32), Plan: p, OldOutcomes: "unknown_not_certified_drained"}
			var hook func() error
			if window == "private_stage_before_commit" {
				hook = func() error { return errors.New("controller died before body publication") }
			}
			err := maintenancePrepareAt(root, s, os.Getuid(), os.Getgid(), hook)
			if window == "private_stage_before_commit" {
				if err == nil {
					t.Fatal("private staging interruption accepted")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if window == "during_marker_staging" {
				b, err := jsonMaintenance(maintenanceMarkerValue(s))
				if err != nil {
					t.Fatal(err)
				}
				err = maintenancePublishIntentAt(root, servicedrain.MaintenanceMarkerName, s.ID, b, os.Getuid(), os.Getgid(), func() error { return errors.New("marker stage interrupted") })
				if err == nil {
					t.Fatal("marker staging interruption accepted")
				}
			}
			if window == "after_private_prepared" {
				parent, _ := maintenanceOwnedDirectory(t)
				j := maintenanceJournal{parent, uint32(os.Getuid())}
				prepared, err := maintenanceReadPreparationAt(root, uint32(os.Getuid()), uint32(os.Getgid()))
				if err != nil {
					t.Fatal(err)
				}
				if err := j.persist(0, prepared); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := os.Stat(filepath.Join(path, servicedrain.MaintenanceMarkerName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("fixture accidentally supplied canonical marker", err)
			}
			intent, err := routerstore.OpenMaintenanceStartupIntent(root, uint32(os.Getuid()), uint32(os.Getgid()))
			if window == "private_stage_before_commit" {
				if intent != nil || err != nil {
					t.Fatal("uncommitted private stage became startup authority", err)
				}
				entries, readErr := os.ReadDir(path)
				if readErr != nil || len(entries) != 0 {
					t.Fatal("private stage leaked into queue namespace", entries, readErr)
				}
				return
			}
			if err != nil || intent == nil {
				t.Fatal("pending preparation was invisible to producer", err)
			}
			defer intent.Close()
			gate, err := servicedrain.NewStartupMaintenance(789, "reboot-fixture", intent.Marker, 0, func(servicedrain.MaintenanceReady) error { return intent.Verify(root) }, nil)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := gate.Wait(ctx); !errors.Is(err, context.Canceled) {
				t.Fatal("pending prepare released without activation", err)
			}
		})
	}
}

func TestMaintenanceMarkerCleanupCrashReconstructsHeldGeneration(t *testing.T) {
	for _, failure := range []string{"completed_append", "prepare_cleanup"} {
		t.Run(failure, func(t *testing.T) {
			path, root, p := maintenancePhysicalFixture(t)
			journalPath, _ := maintenanceOwnedDirectory(t)
			j := maintenanceJournal{journalPath, uint32(os.Getuid())}
			f, c := newMaintenanceFixture(t)
			f.plan = p
			f.d.load = j.load
			f.d.prepare = func(s MaintenanceStatus) error { return maintenancePrepareAt(root, s, os.Getuid(), os.Getgid(), nil) }
			failNow := true
			f.d.persist = func(v uint64, s MaintenanceStatus) error {
				if failure == "completed_append" && s.Phase == "completed" && failNow {
					return errors.New("injected completion append failure")
				}
				return j.persist(v, s)
			}
			f.d.pending = func(s MaintenanceStatus) (bool, error) {
				prepared, err := maintenanceReadPreparationAt(root, uint32(os.Getuid()), uint32(os.Getgid()))
				if errors.Is(err, os.ErrNotExist) {
					return false, nil
				}
				return prepared.ID == s.ID && prepared.Plan == s.Plan, err
			}
			f.d.clearPreparation = func(s MaintenanceStatus) error {
				if failure == "prepare_cleanup" && failNow {
					return errors.New("injected cleanup failure")
				}
				return maintenanceClearPreparationAt(root, s, uint32(os.Getuid()), uint32(os.Getgid()))
			}
			f.d.exclude = func(ctx context.Context, s MaintenanceStatus, op func() error) error {
				if s.Phase != "activated" && s.Phase != "completed" {
					b, err := jsonMaintenance(maintenanceMarkerValue(s))
					if err != nil {
						return err
					}
					if err := os.WriteFile(filepath.Join(path, servicedrain.MaintenanceMarkerName), b, 0600); err != nil {
						return err
					}
					if err := os.Chmod(filepath.Join(path, servicedrain.MaintenanceMarkerName), 0640); err != nil {
						return err
					}
				}
				return routerstore.WithSpoolQuiescence(ctx, path, op)
			}
			var gate *servicedrain.StartupMaintenance
			pid := 456
			f.d.ready = func(ctx context.Context, s MaintenanceStatus) (maintenanceReady, error) {
				if gate == nil {
					intent, err := routerstore.OpenMaintenanceStartupIntent(root, uint32(os.Getuid()), uint32(os.Getgid()))
					if err != nil || intent == nil {
						return maintenanceReady{}, errors.Join(errors.New("startup intent absent"), err)
					}
					defer intent.Close()
					gate, err = servicedrain.NewStartupMaintenance(pid, "fresh-generation", intent.Marker, 3, func(servicedrain.MaintenanceReady) error { return nil }, nil)
					if err != nil {
						return maintenanceReady{}, err
					}
				}
				instance := p.Original
				instance.PID = pid
				instance.Started = "fresh-generation"
				return maintenanceReady{instance, gate.Status().Generation, gate.Status().State == "maintenance_ready_not_drained"}, nil
			}
			f.d.activate = func(ctx context.Context, s MaintenanceStatus, r maintenanceReady) error {
				_, err := gate.Activate(gate.Status())
				return err
			}
			f.d.finish = func(ctx context.Context, s MaintenanceStatus) error {
				err := os.Remove(filepath.Join(path, servicedrain.MaintenanceMarkerName))
				if errors.Is(err, os.ErrNotExist) {
					return nil
				}
				return err
			}
			state, err := c.ConfirmLegacyMaintenance(context.Background(), p)
			if err == nil {
				t.Fatal("crash boundary not reached")
			}
			if _, err := os.Stat(filepath.Join(path, servicedrain.MaintenanceMarkerName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("marker was not actually removed", err)
			}
			if _, err := os.Stat(filepath.Join(path, servicedrain.MaintenancePrepareName)); err != nil {
				t.Fatal("pending durable preparation was lost", err)
			}
			// Reconstruct the producer from real remaining files. The old readiness
			// object is discarded; no synthetic old endpoint survives this boundary.
			gate = nil
			pid = 789
			failNow = false
			result, err := c.RecoverLegacyMaintenance(context.Background(), state.ID, state.Plan.SHA256)
			if err != nil {
				t.Fatal(err)
			}
			if result.Phase != "completed" || result.Successor.PID != 789 || gate.Status().State != "active_with_old_outcomes_unknown" {
				t.Fatalf("generation reconciliation failed: %+v", result)
			}
			if countMaintenanceEvent(f, "retire") != 1 {
				t.Fatal("recovery retired another process", f.events)
			}
			if intent, err := routerstore.OpenMaintenanceStartupIntent(root, uint32(os.Getuid()), uint32(os.Getgid())); err != nil || intent != nil {
				if intent != nil {
					_ = intent.Close()
				}
				t.Fatal("completed cleanup did not restore ordinary admission", err)
			}
			persisted, err := j.load()
			if err != nil || persisted != result {
				t.Fatal("final completion not durable", err)
			}
		})
	}
}
