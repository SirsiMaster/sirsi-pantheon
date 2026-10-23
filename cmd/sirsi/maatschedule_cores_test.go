package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestReserveCLIPropagatesBoundedCoresAndPreservesLegacyOmission(t *testing.T) {
	oldLedger, oldJournal := maatLedger, newMaatDecisionJournal
	oldHolder, oldWork, oldRegime, oldStart, oldEnd := resHolder, resWork, resRegime, resStart, resEstEnd
	oldCores, oldQueue, oldJSON, oldNonce := resCores, resQueue, maatJSON, maatOperationNonce
	t.Cleanup(func() {
		maatLedger, newMaatDecisionJournal = oldLedger, oldJournal
		resHolder, resWork, resRegime, resStart, resEstEnd = oldHolder, oldWork, oldRegime, oldStart, oldEnd
		resCores, resQueue, maatJSON, maatOperationNonce = oldCores, oldQueue, oldJSON, oldNonce
	})
	for _, cores := range []int{0, 1, 2, -1, 65} {
		store, err := routerstore.OpenPath(filepath.Join(t.TempDir(), "fixture.db"))
		if err != nil {
			t.Fatal(err)
		}
		ledger := schedule.NewLedger(store)
		maatLedger = func() (*schedule.Ledger, error) { return ledger, nil }
		newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return &capturedDecisionJournal{}, nil }
		resHolder, resWork, resRegime = "cpu-fixture", "finite-cpu", "build"
		resStart, resEstEnd = time.Now().UTC().Format(time.RFC3339), ""
		resCores, resQueue, maatJSON, maatOperationNonce = cores, false, true, ""
		err = maatReserveCmd.RunE(maatReserveCmd, []string{"m5"})
		rows, readErr := ledger.Status("m5")
		if readErr != nil {
			t.Fatal(readErr)
		}
		if cores < 0 || cores > 64 {
			if err == nil || len(rows) != 0 {
				t.Fatalf("invalid CPU request wrote state: cores=%d err=%v rows=%+v", cores, err, rows)
			}
		} else if err != nil || len(rows) != 1 || rows[0].Cores != cores {
			t.Fatalf("CLI lost explicit/legacy cores=%d: err=%v rows=%+v", cores, err, rows)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
