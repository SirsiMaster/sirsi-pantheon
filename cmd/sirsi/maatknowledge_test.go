package main

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

func TestSafeMaatKnowledgeItemsWithholdsSensitiveLegacyRecords(t *testing.T) {
	sensitive := "pass" + "word" + "=" + "example" + "value"
	items := []seshat.KnowledgeItem{
		{Title: "Safe", Summary: "A normal locally retained note."},
		{Title: "Legacy", Summary: sensitive},
	}

	safe, withheld := safeMaatKnowledgeItems(items)
	if withheld != 1 {
		t.Fatalf("withheld = %d, want 1", withheld)
	}
	if len(safe) != 1 || safe[0].Title != "Safe" {
		t.Fatalf("safe items = %#v, want only Safe", safe)
	}
}

func TestSafeMaatKnowledgeItemsWithholdsSensitiveReference(t *testing.T) {
	secretRef := "token" + "=" + "example" + "value"
	items := []seshat.KnowledgeItem{{
		Title: "Reference test", Summary: "Metadata is inspected too.",
		References: []seshat.KIReference{{Type: "source", Value: secretRef}},
	}}

	safe, withheld := safeMaatKnowledgeItems(items)
	if withheld != 1 || len(safe) != 0 {
		t.Fatalf("safe=%#v withheld=%d, want no records and one withheld", safe, withheld)
	}
}
