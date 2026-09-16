package stacklab

import (
	"fmt"
	"testing"
)

// mockReader is an in-memory RemoteReader — no network in tests, per repo A35
// scope (the real check reads origin only) and per the task's own
// requirement that remote reads sit behind an injectable interface.
type mockReader struct {
	// files[repo][ref][path] = content
	files map[string]map[string]map[string][]byte
	// branches[repo] = branch names (main need not be listed)
	branches map[string][]string
	// dirs[repo][ref][dirPath] = file names
	dirs map[string]map[string]map[string][]string

	readErr map[string]error // key "repo/ref/path" -> forced error
}

func newMockReader() *mockReader {
	return &mockReader{
		files:    map[string]map[string]map[string][]byte{},
		branches: map[string][]string{},
		dirs:     map[string]map[string]map[string][]string{},
		readErr:  map[string]error{},
	}
}

func (m *mockReader) putFile(repo, ref, path string, content []byte) {
	if m.files[repo] == nil {
		m.files[repo] = map[string]map[string][]byte{}
	}
	if m.files[repo][ref] == nil {
		m.files[repo][ref] = map[string][]byte{}
	}
	m.files[repo][ref][path] = content
}

func (m *mockReader) putDir(repo, ref, dir string, names []string) {
	if m.dirs[repo] == nil {
		m.dirs[repo] = map[string]map[string][]string{}
	}
	if m.dirs[repo][ref] == nil {
		m.dirs[repo][ref] = map[string][]string{}
	}
	m.dirs[repo][ref][dir] = names
}

func (m *mockReader) ReadFile(repo, path, ref string) ([]byte, bool, error) {
	if err, ok := m.readErr[repo+"/"+ref+"/"+path]; ok {
		return nil, false, err
	}
	byRef, ok := m.files[repo]
	if !ok {
		return nil, false, nil
	}
	byPath, ok := byRef[ref]
	if !ok {
		return nil, false, nil
	}
	c, ok := byPath[path]
	if !ok {
		return nil, false, nil
	}
	return c, true, nil
}

func (m *mockReader) ListDir(repo, path, ref string) ([]string, bool, error) {
	byRef, ok := m.dirs[repo]
	if !ok {
		return nil, false, nil
	}
	byPath, ok := byRef[ref]
	if !ok {
		return nil, false, nil
	}
	names, ok := byPath[path]
	if !ok {
		return nil, false, nil
	}
	return names, true, nil
}

func (m *mockReader) ListBranches(repo string) ([]string, error) {
	return append([]string{"main"}, m.branches[repo]...), nil
}

// validWingJSON builds a schema-valid wing record for the given lane, ready
// for json.Marshal-free embedding in test fixtures.
func validWingJSON(id string) []byte {
	return []byte(fmt.Sprintf(`{
  "schema": "sirsi.stacklab.wing.v1",
  "id": %q,
  "owner": "test",
  "project_id": "test-proj",
  "router_namespace": "router",
  "class": "engine",
  "scope": "test scope",
  "status": "active",
  "first_gate": "TEST-001.G1: gate",
  "workspace": {
    "repository_root": "/x/y",
    "writable_roots": ["/x/y"],
    "evidence_root": "/x/y/evidence",
    "shared_payload_access": "none",
    "boundary_policy": "default-deny"
  },
  "handoffs": {
    "inbound": "router-receipt-only",
    "outbound": "router-receipt-only",
    "allowed_peer_wings": []
  },
  "provenance": {
    "lifecycle_task_id": "task-1",
    "component_catalog": "catalog@sha",
    "receipt_links": []
  },
  "mirrors": {
    "repository": "current",
    "desktop": "pending",
    "workspace": "pending"
  },
  "next_action": "do the thing"
}`, id))
}

const testLane = "io-connect"
const testWingID = "stacklab.wing." + testLane
const testRepo = "SirsiMaster/sirsi-io-connect"

func testLaneRepoMap() map[string]string {
	return map[string]string{testLane: testRepo}
}

func TestRun_Clean(t *testing.T) {
	r := newMockReader()
	content := validWingJSON(testWingID)
	r.putFile(testRepo, "main", wingPath(testLane), content)
	// A registry pin is a byte-for-byte copy of the origin record.
	r.putFile(RegistryRepo, "main", "wings/pinned/"+testLane+"-wing-v1.json", content)
	r.putDir(RegistryRepo, "main", "wings/pinned", []string{testLane + "-wing-v1.json"})

	rep := Run(r, []string{testWingID}, testLaneRepoMap())
	if !rep.Clean() {
		t.Fatalf("expected clean report, got findings=%v unknown=%v", rep.Findings, rep.Unknown)
	}
}

func TestRun_StrandedUnbuilt(t *testing.T) {
	r := newMockReader()
	// no file anywhere, no branches — declared peer, nothing built.
	r.putDir(RegistryRepo, "main", "wings/pinned", nil)

	rep := Run(r, []string{testWingID}, testLaneRepoMap())
	want := Finding{testWingID, KindStrandedUnbuilt, ""}
	if len(rep.Findings) != 1 || rep.Findings[0].WingID != want.WingID || rep.Findings[0].Kind != want.Kind {
		t.Fatalf("expected exactly one stranded/unbuilt finding, got %+v", rep.Findings)
	}
}

