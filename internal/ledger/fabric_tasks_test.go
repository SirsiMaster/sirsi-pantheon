package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brand"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func fabricTaskFixture(count int) Snapshot {
	s := Snapshot{GeneratedAt: "2026-10-25T12:00:00Z", Agents: []Agent{{AgentID: "lane"}}}
	for i := 0; i < count; i++ {
		s.Agents[0].Tasks = append(s.Agents[0].Tasks, routerstore.Task{
			Agent: "lane", TaskID: fmt.Sprintf("task-%03d", i), Subject: fmt.Sprintf("Inert task %d", i), Status: "pending",
			ResponsibleParty: "fixture-owner", Stage: "build", TestState: "untested", Created: s.GeneratedAt, Updated: s.GeneratedAt,
			CommissionedAt: s.GeneratedAt, CommissionedBy: "fixture-lead",
		})
	}
	return s
}

func TestFabricTasksPublicProjectionExcludesPrivatePayloadAndBindsProvenance(t *testing.T) {
	s := fabricTaskFixture(3)
	secret := "PRIVATE-LEASE-JOURNAL-CREDENTIAL-CANARY"
	task := &s.Agents[0].Tasks[0]
	task.Charter, task.Outline = &secret, &secret
	task.Phase, task.ResultRef, task.BlockedBy = secret, "/private/"+secret, secret
	task.Timeline = []routerstore.TimelineEntry{{Label: secret}}
	task.Links = []routerstore.TaskLink{
		{Kind: "evidence", URL: "https://github.com/SirsiMaster/sirsi-pantheon/pull/1"},
		{Kind: "evidence", URL: "file:///private/" + secret},
		{Kind: "evidence", URL: "https://github.com/x/y?token=" + secret},
		{Kind: "evidence", URL: "https://user:" + secret + "@github.com/x/y"},
	}
	s.Agents[0].Tasks[1].Status = "blocked"
	s.Agents[0].Tasks[1].BlockedBy = task.TaskID
	s.Agents[0].Tasks[2].Status = "done"
	before, _ := json.Marshal(s)
	p, err := FabricTasksFrom(s, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "file://") || strings.Contains(string(raw), "claimed_by") {
		t.Fatalf("private task payload escaped allowlist: %s", raw)
	}
	if p.Total != 2 || len(p.Rows) != 2 || p.Rows[0].ID != task.TaskID || p.Rows[0].Subject != task.Subject || !p.Rows[0].Blocked || p.Rows[0].BlockerID != "" {
		t.Fatalf("public task identity/blocker projection lost: %+v", p)
	}
	origin := p.Rows[0].Provenance
	if origin.Source != "canonical-task-registry" || origin.CommissionedBy != "fixture-lead" || !origin.HasCharter || !origin.HasOutline || !origin.HasResult || len(origin.Links) != 1 || origin.WithheldLinks != 3 {
		t.Fatalf("provenance/refusal truth lost: %+v", origin)
	}
	if p.Rows[1].BlockerID != task.TaskID || p.Rows[1].Role != brand.Warn.Name() {
		t.Fatal("same-snapshot task dependency or blocked role missing")
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("projection mutated canonical input")
	}
}

