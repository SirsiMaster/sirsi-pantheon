//go:build darwin

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/trashreceipt"
	"github.com/spf13/cobra"
)

// Actual fresh Cobra commands, not direct RunE calls or a built CLI. Every
// filesystem operation is restricted to this test's private invoking HOME.
// No parallelism: HOME is process-scoped and restored by t.Setenv.
func TestTrashReceiptCobraOwnedSelectionFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".Trash")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	contents := map[string][]byte{"reviewed": []byte("owned reviewed leaf"), "unselected": []byte("owned unselected leaf")}
	for name, data := range contents {
		if err := os.WriteFile(filepath.Join(root, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	encode := func(value any) []byte {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > trashreceipt.MaxManifestBytes {
			t.Fatal("fixture exceeds canonical input budget")
		}
		return raw
	}
	execute := func(cmd *cobra.Command, input []byte, args ...string) ([]byte, error) {
		t.Helper()
		var out, diagnostic bytes.Buffer
		cmd.SetIn(bytes.NewReader(input))
		cmd.SetOut(&out)
		cmd.SetErr(&diagnostic)
		// A non-nil empty argument slice prevents Cobra from consuming the
		// enclosing go test process's flags when this invocation has no args.
		cmd.SetArgs(append([]string{}, args...))
		cmd.SilenceErrors, cmd.SilenceUsage = true, true
		err := cmd.Execute()
		return out.Bytes(), err
	}
	decode := func(raw []byte, target any) {
		t.Helper()
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if err := d.Decode(target); err != nil {
			t.Fatalf("Cobra stdout is not a typed JSON report: %v", err)
		}
		var extra any
		if err := d.Decode(&extra); err != io.EOF {
			t.Fatalf("Cobra stdout has trailing output: %v", err)
		}
	}
	runReport := func(cmd *cobra.Command, input []byte, args ...string) (trashreceipt.Report, error) {
		t.Helper()
		raw, err := execute(cmd, input, args...)
		var report trashreceipt.Report
		decode(raw, &report)
		if report.Schema != trashreceipt.ReportSchema {
			t.Fatal("wrong report schema", report.Schema)
		}
		return report, err
	}
	assertPreserved := func() {
		t.Helper()
		for name, want := range contents {
			got, err := os.ReadFile(filepath.Join(root, name))
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("owned leaf changed: %s: %v", name, err)
			}
		}
	}

	preview, err := runReport(newTrashReceiptCommand(false), nil)
	if err != nil || preview.Status != "ready" || preview.Preview == nil || len(preview.Preview.Entries) != 2 {
		t.Fatalf("Cobra preview: %+v %v", preview, err)
	}
	var selected []trashreceipt.Entry
	for _, entry := range preview.Preview.Entries {
		if entry.Path == "reviewed" {
			selected = append(selected, entry)
		}
	}
	if len(selected) != 1 {
		t.Fatal("preview did not expose exactly the owned reviewed leaf")
	}
	request := trashreceipt.SelectionRequest{Schema: trashreceipt.SelectionSchema, UID: preview.Preview.UID,
		Root: preview.Preview.Root, Containers: preview.Preview.Containers, Entries: selected}
	review, err := runReport(newTrashSelectionCommand(), encode(request))
	if err != nil || review.Status != "ready" || review.Preview == nil || !reflect.DeepEqual(review.Preview.Entries, selected) {
		t.Fatalf("Cobra bounded selection: %+v %v", review, err)
	}
	manifest := encode(review.Preview)
	contents["late"] = []byte("owned late addition")
	if err := os.WriteFile(filepath.Join(root, "late"), contents["late"], 0o600); err != nil {
		t.Fatal(err)
	}
	for _, refusal := range []struct {
		name  string
		input []byte
		args  []string
	}{
		{"no-confirm", manifest, nil},
		{"malformed", []byte(`{"schema":`), []string{"--confirm"}},
	} {
		report, err := runReport(newTrashReceiptCommand(true), refusal.input, refusal.args...)
		if err == nil || report.Status != "failed" || report.Attempted != 0 || len(report.Deleted) != 0 || len(report.Errors) == 0 {
			t.Fatalf("%s did not refuse with truthful JSON/error: %+v %v", refusal.name, report, err)
		}
		assertPreserved()
	}
	completed, err := runReport(newTrashReceiptCommand(true), manifest, "--confirm")
	if err != nil || completed.Status != "completed" || completed.Attempted != 1 ||
		!reflect.DeepEqual(completed.Deleted, []string{"reviewed"}) || len(completed.Errors) != 0 ||
		completed.LogicalBytes != int64(len(contents["reviewed"])) {
		t.Fatalf("Cobra confirmed selected-only apply: %+v %v", completed, err)
	}
	if _, err := os.Lstat(filepath.Join(root, "reviewed")); !os.IsNotExist(err) {
		t.Fatalf("reviewed owned leaf was not deleted: %v", err)
	}
	delete(contents, "reviewed")
	assertPreserved()

	browse := newTrashBrowseCommand()
	if !browse.Hidden {
		t.Fatal("unsupported stateless compatibility route is visible")
	}
	raw, err := execute(browse, encode(trashreceipt.BrowseRequest{Schema: trashreceipt.BrowseSchema, Directory: "."}))
	var page trashreceipt.Page
	decode(raw, &page)
	if err == nil || page.Status != "failed" || len(page.Entries) != 0 || page.Next != nil || len(page.Errors) == 0 {
		t.Fatalf("hidden compatibility route advertised apparent success: %+v %v", page, err)
	}
	assertPreserved()
}
