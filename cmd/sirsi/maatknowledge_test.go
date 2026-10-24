package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knowledge"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
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

func TestMaatKnowledgeRefreshReceiptDistinguishesUnopenedArchiveFailure(t *testing.T) {
	receipt := maatKnowledgeRefreshReceipt(time.Millisecond, knowledge.RefreshResult{
		State: knowledge.ImportFailed, Runs: []knowledge.ImportRun{}, ErrorCodes: []string{"archive_unavailable"},
	})
	if receipt.Status != string(knowledge.ImportFailed) || strings.Contains(receipt.Summary, "history was preserved") {
		t.Fatalf("archive-open failure receipt is misleading: %+v", receipt)
	}
	if len(receipt.Errors) != 1 || receipt.Errors[0] != "archive_unavailable" {
		t.Fatalf("safe typed failure codes = %#v", receipt.Errors)
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

func TestMaatKnowledgeReconcileCommandProjectsPendingOwnerReviewExactlyOnce(t *testing.T) {
	partial := maatKnowledgeProjectionResult(1, 2)
	if partial.Status != "partial" || len(partial.Errors) != 1 || partial.Errors[0] != "projection_debt_remains" || maatKnowledgeTestEvidence(*partial, "Projected") != 1 || maatKnowledgeTestEvidence(*partial, "Remaining") != 2 {
		t.Fatalf("incomplete projection receipt = %+v", partial)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	archive, err := knowledge.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	item := seshat.SourceRecord{
		Source: "claude", InstanceID: "fixture-profile", NativeItemID: "fixture-record",
		Schema: "fixture-v1", Title: "Local source", Summary: "Bounded summary", Content: "Retained body",
	}
	if _, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		_ = archive.Close()
		t.Fatal(err)
	}
	page, err := archive.Search(context.Background(), knowledge.SearchQuery{Limit: 20})
	if err != nil || len(page.Items) != 1 {
		_ = archive.Close()
		t.Fatalf("fixture record search = %+v, %v", page, err)
	}
	annotationID, err := archive.RecordOwnerAnnotation(context.Background(), page.Items[0].ID, page.Items[0].Revision, page.Items[0].ContentDigest, knowledge.ResolutionAccepted, "Owner accepted the retained evidence.")
	if err != nil {
		_ = archive.Close()
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(home, ".sirsi", "maat", "decisions.jsonl")
	oldFactory := newMaatDecisionJournal
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) {
		return &maat.FileDecisionJournal{Path: journalPath}, nil
	}
	t.Cleanup(func() { newMaatDecisionJournal = oldFactory })
	invoke := func() output.CommandResult {
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := maatKnowledgeReconcileCmd.RunE(cmd, nil); err != nil {
			t.Fatal(err)
		}
		var receipt output.CommandResult
		if err := json.Unmarshal(out.Bytes(), &receipt); err != nil {
			t.Fatalf("reconcile receipt %q: %v", out.String(), err)
		}
		return receipt
	}
	if receipt := invoke(); receipt.Command != "sirsi maat knowledge reconcile-projections" || receipt.Status != "ok" || maatKnowledgeTestEvidence(receipt, "Projected") != 1 || maatKnowledgeTestEvidence(receipt, "Remaining") != 0 {
		t.Fatalf("first reconcile receipt = %+v", receipt)
	}
	if receipt := invoke(); receipt.Command != "sirsi maat knowledge reconcile-projections" || receipt.Status != "ok" || maatKnowledgeTestEvidence(receipt, "Projected") != 0 || maatKnowledgeTestEvidence(receipt, "Remaining") != 0 {
		t.Fatalf("retry reconcile receipt = %+v", receipt)
	}
	rows, err := (&maat.FileDecisionJournal{Path: journalPath}).Recent(10)
	if err != nil || len(rows) != 1 || rows[0].Evidence != fmt.Sprintf("maat-knowledge-annotation:%d", annotationID) {
		t.Fatalf("projected decisions = %+v, %v", rows, err)
	}
}

func maatKnowledgeTestEvidence(receipt output.CommandResult, label string) int {
	for _, item := range receipt.Evidence {
		if item.Label == label {
			value, _ := strconv.Atoi(item.Value)
			return value
		}
	}
	return -1
}

func TestMaatKnowledgeResolveRequiresDisplayedRevisionAndDigest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	archive, err := knowledge.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	item := seshat.SourceRecord{
		Source: "claude", InstanceID: "fixture-profile", NativeItemID: "fixture-record",
		Schema: "fixture-v1", Title: "Reviewed source", Summary: "Bounded summary", Content: "Retained body",
	}
	if _, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		_ = archive.Close()
		t.Fatal(err)
	}
	page, err := archive.Search(context.Background(), knowledge.SearchQuery{Limit: 20})
	if err != nil || len(page.Items) != 1 {
		_ = archive.Close()
		t.Fatalf("fixture record = %+v, %v", page, err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	invoke := func(revision int, digest string) error {
		cmd := &cobra.Command{}
		cmd.Flags().String("state", "accepted", "")
		cmd.Flags().Int("revision", revision, "")
		cmd.Flags().String("digest", digest, "")
		cmd.Flags().Bool("note-stdin", true, "")
		cmd.SetContext(context.Background())
		cmd.SetIn(strings.NewReader("Owner reviewed the displayed version."))
		var out bytes.Buffer
		cmd.SetOut(&out)
		return maatKnowledgeResolveCmd.RunE(cmd, []string{page.Items[0].ID})
	}
	if err := invoke(page.Items[0].Revision+1, page.Items[0].ContentDigest); err == nil {
		t.Fatal("stale displayed revision unexpectedly resolved")
	}
	if err := invoke(page.Items[0].Revision, strings.Repeat("0", 64)); err == nil {
		t.Fatal("mismatched displayed digest unexpectedly resolved")
	}
	if err := invoke(page.Items[0].Revision, page.Items[0].ContentDigest); err != nil {
		t.Fatalf("resolve exact displayed version: %v", err)
	}

	archive, err = knowledge.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	detail, err := archive.Detail(context.Background(), page.Items[0].ID, 0)
	if err != nil || len(detail.Annotations) != 1 || detail.Annotations[0].ReviewedRevision != page.Items[0].Revision || detail.Annotations[0].ReviewedDigest != page.Items[0].ContentDigest {
		t.Fatalf("stored owner decision is not bound to displayed version: %+v, %v", detail, err)
	}
}

func TestSeshatCompatibilityReadsCanonicalArchiveWithoutLegacyFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	archive, err := knowledge.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	canonical := seshat.SourceRecord{
		Source: "claude", InstanceID: "fixture-profile", NativeItemID: "canonical-item",
		Schema: "fixture-v1", Title: "Canonical", Summary: "From Ma'at", Content: "Canonical body",
	}
	if _, err := archive.ImportSource(context.Background(), canonical.Source, []seshat.SourceRecord{canonical}, nil); err != nil {
		_ = archive.Close()
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	legacyDir := filepath.Join(home, ".config", "seshat", "store")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`[{"id":"legacy-only","title":"Legacy","summary":"Must not be a read fallback","references":[]}]`)
	if err := os.WriteFile(filepath.Join(legacyDir, "latest.json"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := loadLatestKnowledgeItems()
	if err != nil || len(items) != 1 || items[0].Title != "Canonical" || items[0].Content != "Canonical body" {
		t.Fatalf("compatibility projection = %+v, %v", items, err)
	}
	if err := seshatPruneCmd.RunE(&cobra.Command{}, nil); err == nil {
		t.Fatal("compatibility prune did not refuse immutable history")
	}
}

func TestSeshatIngestAliasRefusesUnconfiguredSourceWithoutOpeningArchive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cmd := &cobra.Command{}
	cmd.Flags().String("source", "google-workspace", "")
	cmd.Flags().String("since", "", "")
	cmd.Flags().String("export", "", "")
	cmd.Flags().String("profile", "", "")
	cmd.Flags().Bool("all-profiles", false, "")
	cmd.SetContext(context.Background())
	if err := seshatIngestCmd.RunE(cmd, nil); err == nil {
		t.Fatal("Seshat compatibility alias accepted an unconfigured online source")
	}
	if _, err := os.Stat(filepath.Join(home, ".sirsi")); !os.IsNotExist(err) {
		t.Fatalf("refusing unsupported source opened or created the archive: %v", err)
	}
}
