package main

import (
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knowledge"
	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

func TestSeshatCompatibilityCommandDefersOperatorAuthorityToMaat(t *testing.T) {
	if !seshatCmd.Hidden {
		t.Fatal("legacy seshat adapter must remain hidden from the default operator surface")
	}
	if !strings.Contains(seshatCmd.Short, "Ma'at") || !strings.Contains(seshatCmd.Long, "sirsi maat knowledge") {
		t.Fatalf("legacy seshat help does not direct operators to Ma'at knowledge: short=%q long=%q", seshatCmd.Short, seshatCmd.Long)
	}
}

func TestSafeMaatKnowledgeItemsWithholdsSensitiveLegacyRecords(t *testing.T) {
	sensitive := "pass" + "word" + "=" + "example" + "value"
	items := []seshat.KnowledgeItem{
		{Title: "Safe", Summary: "A normal locally retained note."},
		{Title: "Legacy", Summary: sensitive},
	}

	view := knowledge.Project(items, "")
	if view.Withheld != 1 {
		t.Fatalf("withheld = %d, want 1", view.Withheld)
	}
	if len(view.Items) != 1 || view.Items[0].Title != "Safe" {
		t.Fatalf("safe items = %#v, want only Safe", view.Items)
	}
}

func TestMaatKnowledgeRefreshResultIsTypedAndReturnsToCanonicalProjection(t *testing.T) {
	result := maatKnowledgeRefreshResult(1500 * time.Millisecond)
	if result.Command != "sirsi maat knowledge refresh" || result.Status != "ok" {
		t.Fatalf("refresh result = %+v", result)
	}
	if len(result.NextActions) != 1 || result.NextActions[0].Command != "sirsi maat knowledge --json" {
		t.Fatalf("refresh next action = %#v", result.NextActions)
	}
}

func TestSafeMaatKnowledgeItemsWithholdsSensitiveReference(t *testing.T) {
	secretRef := "token" + "=" + "example" + "value"
	items := []seshat.KnowledgeItem{{
		Title: "Reference test", Summary: "Metadata is inspected too.",
		References: []seshat.KIReference{{Type: "source", Value: secretRef}},
	}}

	view := knowledge.Project(items, "")
	if view.Withheld != 1 || len(view.Items) != 0 {
		t.Fatalf("view=%#v, want no records and one withheld", view)
	}
}
