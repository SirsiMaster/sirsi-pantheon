package setup

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type maintenancePublicationFixture struct {
	record MaintenanceRestoration
	events *[]string
	fail   bool
}

func (p maintenancePublicationFixture) Prepared() MaintenanceRestoration { return p.record }
func (p maintenancePublicationFixture) Publish(ctx context.Context) error {
	*p.events = append(*p.events, "publish")
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if p.fail {
		return errors.New("publication outcome unknown")
	}
	return nil
}
func (p maintenancePublicationFixture) Close() error {
	*p.events = append(*p.events, "close")
	return nil
}

func TestLegacyMaintenanceAbortPreservesOriginalAndRefusesRetired(t *testing.T) {
	for _, phase := range []string{"prepared", "abort_requested", "retirement_requested", "retired", "bootstrap_requested"} {
		t.Run(phase, func(t *testing.T) {
			f, c := newMaintenanceFixture(t)
			f.state = MaintenanceStatus{Schema: 1, ID: strings.Repeat("a", 32), Revision: 1, Plan: f.plan, Phase: phase, OldOutcomes: "unknown_not_certified_drained"}
			f.d.abortVerify = func(context.Context, MaintenanceStatus) error {
				f.events = append(f.events, "verify-original")
				return nil
			}
			f.d.abortFinish = func(context.Context, MaintenanceStatus) error {
				f.events = append(f.events, "release-preparation")
				return nil
			}
			s, err := c.AbortLegacyMaintenance(context.Background(), f.state.ID, f.plan.SHA256)
			allowed := phase == "prepared" || phase == "abort_requested"
			if (err == nil) != allowed {
				t.Fatalf("%s: %v", phase, err)
			}
			if allowed && s.Phase != "aborted" {
				t.Fatal(s)
			}
			for _, event := range f.events {
				if event == "retire" || event == "bootstrap" || event == "activate" || event == "preserve" {
					t.Fatal("abort performed lifecycle action", f.events)
				}
			}
			if !allowed && strings.Contains(strings.Join(f.events, ","), "release-preparation") {
				t.Fatal(f.events)
			}
		})
	}
}

func TestLegacyMaintenanceRestorePersistsIntentAndRetainsFailure(t *testing.T) {
	for _, failure := range []string{"", "selection-drift", "publish"} {
		t.Run(failure, func(t *testing.T) {
			f, c := newMaintenanceFixture(t)
			f.state = MaintenanceStatus{Schema: 1, ID: strings.Repeat("a", 32), Revision: 1, Plan: f.plan, Phase: "prepared", OldOutcomes: "unknown_not_certified_drained"}
			f.d.pending = func(MaintenanceStatus) (bool, error) { return true, nil }
			f.d.restorePreview = func(_ context.Context, s MaintenanceStatus, artifact, nonce string) (MaintenanceRestorePlan, error) {
				p := MaintenanceRestorePlan{Transaction: s.ID, MaintenanceSHA256: s.Plan.SHA256, Revision: s.Revision, Artifact: artifact, ArtifactSHA256: s.Plan.Original.ProgramSHA256, ArtifactCDHash: s.Plan.ReplacementCDHash, Nonce: nonce, ParentDevice: 1, ParentInode: 2}
				p.SHA256 = restoreDigest(p)
				return p, nil
			}
			f.d.restorePrepare = func(_ context.Context, _ MaintenanceStatus, p MaintenanceRestorePlan) (maintenanceArtifactPublication, error) {
				f.events = append(f.events, "stage")
				a, b := maintenanceRestoreNames(p)
				return maintenancePublicationFixture{MaintenanceRestoration{Plan: p, Phase: "prepared", StageDevice: 1, StageInode: 3, StagePath: a, BackupPath: b}, &f.events, failure == "publish"}, nil
			}
			p, err := c.PreviewLegacyRestore(context.Background(), f.state.ID, f.plan.SHA256, "/owned/replacement")
			if err != nil {
				t.Fatal(err)
			}
			if failure == "selection-drift" {
				p.TargetInode = 8
				p.SHA256 = restoreDigest(p)
			}
			s, err := c.ConfirmLegacyRestore(context.Background(), p)
			if (err != nil) != (failure != "") {
				t.Fatal(err)
			}
			if failure == "selection-drift" {
				if s.Revision != 1 || strings.Contains(strings.Join(f.events, ","), "stage") {
					t.Fatal(s, f.events)
				}
				return
			}
			if s.Phase != "prepared" || s.ID != p.Transaction || s.OldOutcomes != "unknown_not_certified_drained" {
				t.Fatal(s)
			}
			want := "verified_current_replacement"
			if failure == "publish" {
				want = "publication_requested"
			}
			if s.Restoration.Phase != want || s.Restoration.StagePath == "" || s.Restoration.BackupPath == "" {
				t.Fatal(s)
			}
			if f.events[len(f.events)-1] != "close" {
				t.Fatal(f.events)
			}
			for _, event := range f.events {
				if event == "retire" || event == "bootstrap" || event == "activate" || event == "preserve" {
					t.Fatal("restoration changed runtime", f.events)
				}
			}
		})
	}
}
