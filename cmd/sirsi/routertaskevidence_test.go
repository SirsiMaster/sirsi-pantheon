//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"golang.org/x/sys/unix"
)

func taskEvidenceCLIFixture(t *testing.T) (string, string, routerstore.TaskEvidenceManifest) {
	t.Helper()
	// Physical parents are required by the real nofollow reader, not bypassed
	// for tests (/var on macOS can otherwise introduce a parent symlink).
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_TOKEN", "")
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(root, "must-not-create.db"))
	target := filepath.Join(root, "preserved.json")
	body := []byte(`{"owned":"fixture evidence"}`)
	if err := os.WriteFile(target, body, 0600); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(body)
	digest := hex.EncodeToString(h[:])
	old := filepath.Join(root, "removed.json") // Intentionally absent; never restored.
	mapping := map[string]any{"schema": "pantheon.checkout-preserved-namespace.v1", "rows": []map[string]string{{"old_absolute": old, "preserved_absolute": target, "sha256": digest, "type": "regular"}}}
	b, err := json.Marshal(mapping)
	if err != nil {
		t.Fatal(err)
	}
	mapPath := filepath.Join(root, "map.json")
	if err := os.WriteFile(mapPath, b, 0600); err != nil {
		t.Fatal(err)
	}
	mh := sha256.Sum256(b)
	m := routerstore.TaskEvidenceManifest{Version: 1, ID: "owned-fixture", Agent: "fixture-lane", AttestationRef: "review://external-process", PreservationMapSHA256: hex.EncodeToString(mh[:]), Tasks: []routerstore.TaskEvidenceRow{{TaskID: "owned-task", Changes: []routerstore.TaskEvidenceChange{{Field: "link", Kind: "evidence", Label: "receipt", OldURI: "file://" + old, NewURI: target, SHA256: digest}}}}}
	manifest := filepath.Join(root, "manifest.json")
	writeTaskEvidenceManifest(t, manifest, m)
	return manifest, mapPath, m
}

