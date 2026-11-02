package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

type lifecycleCommandResolver struct{ value maat.LifecycleEvidence }

func (r lifecycleCommandResolver) ResolveLifecycle(_ context.Context, id string) (maat.LifecycleEvidence, error) {
	if id != r.value.ID {
		return maat.LifecycleEvidence{}, fmt.Errorf("unknown receipt")
	}
	return r.value, nil
}

func TestMaatLifecycleActualCommandPreviewConfirmUnknownOnceAndScopeRefusals(t *testing.T) {
	policy := maat.QualificationPolicy{ID: "inert-policy", Threshold: .9, BoundaryThreshold: .98, MinimumPass: 1, MinimumChanges: 1}
	raw, _ := json.Marshal(policy)
	sum := sha256.Sum256(raw)
	r := maat.LifecycleReceipt{Schema: maat.LifecycleSchema, Kind: "policy", Profile: maat.SourceProfileIdentity{Producer: "producer", ArtifactSHA: strings.Repeat("a", 64), ModelVersion: "v1", PromptSHA: strings.Repeat("b", 64), RecipeSHA: strings.Repeat("c", 64), PolicySHA: hex.EncodeToString(sum[:])}, Repo: "fixture/repo", HeadSHA: strings.Repeat("a", 40), Policy: policy, RunID: "owned-inert", Samples: []maat.LifecycleSample{}, SelectedIDs: []string{}}
	raw, _ = json.Marshal(r)
	sum = sha256.Sum256(raw)
	resolver := lifecycleCommandResolver{maat.LifecycleEvidence{ID: "review", SHA: hex.EncodeToString(sum[:]), Issuer: "canonical-claimed-sender", Receipt: r, Reason: "canonical custody only; acceptance unknown"}}
	j := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	run := func(args ...string) (maat.LifecycleObservation, error) {
		cmd := newMaatLifecycleCmdWith(func() (lifecycleJournal, error) { return j, nil }, func() (maat.LifecycleResolver, func(), error) { return resolver, func() {}, nil })
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetErr(&output)
		cmd.SetArgs(args)
		err := cmd.Execute()
		var view maat.LifecycleObservation
		if err == nil {
			err = json.Unmarshal(output.Bytes(), &view)
		}
		return view, err
	}
	base := []string{"--receipt-id", "review", "--expect-repo", r.Repo, "--expect-head", r.HeadSHA, "--json"}
	preview, err := run(append(append([]string{}, base...), "--preview")...)
	if err != nil {
		t.Fatal(err)
	}
	if preview.State.Status != "unqualified" || preview.Recorded {
		t.Fatalf("preview=%+v", preview)
	}
	if _, err := os.Stat(j.Path); !os.IsNotExist(err) {
		t.Fatal("preview wrote journal")
	}
	if _, err := run(append(append([]string{}, base...), "--confirm")...); err == nil {
		t.Fatal("unreviewed confirmation admitted")
	}
	for _, args := range [][]string{{"--receipt-id", "review", "--expect-repo", "other/repo", "--expect-head", r.HeadSHA}, {"--receipt-id", "review", "--expect-repo", r.Repo, "--expect-head", strings.Repeat("f", 40)}, append(append([]string{}, base...), "--confirm", "--expect-receipt-sha256", strings.Repeat("0", 64)), append(append([]string{}, base...), "--preview", "--confirm")} {
		if _, err := run(args...); err == nil {
			t.Fatal("wrong scope/drift/mixed intent admitted")
		}
	}
	confirmed := append(append([]string{}, base...), "--expect-receipt-sha256", preview.ReceiptSHA, "--confirm")
	view, err := run(confirmed...)
	if err != nil || !view.Recorded || view.State.Status != "unqualified" {
		t.Fatalf("record=%+v %v", view, err)
	}
	before, _ := os.ReadFile(j.Path)
	view, err = run(confirmed...)
	if err != nil || !view.Duplicate || view.Recorded {
		t.Fatalf("repeat=%+v %v", view, err)
	}
	after, _ := os.ReadFile(j.Path)
	if !bytes.Equal(before, after) {
		t.Fatal("duplicate command appended")
	}
}