func TestRun_UnpushedStranded(t *testing.T) {
	r := newMockReader()
	r.branches[testRepo] = []string{"feature/publish-wing"}
	content := validWingJSON(testWingID)
	r.putFile(testRepo, "feature/publish-wing", wingPath(testLane), content)
	// deliberately absent on main.
	r.putDir(RegistryRepo, "main", "wings/pinned", nil)

	rep := Run(r, []string{testWingID}, testLaneRepoMap())
	if len(rep.Findings) != 1 || rep.Findings[0].Kind != KindUnpushedStranded {
		t.Fatalf("expected one unpushed/stranded finding, got %+v", rep.Findings)
	}
}

func TestRun_Unpinned_NoPin(t *testing.T) {
	r := newMockReader()
	content := validWingJSON(testWingID)
	r.putFile(testRepo, "main", wingPath(testLane), content)
	r.putDir(RegistryRepo, "main", "wings/pinned", nil) // no pin file for this lane

	rep := Run(r, []string{testWingID}, testLaneRepoMap())
	if len(rep.Findings) != 1 || rep.Findings[0].Kind != KindUnpinned {
		t.Fatalf("expected one unpinned finding, got %+v", rep.Findings)
	}
}

func TestRun_Unpinned_HashMismatch(t *testing.T) {
	r := newMockReader()
	content := validWingJSON(testWingID)
	r.putFile(testRepo, "main", wingPath(testLane), content)
	// Pin has the right id but stale/edited content — hash mismatch.
	stalePin := validWingJSON(testWingID)
	stalePin = append(stalePin[:len(stalePin)-1], []byte(`,"_stale":true}`)...)
	r.putFile(RegistryRepo, "main", "wings/pinned/"+testLane+"-wing-v1.json", stalePin)
	r.putDir(RegistryRepo, "main", "wings/pinned", []string{testLane + "-wing-v1.json"})

	rep := Run(r, []string{testWingID}, testLaneRepoMap())
	if len(rep.Findings) != 1 || rep.Findings[0].Kind != KindUnpinned {
		t.Fatalf("expected one unpinned finding for hash mismatch, got %+v", rep.Findings)
	}
}

func TestRun_Invalid(t *testing.T) {
	r := newMockReader()
	r.putFile(testRepo, "main", wingPath(testLane), []byte(`{"schema": "sirsi.stacklab.wing.v1", "id": "bad id with spaces"}`))
	r.putDir(RegistryRepo, "main", "wings/pinned", nil)

	rep := Run(r, []string{testWingID}, testLaneRepoMap())
	if len(rep.Findings) != 1 || rep.Findings[0].Kind != KindInvalid {
		t.Fatalf("expected one invalid finding, got %+v", rep.Findings)
	}
}

func TestRun_Undeclared(t *testing.T) {
	r := newMockReader()
	// registry has a pin for a lane never declared in the roster — filename
	// deliberately does NOT match the lane, to exercise content-based (id
	// field) matching rather than filename-derived matching.
	r.putFile(RegistryRepo, "main", "wings/pinned/oddly-named-file.json", validWingJSON("stacklab.wing.rogue-lane"))
	r.putDir(RegistryRepo, "main", "wings/pinned", []string{"oddly-named-file.json"})

	rep := Run(r, nil, map[string]string{}) // empty roster
	found := false
	for _, f := range rep.Findings {
		if f.Kind == KindUndeclared && f.WingID == "stacklab.wing.rogue-lane" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an undeclared finding for stacklab.wing.rogue-lane, got %+v", rep.Findings)
	}
}

func TestRun_UnknownMapping_IsStrandedUnbuilt(t *testing.T) {
	r := newMockReader()
	r.putDir(RegistryRepo, "main", "wings/pinned", nil)

	rep := Run(r, []string{"stacklab.wing.hardware-estate"}, map[string]string{}) // no mapping known
	if len(rep.Findings) != 1 || rep.Findings[0].Kind != KindStrandedUnbuilt {
		t.Fatalf("expected stranded/unbuilt for unmapped lane, got %+v", rep.Findings)
	}
}

func TestRun_ReadErrorIsUnknownNotClean(t *testing.T) {
	r := newMockReader()
	r.readErr[testRepo+"/main/"+wingPath(testLane)] = fmt.Errorf("gh: auth error")
	r.putDir(RegistryRepo, "main", "wings/pinned", nil)

	rep := Run(r, []string{testWingID}, testLaneRepoMap())
	if rep.Clean() {
		t.Fatalf("a read error must never be reported as clean")
	}
	if len(rep.Unknown) != 1 {
		t.Fatalf("expected exactly one unknown entry, got %v", rep.Unknown)
	}
}

func TestValidateWing_RejectsUnknownFields(t *testing.T) {
	valid := validWingJSON(testWingID)
	// Inject an unknown top-level field by string surgery — additionalProperties:false.
	bad := append(valid[:len(valid)-1], []byte(`,"unexpected_field":"x"}`)...)
	if _, err := ValidateWing(bad); err == nil {
		t.Fatalf("expected an error for an unknown field")
	}
}

func TestContentSHA256_Deterministic(t *testing.T) {
	a := ContentSHA256([]byte("hello"))
	b := ContentSHA256([]byte("hello"))
	if a != b {
		t.Fatalf("hash must be deterministic: %s != %s", a, b)
	}
	if a == ContentSHA256([]byte("world")) {
		t.Fatalf("different content must hash differently")
	}
}