func writeTaskEvidenceManifest(t *testing.T, path string, m routerstore.TaskEvidenceManifest) {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestTaskEvidenceCLIPlanOfflineAndExplicitAuthority(t *testing.T) {
	manifest, mapping, m := taskEvidenceCLIFixture(t)
	calls := 0
	call := func(_ context.Context, r routerstore.TaskEvidenceRequest) (routerstore.TaskEvidenceResult, error) {
		calls++
		if r.Action != "inspect" {
			t.Fatal("unexpected action", r.Action)
		}
		return routerstore.TaskEvidenceResult{Rows: []routerstore.TaskEvidenceFingerprint{{TaskID: "owned-task", Before: strings.Repeat("a", 64)}}}, nil
	}
	cmd := newTaskEvidenceRelocationCmd(call)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--manifest", manifest, "--preservation-map", mapping})
	if err := cmd.Execute(); err != nil || calls != 0 || !strings.Contains(out.String(), `"action":"local-plan"`) {
		t.Fatalf("offline plan: %s calls=%d err=%v", out.String(), calls, err)
	}
	if _, err := os.Stat(os.Getenv("SIRSI_ROUTER_DB")); !os.IsNotExist(err) {
		t.Fatal("plan touched router DB", err)
	}
	old, _ := routerstore.TaskEvidenceLocalPath(m.Tasks[0].Changes[0].OldURI)
	if _, err := os.Lstat(old); !os.IsNotExist(err) {
		t.Fatal("plan recreated old artifact", err)
	}
	cmd = newTaskEvidenceRelocationCmd(call)
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--manifest", manifest, "--preservation-map", mapping, "--inspect"})
	if err := cmd.Execute(); err != nil || calls != 1 {
		t.Fatal("explicit inspection did not reach injected authority", err, calls)
	}
	// Apply cannot invent a fingerprint, nor treat successful local hashing as
	// proof that the current canonical row still matches.
	cmd = newTaskEvidenceRelocationCmd(call)
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--manifest", manifest, "--preservation-map", mapping, "--apply"})
	if err := cmd.Execute(); err == nil || calls != 1 {
		t.Fatal("unpinned apply reached authority")
	}
	m.Tasks[0].ExpectedSHA256 = strings.Repeat("a", 64)
	writeTaskEvidenceManifest(t, manifest, m)
	sentinel := errors.New("fixture authority unreachable")
	cmd = newTaskEvidenceRelocationCmd(func(_ context.Context, r routerstore.TaskEvidenceRequest) (routerstore.TaskEvidenceResult, error) {
		calls++
		if r.Action != "apply" || r.Manifest.Tasks[0].ExpectedSHA256 != m.Tasks[0].ExpectedSHA256 {
			t.Fatal("apply changed exact manifest")
		}
		return routerstore.TaskEvidenceResult{}, sentinel
	})
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--manifest", manifest, "--preservation-map", mapping, "--apply"})
	if err := cmd.Execute(); !errors.Is(err, sentinel) || calls != 2 {
		t.Fatal("authority failure misreported", err, calls)
	}
}

func TestTaskEvidenceCLIRefusesInvalidInputsBeforeAuthority(t *testing.T) {
	for _, mode := range []string{"args", "both-actions", "relative", "symlink-manifest", "symlink-parent", "symlink-map", "symlink-target", "fifo", "artifact-hash", "map-hash", "duplicate-map-row", "unknown-manifest", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			manifest, mapping, m := taskEvidenceCLIFixture(t)
			root := filepath.Dir(manifest)
			args := []string{"--manifest", manifest, "--preservation-map", mapping, "--inspect"}
			switch mode {
			case "args":
				args = append(args, "ignored")
			case "both-actions":
				args = append(args, "--apply")
			case "relative":
				args[1] = "manifest.json"
			case "symlink-manifest", "symlink-map", "symlink-target":
				path := manifest
				if mode == "symlink-map" {
					path = mapping
				}
				if mode == "symlink-target" {
					path = m.Tasks[0].Changes[0].NewURI
				}
				backup := path + ".real"
				if err := os.Rename(path, backup); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(backup, path); err != nil {
					t.Fatal(err)
				}
			case "symlink-parent":
				link := filepath.Join(root, "alias")
				if err := os.Symlink(root, link); err != nil {
					t.Fatal(err)
				}
				args[1] = filepath.Join(link, "manifest.json")
			case "fifo":
				fifo := filepath.Join(root, "fifo")
				if err := unix.Mkfifo(fifo, 0600); err != nil {
					t.Fatal(err)
				}
				args[1] = fifo
			case "artifact-hash":
				if err := os.WriteFile(m.Tasks[0].Changes[0].NewURI, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "map-hash":
				if err := os.WriteFile(mapping, []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
			case "duplicate-map-row":
				b, err := os.ReadFile(mapping)
				if err != nil {
					t.Fatal(err)
				}
				var obj map[string]json.RawMessage
				if err := json.Unmarshal(b, &obj); err != nil {
					t.Fatal(err)
				}
				var rows []json.RawMessage
				if err := json.Unmarshal(obj["rows"], &rows); err != nil {
					t.Fatal(err)
				}
				rows = append(rows, rows[0])
				obj["rows"], err = json.Marshal(rows)
				if err != nil {
					t.Fatal(err)
				}
				b, err = json.Marshal(obj)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(mapping, b, 0600); err != nil {
					t.Fatal(err)
				}
				h := sha256.Sum256(b)
				m.PreservationMapSHA256 = hex.EncodeToString(h[:])
				writeTaskEvidenceManifest(t, manifest, m)
			case "unknown-manifest":
				b, err := os.ReadFile(manifest)
				if err != nil {
					t.Fatal(err)
				}
				b = bytes.Replace(b, []byte(`"version":1`), []byte(`"version":1,"force":true`), 1)
				if err := os.WriteFile(manifest, b, 0600); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				if err := os.WriteFile(manifest, []byte(strings.Repeat(" ", routerstore.TaskEvidenceManifestLimit+1)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			cmd := newTaskEvidenceRelocationCmd(func(context.Context, routerstore.TaskEvidenceRequest) (routerstore.TaskEvidenceResult, error) {
				calls++
				return routerstore.TaskEvidenceResult{}, nil
			})
			cmd.SilenceUsage = true
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(args)
			if err := cmd.Execute(); err == nil || calls != 0 {
				t.Fatalf("%s accepted/contacted authority: calls=%d err=%v", mode, calls, err)
			}
		})
	}
}

func TestTaskEvidenceCLIRejectsLocalDBWithoutCreatingIt(t *testing.T) {
	taskEvidenceCLIFixture(t)
	if _, err := callTaskEvidenceAuthority(context.Background(), routerstore.TaskEvidenceRequest{}); err == nil {
		t.Fatal("explicit local DB accepted as remote authority")
	}
	if _, err := os.Stat(os.Getenv("SIRSI_ROUTER_DB")); !os.IsNotExist(err) {
		t.Fatal("refusal created local DB", err)
	}
}

func TestTaskEvidencePreservationMapRejectsDuplicateAndAliasKeys(t *testing.T) {
	for _, keys := range [][2]string{{"schema", "Schema"}, {"rows", "Rows"}, {"old_absolute", "Old_Absolute"}, {"preserved_absolute", "Preserved_Absolute"}, {"sha256", "SHA256"}, {"type", "Type"}} {
		for _, mode := range []string{"alias-only", "alias-and-exact", "duplicate-exact"} {
			t.Run(keys[0]+"/"+mode, func(t *testing.T) {
				manifest, mapping, m := taskEvidenceCLIFixture(t)
				b, err := os.ReadFile(mapping)
				if err != nil {
					t.Fatal(err)
				}
				exact := `"` + keys[0] + `":`
				alias := `"` + keys[1] + `":`
				replacement := alias
				if mode == "alias-and-exact" {
					replacement = alias + `null,` + exact
				}
				if mode == "duplicate-exact" {
					replacement = exact + `null,` + exact
				}
				b = bytes.Replace(b, []byte(exact), []byte(replacement), 1)
				if err := os.WriteFile(mapping, b, 0600); err != nil {
					t.Fatal(err)
				}
				sum := sha256.Sum256(b)
				m.PreservationMapSHA256 = hex.EncodeToString(sum[:])
				writeTaskEvidenceManifest(t, manifest, m)
				// Rebind the exact map hash so refusal proves JSON ambiguity, not
				// merely a digest mismatch or a network/service failure.
				if _, err := loadTaskEvidencePlan(manifest, mapping); err == nil {
					t.Fatal("ambiguous preservation map accepted")
				}
			})
		}
	}
	for _, metadata := range []string{`{"note":1,"note":2}`, `{"nested":[{"value":1,"value":2}]}`} {
		if err := validateTaskEvidenceMapJSON([]byte(`{"schema":"pantheon.checkout-preserved-namespace.v1","rows":[],"metadata":` + metadata + `}`)); err == nil {
			t.Fatal("duplicate ancillary object key accepted")
		}
	}
}

func TestTaskEvidencePreservationMapRetainsAncillaryMetadata(t *testing.T) {
	manifest, mapping, m := taskEvidenceCLIFixture(t)
	b, err := os.ReadFile(mapping)
	if err != nil {
		t.Fatal(err)
	}
	// Ancillary fields are opaque metadata, not aliases for decoded root/row
	// controls. In particular a nested Rows note must not overwrite root rows.
	b = append([]byte(`{"Metadata":{"Rows":"opaque note","nested":[{"MixedCase":true}]},`), b[1:]...)
	if err := os.WriteFile(mapping, b, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	m.PreservationMapSHA256 = hex.EncodeToString(sum[:])
	writeTaskEvidenceManifest(t, manifest, m)
	if _, err := loadTaskEvidencePlan(manifest, mapping); err != nil {
		t.Fatal("allowed ancillary metadata refused", err)
	}
}
