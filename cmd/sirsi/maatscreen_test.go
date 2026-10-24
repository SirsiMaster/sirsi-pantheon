package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

const maatSourceCLIInput = `{
 "contract":"maat.adr070.source-screen","schema_version":2,
 "subject":{"kind":"commit","repo":"fixture/repo","ref":"main","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","boundary":""},
 "feather_weight":92,"confidence":0.96,"findings":[],
 "floor":{"passed":true,"checks":[{"name":"gofmt","passed":true},{"name":"vet","passed":true},{"name":"golangci-lint","passed":true},{"name":"test-race","passed":true}]},
 "model":{"provider":"local:fixture","version":"fixture-v1","local":true,"latency_ms":0,
 "evidence_set_digest":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
 "calibration_id":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","calibration":"unqualified"}
}`

func maatSourceCLIFixture(t *testing.T) (*maatTestJournal, string) {
	t.Helper()
	oldFactory, oldInput, oldConfirm, oldJSON, oldMaatJSON, oldHead := newMaatDecisionJournal, maatScreenInput, maatScreenConfirm, JsonOutput, maatJSON, maatScreenExpectedHead
	t.Cleanup(func() {
		newMaatDecisionJournal, maatScreenInput, maatScreenConfirm, JsonOutput, maatJSON, maatScreenExpectedHead = oldFactory, oldInput, oldConfirm, oldJSON, oldMaatJSON, oldHead
	})
	j := &maatTestJournal{}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return j, nil }
	path := filepath.Join(t.TempDir(), "source.json")
	maatScreenInput, maatScreenExpectedHead, maatScreenConfirm, JsonOutput, maatJSON = path, strings.Repeat("a", 40), true, true, false
	return j, path
}

func TestMaatScreenRecordsClosedTypedVerdict(t *testing.T) {
	j, path := maatSourceCLIFixture(t)
	if err := os.WriteFile(path, []byte(maatSourceCLIInput), 0600); err != nil {
		t.Fatal(err)
	}
	if err := maatScreenCmd.RunE(maatScreenCmd, nil); err != nil {
		t.Fatal(err)
	}
	if len(j.decisions) != 1 || j.decisions[0].SystemOne == nil || j.decisions[0].SystemOne.Gate != maat.GateEscalate || j.decisions[0].SystemOne.SchemaVersion != 2 {
		t.Fatalf("decisions=%+v", j.decisions)
	}
	if !strings.Contains(systemOneSummary(*j.decisions[0].SystemOne), "declared only") {
		t.Fatal("claimed executed floor")
	}
}

func TestMaatScreenRejectsUnknownOrTrailingInput(t *testing.T) {
	for _, raw := range []string{`{"unknown":true}`, maatSourceCLIInput + `{}`,
		strings.Replace(maatSourceCLIInput, `"schema_version":2`, `"schema_version":2,"schema_version":2`, 1),
		strings.Replace(maatSourceCLIInput, `"local":true`, `"LOCAL":true`, 1),
		strings.Replace(maatSourceCLIInput, `"unqualified"`, `"qualified"`, 1),
		strings.Replace(maatSourceCLIInput, `"schema_version":2`, `"schema_version":3`, 1),
		`{"subject":{"kind":"commit"},"confidence":1}`,
	} {
		t.Run(raw[:min(len(raw), 30)], func(t *testing.T) {
			j, path := maatSourceCLIFixture(t)
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			if err := maatScreenCmd.RunE(maatScreenCmd, nil); err == nil {
				t.Fatal("invalid packet admitted")
			}
			if len(j.decisions) != 0 {
				t.Fatal("invalid input wrote journal")
			}
		})
	}
}

func TestMaatScreenRequiresConfirmationBeforeJournalWrite(t *testing.T) {
	j, path := maatSourceCLIFixture(t)
	if err := os.WriteFile(path, []byte(maatSourceCLIInput), 0600); err != nil {
		t.Fatal(err)
	}
	maatScreenConfirm = false
	if err := maatScreenCmd.RunE(maatScreenCmd, nil); err == nil || !strings.Contains(err.Error(), "--confirm") {
		t.Fatalf("err=%v", err)
	}
	if len(j.decisions) != 0 {
		t.Fatal("unconfirmed write")
	}
}

func TestMaatScreenExpectedHeadAndDeclaredFloor(t *testing.T) {
	j, path := maatSourceCLIFixture(t)
	if err := os.WriteFile(path, []byte(maatSourceCLIInput), 0600); err != nil {
		t.Fatal(err)
	}
	for _, head := range []string{"", strings.Repeat("e", 40)} {
		maatScreenExpectedHead = head
		if err := maatScreenCmd.RunE(maatScreenCmd, nil); err == nil {
			t.Fatal("expected head bypass")
		}
	}
	maatScreenExpectedHead = strings.Repeat("a", 40)
	raw := strings.Replace(maatSourceCLIInput, `"floor":{"passed":true`, `"floor":{"passed":false`, 1)
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := maatScreenCmd.RunE(maatScreenCmd, nil); err != nil {
		t.Fatal(err)
	}
	if len(j.decisions) != 1 || j.decisions[0].SystemOne.Gate != maat.GateBlock {
		t.Fatal("floor did not precede unqualified")
	}
}

func TestMaatScreenBoundedRegularInput(t *testing.T) {
	_, path := maatSourceCLIFixture(t)
	if err := os.WriteFile(path, []byte(strings.Repeat(" ", maat.SourceScreenMaxBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readMaatSourceScreen(path); err == nil {
		t.Fatal("oversize admitted")
	}
	if _, err := readMaatSourceScreen(filepath.Dir(path)); err == nil {
		t.Fatal("directory admitted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := readMaatSourceScreen(path); err == nil {
		t.Fatal("missing admitted")
	}
}
