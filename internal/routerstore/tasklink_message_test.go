package routerstore

import (
	"strings"
	"testing"
)

// TestEvidenceGateMessage_NamesKindAndShowsWhatYouHave guards the wording of the
// test-state gate, not its behaviour (behaviour is covered in store_test.go).
//
// The original message — "test-state passed requires an evidence link" — was
// read by callers who HAD just attached a link as "the gate is broken". Two
// agents independently recorded the gate as unsatisfiable and adopted "close
// without test-state" as the workaround, discarding the completion signal the
// gate exists to preserve. The message must therefore name the required KIND
// and report the kinds actually present.
func TestEvidenceGateMessage_NamesKindAndShowsWhatYouHave(t *testing.T) {
	base := Task{
		Agent: "a", TaskID: "t", Subject: "s", Status: "done",
		ResponsibleParty: "self", Stage: "shipped", TestState: "passed",
		CommissionedAt: "2026-08-06T00:00:00Z",
	}

	t.Run("wrong kind attached", func(t *testing.T) {
		task := base
		task.Links = []TaskLink{{Kind: "pr", Label: "543", URL: "https://example.test/543"}}
		err := validateTask(task)
		if err == nil {
			t.Fatal("expected rejection when no link has kind evidence")
		}
		for _, want := range []string{`"evidence"`, "pr", "--link"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("message %q missing %q", err.Error(), want)
			}
		}
	})

	t.Run("no links at all", func(t *testing.T) {
		task := base
		err := validateTask(task)
		if err == nil {
			t.Fatal("expected rejection with no links")
		}
		if !strings.Contains(err.Error(), "none") {
			t.Errorf("message %q should report kinds present as none", err.Error())
		}
	})

	t.Run("evidence kind is accepted", func(t *testing.T) {
		task := base
		task.Links = []TaskLink{
			{Kind: "pr", Label: "543", URL: "https://example.test/543"},
			{Kind: "evidence", Label: "ci", URL: "https://example.test/run"},
		}
		if err := validateTask(task); err != nil {
			t.Fatalf("evidence link should satisfy the gate, got %v", err)
		}
	})
}