func TestFabricTaskPagesCoverEveryOpenRowAndRefuseChangedSnapshot(t *testing.T) {
	s := fabricTaskFixture(130)
	first, err := FabricTasksFrom(s, 0, "")
	if err != nil || first.Total != 130 || len(first.Rows) != 64 || first.NextOffset == nil || *first.NextOffset != 64 {
		t.Fatalf("first bounded page: %+v %v", first, err)
	}
	second, err := FabricTasksFrom(s, *first.NextOffset, first.Snapshot)
	if err != nil || second.NextOffset == nil || *second.NextOffset != 128 {
		t.Fatalf("second page: %+v %v", second, err)
	}
	last, err := FabricTasksFrom(s, *second.NextOffset, second.Snapshot)
	if err != nil || len(last.Rows) != 2 || last.NextOffset != nil {
		t.Fatalf("last page: %+v %v", last, err)
	}
	seen := map[string]bool{}
	for _, page := range []FabricTaskPage{first, second, last} {
		for _, row := range page.Rows {
			if seen[row.ID] {
				t.Fatal("duplicate across pages")
			}
			seen[row.ID] = true
		}
	}
	if len(seen) != 130 || !seen["task-129"] {
		t.Fatal("tail task hidden by a hard cap")
	}
	s.GeneratedAt = "2026-10-25T12:00:01Z"
	again, err := FabricTasksFrom(s, 64, first.Snapshot)
	if err != nil || !reflect.DeepEqual(second, again) {
		t.Fatal("collection-time-only change invalidated unchanged public rows")
	}
	s.Agents[0].Tasks[129].Subject = "Changed canonical task"
	if _, err := FabricTasksFrom(s, 64, first.Snapshot); !errors.Is(err, ErrFabricTasksChanged) {
		t.Fatalf("changed continuation must refuse, got %v", err)
	}
}

func TestFabricTaskProjectionBoundsIdentityAndUnavailableAreExplicit(t *testing.T) {
	s := fabricTaskFixture(1)
	s.Agents[0].Tasks[0].Subject = strings.Repeat("界", 600)
	p, err := FabricTasksFrom(s, 0, "")
	if err != nil || len([]rune(p.Rows[0].Subject)) != 512 || !p.Rows[0].TextLimited {
		t.Fatal("subject bound undisclosed")
	}
	for _, offset := range []int{-1, 1, 64} {
		if _, err := FabricTasksFrom(s, offset, p.Snapshot); err == nil {
			t.Fatalf("invalid offset %d accepted", offset)
		}
	}
	s.Agents[0].Tasks = append(s.Agents[0].Tasks, s.Agents[0].Tasks[0])
	if _, err := FabricTasksFrom(s, 0, ""); err == nil {
		t.Fatal("duplicate task identity accepted")
	}
	b := FabricFrom(s, nil, "fixture")
	if b.Tasks.State != "unavailable" || b.Work.Open != 2 {
		t.Fatal("unavailable detail was mislabeled as an empty ledger or changed aggregate")
	}
	empty, err := FabricTasksFrom(Snapshot{}, 0, "")
	if err != nil || empty.State != "observed" || empty.Total != 0 || empty.Rows == nil || empty.NextOffset != nil {
		t.Fatal("known empty differs from missing task observation")
	}
}

func TestFabricSemanticRolesUseExistingBrandTokensWithoutReclassifying(t *testing.T) {
	p := fabricPresentation()
	if p.Headlines["completed"] != "ok" || p.Headlines["progress"] != "gold" || p.Headlines["stalled"] != "warn" || p.Headlines["messages"] != "ink2" {
		t.Fatal("original semantic roles diverged")
	}
	for _, role := range []brand.Role{brand.OK, brand.Gold, brand.Warn, brand.Ink2, brand.Dim} {
		if p.Palette[role.Name()] != (FabricSwatch{Dark: brand.For(brand.Dark).Hex(role), Light: brand.For(brand.Light).Hex(role)}) {
			t.Fatal("palette duplicated rather than inherited from brand")
		}
	}
	for state, role := range map[string]string{"WORKING": "ok", "COMPLETE": "ok", "ASSIGNED": "gold", "IDLE_WITH_WORK": "warn", "BLOCKED": "warn", "UNROUTABLE": "warn", "UNKNOWN": "dim"} {
		if fabricLaneRole(state) != role {
			t.Fatalf("%s mapped to wrong role", state)
		}
	}
	b := FabricFrom(fabricTaskFixture(1), nil, "fixture")
	if b.Lanes[0].Role != "dim" || b.Lanes[0].State != "UNKNOWN" {
		t.Fatal("a task status manufactured a runtime role")
	}
}
