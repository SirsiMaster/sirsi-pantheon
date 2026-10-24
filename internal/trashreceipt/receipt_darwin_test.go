//go:build darwin

package trashreceipt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ownedTrash(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, ".Trash")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return home, root
}

func put(t *testing.T, p string) {
	t.Helper()
	if err := os.WriteFile(p, []byte("owned inert fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
}
func mustPreview(t *testing.T, home string) Manifest {
	t.Helper()
	r := previewHome(home)
	if r.Preview == nil {
		t.Fatalf("preview: %+v", r)
	}
	return *r.Preview
}

func TestTrashReceiptPreflightRefusesAnyDrift(t *testing.T) {
	for _, change := range []string{"new-nested", "root-replaced", "leaf-replaced"} {
		t.Run(change, func(t *testing.T) {
			home, root := ownedTrash(t)
			if err := os.Mkdir(filepath.Join(root, "folder"), 0o700); err != nil {
				t.Fatal(err)
			}
			leaf := filepath.Join(root, "folder", "old")
			put(t, leaf)
			m := mustPreview(t, home)
			switch change {
			case "new-nested":
				put(t, filepath.Join(root, "folder", "new"))
			case "root-replaced":
				if err := os.Rename(root, root+"-retained"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
			case "leaf-replaced":
				if err := os.Rename(leaf, leaf+"-retained"); err != nil {
					t.Fatal(err)
				}
				put(t, leaf)
			}
			r := applyHome(home, m, true, nil)
			if r.Status != "review_required" || r.Attempted != 0 || len(r.Deleted) != 0 {
				t.Fatalf("drift must refuse before deletion: %+v", r)
			}
		})
	}
}

func TestTrashReceiptLateNestedAdditionSurvivesPartialPrefix(t *testing.T) {
	home, root := ownedTrash(t)
	dir := filepath.Join(root, "folder")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(dir, "reviewed"))
	m := mustPreview(t, home)
	r := applyHome(home, m, true, func() { put(t, filepath.Join(dir, "late")) })
	if r.Status != "partial" || len(r.Deleted) != 1 || r.Deleted[0] != "folder/reviewed" || r.Attempted != 2 || len(r.Errors) == 0 {
		t.Fatalf("partial prefix: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(dir, "late")); err != nil {
		t.Fatal("unreviewed nested addition was not preserved", err)
	}
}

func TestTrashReceiptLateTopAdditionIsNotDiscovered(t *testing.T) {
	home, root := ownedTrash(t)
	put(t, filepath.Join(root, "reviewed"))
	m := mustPreview(t, home)
	r := applyHome(home, m, true, func() { put(t, filepath.Join(root, "late")) })
	if r.Status != "completed" || len(r.Deleted) != 1 || r.Deleted[0] != "reviewed" {
		t.Fatalf("reviewed operation: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(root, "late")); err != nil {
		t.Fatal("late top-level addition lost", err)
	}
}

func TestTrashReceiptEmptyFailedAndInvalidPreview(t *testing.T) {
	home, root := ownedTrash(t)
	if r := previewHome(home); r.Status != "empty" {
		t.Fatalf("empty: %+v", r)
	}
	if err := os.Rename(root, root+"-retained"); err != nil {
		t.Fatal(err)
	}
	if r := previewHome(home); r.Status != "failed" || r.Preview != nil {
		t.Fatalf("failed read cannot mean empty: %+v", r)
	}
	if err := os.Symlink(root+"-retained", root); err != nil {
		t.Fatal(err)
	}
	if r := previewHome(home); r.Status != "failed" {
		t.Fatalf("root symlink: %+v", r)
	}
}

func TestTrashReceiptStrictManifestInput(t *testing.T) {
	home, _ := ownedTrash(t)
	m := mustPreview(t, home)
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(strings.NewReader(string(raw))); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{}`, string(raw) + ` {}`, strings.Replace(string(raw), `"schema":`, `"extra":1,"schema":`, 1), strings.Replace(string(raw), `"schema":`, `"schema":"duplicate","schema":`, 1), string(raw) + strings.Repeat(" ", MaxManifestBytes)} {
		if _, err := Decode(strings.NewReader(bad)); err == nil {
			t.Fatal("invalid input admitted")
		}
	}
	if r := applyHome(home, m, false, nil); r.Status != "review_required" || r.Attempted != 0 {
		t.Fatalf("missing consent: %+v", r)
	}
}

func TestTrashReceiptExactKeySetsRejectCaseAliasesAndExtras(t *testing.T) {
	// Pure wire fixture: no home, Trash directory or deletion is consulted.
	root := Identity{Device: 1, Inode: 2, UID: 501, Mode: 0o040700, Type: "directory"}
	leaf := Identity{Device: 1, Inode: 3, UID: 501, Mode: 0o100600, Type: "file"}
	m := Manifest{Schema: ManifestSchema, UID: 501, Root: root, Containers: []Entry{}, Entries: []Entry{{Path: "owned-fixture", Identity: leaf}}}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(strings.NewReader(string(raw))); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"manifest", "root", "member", "identity"} {
		objectAt := func(document map[string]any) map[string]any {
			switch scope {
			case "root":
				return document["root"].(map[string]any)
			case "member":
				return document["entries"].([]any)[0].(map[string]any)
			case "identity":
				return document["entries"].([]any)[0].(map[string]any)["identity"].(map[string]any)
			default:
				return document
			}
		}
		var original map[string]any
		if err := json.Unmarshal(raw, &original); err != nil {
			t.Fatal(err)
		}
		for key := range objectAt(original) {
			t.Run(scope+"/alias-"+key, func(t *testing.T) {
				var document map[string]any
				if err := json.Unmarshal(raw, &document); err != nil {
					t.Fatal(err)
				}
				target := objectAt(document)
				// Keep every canonical key and add a case alias with the same
				// value: Go's ordinary struct decoder would accept this extra.
				target[strings.ToUpper(key)] = target[key]
				bad, err := json.Marshal(document)
				if err != nil {
					t.Fatal(err)
				}
				if err := requiredManifestFields(bad); err == nil {
					t.Fatal("exact-key validator admitted case alias")
				}
				if _, err := Decode(strings.NewReader(string(bad))); err == nil {
					t.Fatal("manifest decoder admitted case alias")
				}
			})
		}
		t.Run(scope+"/unknown-extra", func(t *testing.T) {
			var document map[string]any
			if err := json.Unmarshal(raw, &document); err != nil {
				t.Fatal(err)
			}
			objectAt(document)["unexpected"] = true
			bad, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			if err := requiredManifestFields(bad); err == nil {
				t.Fatal("exact-key validator admitted extra key")
			}
		})
	}
}

func TestTrashReceiptRejectsInvalidMembersAndBounds(t *testing.T) {
	home, root := ownedTrash(t)
	put(t, filepath.Join(root, "file"))
	m := mustPreview(t, home)
	for _, name := range []string{"schema", "parent", "duplicate", "type", "uid", "size", "entries"} {
		t.Run(name, func(t *testing.T) {
			bad := m
			bad.Entries = append([]Entry(nil), m.Entries...)
			switch name {
			case "schema":
				bad.Schema = "unsupported"
			case "parent":
				bad.Entries[0].Path = ".."
			case "duplicate":
				bad.Entries = append(bad.Entries, bad.Entries[0])
			case "type":
				bad.Entries[0].Identity.Type = "directory"
			case "uid":
				bad.Entries[0].Identity.UID++
			case "size":
				bad.Entries[0].Identity.Size = -1
			case "entries":
				bad.Entries = make([]Entry, MaxEntries+1)
			}
			if err := Validate(bad); err == nil {
				t.Fatal("invalid manifest admitted")
			}
			r := applyHome(home, bad, true, nil)
			if r.Status != "review_required" || r.Attempted != 0 {
				t.Fatalf("invalid review attempted delete: %+v", r)
			}
			if _, err := os.Stat(filepath.Join(root, "file")); err != nil {
				t.Fatal("invalid review changed the owned fixture", err)
			}
		})
	}
}

func TestTrashReceiptPostPreflightBindingsAndSymlinkLeaf(t *testing.T) {
	for _, kind := range []string{"root", "leaf"} {
		t.Run(kind, func(t *testing.T) {
			home, root := ownedTrash(t)
			leaf := filepath.Join(root, "file")
			put(t, leaf)
			m := mustPreview(t, home)
			r := applyHome(home, m, true, func() {
				if kind == "root" {
					if err := os.Rename(root, filepath.Join(home, "retained-root")); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(root, 0o700); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.Rename(leaf, filepath.Join(home, "retained-leaf")); err != nil {
						t.Fatal(err)
					}
					put(t, leaf)
				}
			})
			if r.Status != "review_required" || r.Attempted != 0 {
				t.Fatalf("changed binding reached unlink: %+v", r)
			}
		})
	}
	home, root := ownedTrash(t)
	target := filepath.Join(home, "owned-target")
	put(t, target)
	if err := os.Symlink(target, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	m := mustPreview(t, home)
	if len(m.Entries) != 1 || m.Entries[0].Identity.Type != "symlink" {
		t.Fatal("symlink was traversed")
	}
	if r := applyHome(home, m, true, nil); r.Status != "completed" {
		t.Fatalf("symlink removal: %+v", r)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal("symlink target was affected", err)
	}
}
