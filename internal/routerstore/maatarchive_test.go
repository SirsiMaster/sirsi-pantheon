package routerstore

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMaatHistoryArchivesFinishedRowsPastRetention(t *testing.T) {
	s := openBackendStore(t, filepath.Join(t.TempDir(), "test.db"))
	defer s.Close()
	now := s.clock().UTC()
	old := now.Add(-72 * time.Hour).Format(time.RFC3339)
	recent := now.Add(-time.Hour).Format(time.RFC3339)
	seed := `{"reservations":[` +
		`{"id":"old-done","resource":"m1","holder":"ra","status":"released","start":"` + old + `","future_field":"kept"},` +
		`{"id":"old-active","resource":"m1","holder":"ra","status":"active","start":"` + old + `"},` +
		`{"id":"new-done","resource":"m1","holder":"ra","status":"released","start":"` + recent + `"}],"other":1}`
	if err := s.withTx(func(tx *txHandle) error {
		_, err := tx.Exec(`INSERT INTO state(key,value) VALUES(?,?)`, maatReservationsKey, seed)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MaatRead("status", ""); err != nil {
		t.Fatal(err)
	}
	hot, _, _ := s.GetState(maatReservationsKey)
	if strings.Contains(hot, `"old-done"`) || !strings.Contains(hot, `"old-active"`) || !strings.Contains(hot, `"new-done"`) || !strings.Contains(hot, `"other"`) {
		t.Fatalf("hot row after archive: %s", hot)
	}
	arch, ok, _ := s.GetState(maatArchivePrefix + now.Add(-72*time.Hour).Format("2006-01-02"))
	var rows []map[string]any
	if !ok || json.Unmarshal([]byte(arch), &rows) != nil || len(rows) != 1 || rows[0]["id"] != "old-done" || rows[0]["future_field"] != "kept" {
		t.Fatalf("archive row: ok=%v %s", ok, arch)
	}
	// The Ma'at ledger still serves the kept rows.
	got, err := s.MaatRead("status", "")
	if err != nil || len(got) != 2 {
		t.Fatalf("status after archive: %v %+v", err, got)
	}
}
