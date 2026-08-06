package routerboard

import (
	"encoding/json"
	"testing"
)

// uiFields are the payload keys index.html actually reads.
//
// Extracted from the page itself, not from intent:
//
//	grep -oE "d\.[a-z_]+" index.html | sort -u
//
// This test exists because the payload shipped `board` while the page rendered
// `d.ledger`. Every tile bound to it read undefined, so the board displayed
// nonsense at the exact moment /api/ledger and the CLI returned correct numbers
// — the owner reported "8734 is BROKEN and reporting nonsense, the only
// truthteller is menubar," and both halves of that were true.
//
// A renamed or dropped key is invisible in Go: the struct still compiles, the
// JSON still marshals, and only the page breaks. Nothing else in the build can
// catch it, so it is asserted here.
var uiFields = []string{
	"activity",
	"build",
	"counters",
	"data_errors",
	"fleet",
	"ledger",
	"registration_gaps",
	"tasks",
	"threads",
}

func TestPayloadCarriesEveryFieldTheUIReads(t *testing.T) {
	body, err := json.Marshal(Payload{})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	for _, f := range uiFields {
		if _, ok := got[f]; !ok {
			t.Errorf("payload is missing %q — index.html reads it, so that section renders undefined", f)
		}
	}
}

// board and ledger must stay the SAME summary. They are two keys for one truth:
// the API endpoint serves `board`, the page reads `ledger`. If they ever diverge
// the surfaces disagree again, which is the whole defect this package removes.
func TestLedgerMirrorsBoard(t *testing.T) {
	fleet := []Lane{{
		Agent: "a", TasksTotal: 3, OpenItems: 2,
		Counts: map[string]int{"done": 1, "in-progress": 1, "blocked": 1},
	}}
	p := Payload{Board: summarize(fleet), Ledger: summarize(fleet)}

	b, err := json.Marshal(p.Board)
	if err != nil {
		t.Fatal(err)
	}
	l, err := json.Marshal(p.Ledger)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != string(l) {
		t.Errorf("board and ledger diverged:\n board=%s\nledger=%s", b, l)
	}
}

// blocked is a SUBSET of active, never a third independent segment. The menubar
// projection and the board both depend on this; counting blocked separately is
// how `router fleet --json` and the board disagreed.
func TestBlockedIsSubsetOfActive(t *testing.T) {
	s := summarize([]Lane{{
		Agent: "a", TasksTotal: 4,
		Counts: map[string]int{"done": 1, "in-progress": 1, "pending": 1, "blocked": 1},
	}})
	if s.ActiveTasks != 3 {
		t.Errorf("ActiveTasks = %d, want 3 (in-progress + pending + blocked)", s.ActiveTasks)
	}
	if s.BlockedTasks != 1 {
		t.Errorf("BlockedTasks = %d, want 1", s.BlockedTasks)
	}
	if s.BlockedTasks > s.ActiveTasks {
		t.Error("blocked exceeded active — it must be a subset")
	}
}

// Empty slices must marshal as [] and never null: the page calls .length on
// them, and null throws before anything renders.
func TestEmptyCollectionsMarshalAsArrays(t *testing.T) {
	body, err := json.Marshal(Payload{
		DataErrors: []string{}, Activity: []Event{}, Fleet: []Lane{},
		Threads: []Thread{}, RegistrationGaps: []string{}, Tasks: []TaskDetail{},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"data_errors", "activity", "fleet", "threads", "registration_gaps", "tasks"} {
		if string(got[f]) == "null" {
			t.Errorf("%s marshaled as null — the page calls .length on it and throws", f)
		}
	}
}
