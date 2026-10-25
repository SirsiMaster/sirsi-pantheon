package tui

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/casebook"
)

func historyFixture(snapshot, query, next string) maatCasebookReport {
	return maatCasebookReport{Cases: []maatCase{{ID: snapshot, Category: "fixture", Evidence: "old:evidence"}}, History: casebook.HistoryWindow{Coverage: "complete", Snapshot: snapshot, QueryID: query, NextCursor: next, CountScope: "query", Returned: 1}}
}
func TestMaatHistoryTUIStaleSuccessFailureAndCancellation(t *testing.T) {
	old := getRunner()
	t.Cleanup(func() { setRunner(old) })
	setRunner(RunnerFunc(func(ctx context.Context, verb string, args ...string) ([]byte, error) {
		return json.Marshal(historyFixture("fresh", "query-b", ""))
	}))
	s := newActivityScreen()
	t.Cleanup(s.CancelRead)
	s.showMaat = true
	_ = s.loadMaat("")
	first := s.maatGeneration
	s.maatQuery = "new query"
	second := s.loadMaat("")
	s.Update(second(), Capabilities{})
	if s.maatReport.History.Snapshot != "fresh" {
		t.Fatal("current read did not commit")
	}
	for _, err := range []error{nil, errors.New("delayed old failure")} {
		s.Update(maatCasebookLoaded{generation: first, report: historyFixture("old", "query-a", ""), err: err}, Capabilities{})
	}
	if s.maatErr != nil || s.maatReport.History.Snapshot != "fresh" || s.maatState != stateReady {
		t.Fatal("late read replaced current projection")
	}
	_ = s.loadMaat("")
	cancelled := s.maatGeneration
	s.CancelRead()
	s.Update(maatCasebookLoaded{generation: cancelled, report: historyFixture("cancelled", "q", "")}, Capabilities{})
	if s.maatState != stateIdle || s.maatReport.History.Snapshot == "cancelled" {
		t.Fatal("cancelled read committed")
	}
}
func TestMaatHistoryTUIActualContinuationArgsAndDrift(t *testing.T) {
	old := getRunner()
	t.Cleanup(func() { setRunner(old) })
	var argv []string
	setRunner(RunnerFunc(func(ctx context.Context, verb string, args ...string) ([]byte, error) {
		argv = append([]string{verb}, args...)
		return json.Marshal(historyFixture("changed", "q", ""))
	}))
	s := newActivityScreen()
	t.Cleanup(s.CancelRead)
	s.showMaat = true
	s.maatQuery = "-older match"
	s.maatScope = "system-one"
	s.maatStatus = "resolved"
	s.maatReport = historyFixture("original", "q", "opaque-cursor")
	s.maatState = stateReady
	_, cmd := s.handleMaatCmd(Command{ID: CmdMaatContinue})
	if cmd == nil {
		t.Fatal("continue is not wired")
	}
	s.Update(cmd(), Capabilities{})
	joined := strings.Join(argv, "|")
	for _, part := range []string{"maat|casebook", "--scope|system-one", "--status|resolved", "--cursor|opaque-cursor", "--|-older match"} {
		if !strings.Contains(joined, part) {
			t.Fatalf("missing %s in %s", part, joined)
		}
	}
	if s.maatState != stateError || s.maatReport.History.Snapshot != "original" {
		t.Fatal("snapshot drift replaced current page")
	}
	_, retry := s.handleMaatCmd(Command{ID: CmdMaatContinue})
	if retry == nil {
		t.Fatal("failed continuation has no retry")
	}
}
func TestMaatHistoryTUIPartialEmptyAndCanonicalRunnerRefusal(t *testing.T) {
	runner := RunnerFunc(func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`{"cases":[],"summary":{},"history":{"coverage":"partial","next_cursor":"same-snapshot"}}`), nil
	})
	r, err := readMaatReport(context.Background(), runner, nil)
	if err != nil || r.History.NextCursor == "" {
		t.Fatalf("partial wire=%+v %v", r, err)
	}
	s := newActivityScreen()
	t.Cleanup(s.CancelRead)
	s.showMaat = true
	s.maatState = stateReady
	s.maatReport = r
	if text := strings.Join(s.maatView(30, Capabilities{}), "\n"); !strings.Contains(text, "No provisional") || strings.Contains(text, "No cases on") {
		t.Fatalf("partial disguised as empty: %s", text)
	}
	for _, raw := range []string{`{}`, `{"cases":null,"history":{"coverage":"complete"}}`, `{"cases":[{"id":"provisional"}],"history":{"coverage":"partial"}}`} {
		bad := RunnerFunc(func(context.Context, string, ...string) ([]byte, error) { return []byte(raw), nil })
		if _, err = readMaatReport(context.Background(), bad, nil); err == nil {
			t.Fatalf("invalid wire accepted: %s", raw)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	noRead := RunnerFunc(func(context.Context, string, ...string) ([]byte, error) { calls++; return nil, nil })
	if _, err = readMaatReport(ctx, noRead, nil); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("pre-cancel called runner")
	}
	for _, path := range []string{"", "sirsi", "./sirsi"} {
		if _, err = (execRunner{self: path}).RunJSON(context.Background(), "maat"); err == nil {
			t.Fatalf("noncanonical executable accepted: %q", path)
		}
	}
}
