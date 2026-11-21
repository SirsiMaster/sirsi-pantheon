package routerstore

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// hotRowRow builds one stored reservation row that started startAgo ago.
func hotRowRow(now time.Time, status string, startAgo, endedAgo time.Duration, extra map[string]any) map[string]any {
	start := now.Add(-startAgo)
	id := start.Format("20060102T150405Z") + "-m1-ra-" + status + "-" + strings.ReplaceAll(startAgo.String(), ".", "_")
	row := map[string]any{
		"id": id, "resource": "m1", "holder": "ra", "regime": "build", "status": status,
		"start": ts(start), "est_end": ts(start.Add(10 * time.Minute)), "created": ts(start),
		"heartbeat_at": ts(start), "lease_ttl_sec": 120,
	}
	if endedAgo >= 0 {
		row["ended_at"] = ts(now.Add(-endedAgo))
	}
	for k, v := range extra {
		row[k] = v
	}
	return row
}

func seedMaatRows(t *testing.T, s *SQLiteStore, rows ...map[string]any) {
	t.Helper()
	b, err := json.Marshal(map[string]any{"reservations": rows})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.withTx(func(tx *txHandle) error {
		_, err := tx.Exec(`INSERT INTO state(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, maatReservationsKey, string(b))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// Let the next call archive immediately.
	if err := s.withTx(func(tx *txHandle) error {
		_, err := tx.Exec(`DELETE FROM state WHERE key=?`, maatArchiveAtKey)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func hotIDs(t *testing.T, s *SQLiteStore) map[string]bool {
	t.Helper()
	raw, _, err := s.GetState(maatReservationsKey)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct{ Reservations []struct{ ID string } }
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, r := range doc.Reservations {
		out[r.ID] = true
	}
	return out
}

func archivedIDs(t *testing.T, s *SQLiteStore) map[string]int {
	t.Helper()
	out := map[string]int{}
	err := s.withTx(func(tx *txHandle) error {
		rows, err := tx.Query(`SELECT value FROM state WHERE key LIKE ?`, maatArchivePrefix+"%")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				return err
			}
			var day []struct{ ID string }
			if err := json.Unmarshal([]byte(v), &day); err != nil {
				return err
			}
			for _, r := range day {
				out[r.ID]++
			}
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMaatHotDocumentKeepsLiveAndRecentRows(t *testing.T) {
	s := openBackendStore(t, filepath.Join(t.TempDir(), "test.db"))
	defer s.Close()
	now := s.clock().UTC()
	h := time.Hour
	keepIDs := []map[string]any{
		hotRowRow(now, "active", 72*h, -1, nil),                                           // live, however old
		hotRowRow(now, "queued", 72*h, -1, nil),                                           // waiting, however old
		hotRowRow(now, "released", 5*h, 30*time.Minute, nil),                              // ended 30 min ago, started long ago
		hotRowRow(now, "released", h, -1, nil),                                            // legacy (no ended_at), recent
		hotRowRow(now, "invalidated", 6*h, 5*h, map[string]any{"invalidated_by": "intr"}), // coverage counts 24h
	}
	moveIDs := []map[string]any{
		hotRowRow(now, "released", 4*h, 3*h, nil),                                                // ended 3h ago
		hotRowRow(now, "expired", 5*h, 4*h, nil),                                                 // ended 4h ago
		hotRowRow(now, "released", 3*h, -1, map[string]any{"heartbeat_at": ts(now.Add(-3 * h))}), // legacy, last seen 3h ago
		hotRowRow(now, "invalidated", 31*h, 30*h, map[string]any{"invalidated_by": "intr"}),
		hotRowRow(now, "released", 72*h, -1, nil), // legacy past 48h
	}
	all := append(append([]map[string]any{}, keepIDs...), moveIDs...)
	seedMaatRows(t, s, all...)
	if _, err := s.MaatRead("status", ""); err != nil {
		t.Fatal(err)
	}
	hot, arch := hotIDs(t, s), archivedIDs(t, s)
	for _, r := range keepIDs {
		if !hot[r["id"].(string)] || arch[r["id"].(string)] != 0 {
			t.Fatalf("%v must stay hot (hot=%v arch=%d)", r["id"], hot[r["id"].(string)], arch[r["id"].(string)])
		}
	}
	for _, r := range moveIDs {
		if hot[r["id"].(string)] || arch[r["id"].(string)] != 1 {
			t.Fatalf("%v must be archived exactly once (hot=%v arch=%d)", r["id"], hot[r["id"].(string)], arch[r["id"].(string)])
		}
	}
	// Nothing is deleted: every row is in exactly one place.
	if len(hot)+len(arch) != len(all) {
		t.Fatalf("rows lost: hot %d + archived %d != %d", len(hot), len(arch), len(all))
	}
}

func TestMaatArchivableRule(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) string { return ts(now.Add(-d)) }
	cases := []struct {
		name string
		row  maatRow
		want bool
	}{
		{"active never", maatRow{Status: "active", Start: ago(100 * time.Hour)}, false},
		{"queued never", maatRow{Status: "queued", Start: ago(100 * time.Hour)}, false},
		{"unreadable start stays", maatRow{Status: "released", Start: "x"}, false},
		{"ended 1h59m", maatRow{Status: "released", Start: ago(9 * time.Hour), EndedAt: ago(119 * time.Minute)}, false},
		{"ended 2h", maatRow{Status: "released", Start: ago(9 * time.Hour), EndedAt: ago(2 * time.Hour)}, true},
		{"ended recently, started 60h ago: end rules", maatRow{Status: "released", Start: ago(60 * time.Hour), EndedAt: ago(time.Minute)}, false},
		{"legacy fresh", maatRow{Status: "done", Start: ago(time.Hour)}, false},
		{"legacy heartbeat+ttl recent", maatRow{Status: "released", Start: ago(5 * time.Hour), HeartbeatAt: ago(2*time.Hour - time.Minute)}, false},
		{"legacy 48h", maatRow{Status: "released", Start: ago(49 * time.Hour)}, true},
		{"legacy stale 3h", maatRow{Status: "expired", Start: ago(4 * time.Hour), HeartbeatAt: ago(3 * time.Hour)}, true},
		{"invalidated 24h est holds", maatRow{Status: "invalidated", InvalidatedBy: "i", Start: ago(30 * time.Hour), EndedAt: ago(10 * time.Hour), EstEnd: ago(23 * time.Hour)}, false},
		{"invalidated est later than end holds longer", maatRow{Status: "invalidated", InvalidatedBy: "i", Start: ago(30 * time.Hour), EndedAt: ago(26 * time.Hour), EstEnd: ago(2 * time.Hour)}, false},
		{"invalidated past 25h", maatRow{Status: "invalidated", InvalidatedBy: "i", Start: ago(30 * time.Hour), EndedAt: ago(26 * time.Hour), EstEnd: ago(26 * time.Hour)}, true},
	}
	for _, c := range cases {
		if _, got := maatArchivable(c.row, now); got != c.want {
			t.Errorf("%s: archivable=%v want %v", c.name, got, c.want)
		}
	}
}

func archivedReceiptFixture(t *testing.T) (*SQLiteStore, string, string) {
	t.Helper()
	s := openBackendStore(t, filepath.Join(t.TempDir(), "test.db"))
	t.Cleanup(func() { _ = s.Close() })
	now := s.clock().UTC()
	h := time.Hour
	old := hotRowRow(now, "released", 4*h, 3*h, map[string]any{"exit_code": 0, "future_field": "kept"})
	fresh := hotRowRow(now, "released", 20*time.Minute, 10*time.Minute, map[string]any{"exit_code": 0})
	seedMaatRows(t, s, old, fresh)
	if _, err := s.MaatRead("status", ""); err != nil {
		t.Fatal(err)
	}
	return s, old["id"].(string), fresh["id"].(string)
}

func TestMaatReceiptVerifiesArchivedLikeHot(t *testing.T) {
	s, archived, hot := archivedReceiptFixture(t)
	if hotIDs(t, s)[archived] || !hotIDs(t, s)[hot] {
		t.Fatal("fixture: archived row must have left the hot document and the fresh one stay")
	}
	for _, id := range []string{hot, archived} {
		if err := s.verifyMaatReceipt("ra", id); err != nil {
			t.Fatalf("receipt maat:%s: %v", id, err)
		}
		if err := s.verifyMaatReceipt("someone-else", id); !errors.Is(err, ErrReceiptInvalid) {
			t.Fatalf("wrong holder must be invalid for %s: %v", id, err)
		}
	}
	if err := s.verifyMaatReceipt("ra", "20250101T000000Z-m1-ra-nope"); !errors.Is(err, ErrReceiptInvalid) {
		t.Fatalf("unknown id must be invalid: %v", err)
	}
}

func TestMaatStatusByIDFindsArchivedRow(t *testing.T) {
	s, archived, _ := archivedReceiptFixture(t)
	rows, err := s.MaatRead("reservation", archived)
	if err != nil || len(rows) != 1 || rows[0].ID != archived || rows[0].Status != schedule.StatusReleased || rows[0].ExitCode == nil {
		t.Fatalf("status --id on archived row: %v %+v", err, rows)
	}
	// Reads do not move the row back.
	if hotIDs(t, s)[archived] {
		t.Fatal("a read must not resurrect an archived row")
	}
}

func TestMaatMutateByIDOnArchivedRowBehavesAsOnHotRow(t *testing.T) {
	s, archived, _ := archivedReceiptFixture(t)
	if _, err := s.MaatMutate("heartbeat", archived, ""); err == nil || !strings.Contains(err.Error(), "is released, not active") {
		t.Fatalf("heartbeat on terminal row: %v", err)
	}
	if _, err := s.MaatMutate("release", archived, ""); err == nil || !strings.Contains(err.Error(), "already released") {
		t.Fatalf("release on terminal row: %v", err)
	}
	// Invalidate still records contamination on a finished run; it stays archived
	// and keeps the field this build does not know.
	got, err := s.MaatMutate("invalidate", archived, "intruder-x")
	if err != nil || got.Status != schedule.StatusInvalidated || got.InvalidatedBy != "intruder-x" {
		t.Fatalf("invalidate archived: %v %+v", err, got)
	}
	if hotIDs(t, s)[archived] || archivedIDs(t, s)[archived] != 1 {
		t.Fatal("mutated archived row must stay archived, once")
	}
	var raw string
	_ = s.withTx(func(tx *txHandle) error {
		return tx.QueryRow(`SELECT value FROM state WHERE key LIKE ? AND value LIKE ?`, maatArchivePrefix+"%", `%`+archived+`%`).Scan(&raw)
	})
	if !strings.Contains(raw, `"future_field":"kept"`) {
		t.Fatalf("unknown field lost on archive rewrite: %s", raw)
	}
	if _, err := s.MaatMutate("heartbeat", "no-such-id", ""); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unknown id: %v", err)
	}
}

func TestMaatReplayOfArchivedIDIsNotAReinsert(t *testing.T) {
	s := openBackendStore(t, filepath.Join(t.TempDir(), "test.db"))
	defer s.Close()
	now := s.clock().UTC()
	h := time.Hour
	row := hotRowRow(now, "released", 4*h, 3*h, map[string]any{"authority": "pod:p", "epoch": 1, "seq": 5, "share": "full"})
	id := row["id"].(string)
	seedMaatRows(t, s, row)
	if _, err := s.MaatRead("status", ""); err != nil {
		t.Fatal(err)
	}
	if hotIDs(t, s)[id] {
		t.Fatal("fixture: row must be archived")
	}
	replay := func(status schedule.Status, seq int64) error {
		return s.withMaatTransaction(func(_ *txHandle, l *schedule.Ledger) error {
			_, err := l.UpsertReservation(schedule.Reservation{ID: id, Resource: "m1", Holder: "ra", Regime: "build", Authority: "pod:p", Epoch: 1, Seq: seq, Status: status})
			return err
		})
	}
	if err := replay(schedule.StatusReleased, 5); err != nil {
		t.Fatalf("idempotent replay of archived terminal row: %v", err)
	}
	if hotIDs(t, s)[id] || archivedIDs(t, s)[id] != 1 {
		t.Fatalf("replay must not duplicate: hot=%v archived=%d", hotIDs(t, s)[id], archivedIDs(t, s)[id])
	}
	if err := replay(schedule.StatusActive, 6); err == nil {
		t.Fatal("an active replay must not resurrect an archived released row")
	}
	if err := replay(schedule.StatusReleased, 4); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale replay of archived row must be refused: %v", err)
	}
	// A caller-supplied id that is archived is a duplicate, not a new reservation.
	_, err := s.MaatReserve(schedule.Reservation{ID: id, Resource: "m1", Holder: "ra", Regime: schedule.RegimeBuild, Start: ts(now)}, false)
	if err == nil || !strings.Contains(err.Error(), "duplicate reservation id") {
		t.Fatalf("reserve with archived id: %v", err)
	}
	if hotIDs(t, s)[id] || archivedIDs(t, s)[id] != 1 {
		t.Fatal("duplicate reserve changed the ledger")
	}
}

func TestMaatEndedAtStampedByEveryTerminalTransition(t *testing.T) {
	s := openBackendStore(t, filepath.Join(t.TempDir(), "test.db"))
	defer s.Close()
	now := s.clock().UTC()
	mk := func(w string) string {
		r, err := s.MaatReserve(schedule.Reservation{Resource: "m1-" + w, Holder: "ra", Regime: schedule.RegimeBuild, Start: ts(now), EstEnd: ts(now.Add(time.Hour))}, false)
		if err != nil {
			t.Fatal(err)
		}
		return r.Reservation.ID
	}
	rel, relExit, queued := mk("a"), mk("b"), mk("c")
	if _, err := s.MaatMutate("release", rel, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MaatMutate("release", relExit, "exit=0"); err != nil {
		t.Fatal(err)
	}
	rows, err := s.MaatRead("status", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		switch r.ID {
		case rel, relExit:
			if _, e := time.Parse(time.RFC3339, r.EndedAt); e != nil {
				t.Fatalf("release did not stamp ended_at on %s: %q", r.ID, r.EndedAt)
			}
		case queued:
			if r.EndedAt != "" {
				t.Fatalf("live row stamped: %+v", r)
			}
		}
	}
}
