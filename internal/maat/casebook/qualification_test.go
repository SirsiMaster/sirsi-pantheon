package casebook

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

func TestLifecycleProjectionNeverPromotesStoredDeterminationToAcceptance(t *testing.T) {
	e := maat.QualificationEvent{Schema: maat.LifecycleSchema, ReceiptID: "review", ReceiptSHA: strings.Repeat("a", 64), HeadSHA: strings.Repeat("b", 40), Profile: maat.SourceProfileIdentity{Producer: "fixture", ModelVersion: "v1", ArtifactSHA: strings.Repeat("c", 64), PromptSHA: strings.Repeat("d", 64), RecipeSHA: strings.Repeat("e", 64), PolicySHA: strings.Repeat("f", 64)}}
	view := Search([]maat.Decision{{Time: "2026-10-24T00:00:00Z", Host: "fixture", Kind: "source qualification observation", Requester: "fixture", Resource: "fixture/repo", Assessed: "lifecycle", Determination: "qualified", Why: "stored declaration", Evidence: "fixture:event", SourceQualification: &e}}, Query{})
	if len(view.Cases) != 1 {
		t.Fatal("observation hidden")
	}
	c := view.Cases[0]
	if c.Determination != "unqualified" || c.Status != StatusOpen || c.Category != "governance" || c.SourceQualification == nil || !strings.Contains(c.Why, "not current verified acceptance") {
		t.Fatalf("stored row promoted: %+v", c)
	}
}
