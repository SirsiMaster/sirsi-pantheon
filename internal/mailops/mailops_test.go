package mailops

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"google.golang.org/api/gmail/v1"
)

// fakeClient is a Rule A16 test double — no network, no Gmail SDK.
type fakeClient struct {
	inboxIDs     []string
	poisoned     map[string]bool
	headers      map[string]Message
	senderCounts map[string]int
	senderLists  map[string]bool
	archived     []string
	archiveErr   error
}

func (f *fakeClient) ListInboxIDs(query string) ([]string, error) { return f.inboxIDs, nil }

func (f *fakeClient) HasEmptyTextPart(ids []string) ([]string, error) {
	var out []string
	for _, id := range ids {
		if f.poisoned[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

func (f *fakeClient) Headers(ids []string) (map[string]Message, error) {
	out := make(map[string]Message, len(ids))
	for _, id := range ids {
		out[id] = f.headers[id]
	}
	return out, nil
}

func (f *fakeClient) SendersLastYear() (map[string]int, map[string]bool, error) {
	return f.senderCounts, f.senderLists, nil
}

func (f *fakeClient) Archive(ids []string) error {
	if f.archiveErr != nil {
		return f.archiveErr
	}
	f.archived = append(f.archived, ids...)
	return nil
}

func TestPoisonScan_DryRunNeverArchives(t *testing.T) {
	c := &fakeClient{
		inboxIDs: []string{"a", "b", "c"},
		poisoned: map[string]bool{"a": true},
		headers:  map[string]Message{"a": {ID: "a", From: "x@y.com", Subject: "poison"}},
	}
	res, err := PoisonScan(c, false /* apply */, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.ScannedTotal != 3 {
		t.Errorf("ScannedTotal = %d, want 3", res.ScannedTotal)
	}
	if len(res.Poisoned) != 1 || res.Poisoned[0].ID != "a" {
		t.Errorf("Poisoned = %v, want [a]", res.Poisoned)
	}
	if len(c.archived) != 0 {
		t.Errorf("dry-run must never archive, got %v", c.archived)
	}
	if len(res.Archived) != 0 {
		t.Errorf("dry-run result.Archived must be empty, got %v", res.Archived)
	}
}

func TestPoisonScan_ApplyArchivesOnlyPoisoned(t *testing.T) {
	c := &fakeClient{
		inboxIDs: []string{"a", "b"},
		poisoned: map[string]bool{"a": true},
		headers:  map[string]Message{"a": {ID: "a"}},
	}
	res, err := PoisonScan(c, true /* apply */, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.archived, []string{"a"}) {
		t.Errorf("archived = %v, want [a]", c.archived)
	}
	if !reflect.DeepEqual(res.Archived, []string{"a"}) {
		t.Errorf("result.Archived = %v, want [a]", res.Archived)
	}
}

func TestPoisonScan_NoneFoundSkipsArchive(t *testing.T) {
	c := &fakeClient{inboxIDs: []string{"a", "b"}}
	res, err := PoisonScan(c, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Poisoned) != 0 || len(c.archived) != 0 {
		t.Errorf("expected no poisoned/archived, got poisoned=%v archived=%v", res.Poisoned, c.archived)
	}
}

func TestSenderCensus_SortedDescendingAndTruncated(t *testing.T) {
	c := &fakeClient{
		senderCounts: map[string]int{"a@x.com": 5, "b@x.com": 20, "c@x.com": 1},
		senderLists:  map[string]bool{"b@x.com": true},
	}
	out, err := SenderCensus(c, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	if out[0].From != "b@x.com" || out[0].Count != 20 || !out[0].ListUnsubscribe {
		t.Errorf("out[0] = %+v, want b@x.com/20/true", out[0])
	}
	if out[1].From != "a@x.com" || out[1].Count != 5 {
		t.Errorf("out[1] = %+v, want a@x.com/5", out[1])
	}
}

func TestSenderCensus_ZeroNReturnsAll(t *testing.T) {
	c := &fakeClient{senderCounts: map[string]int{"a@x.com": 1, "b@x.com": 2}, senderLists: map[string]bool{}}
	out, err := SenderCensus(c, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Errorf("len(out) = %d, want 2", len(out))
	}
}

func TestEmptyTextPart(t *testing.T) {
	// The Gmail API request/response types live in gmail.go's real client;
	// this exercises the pure recursive predicate directly against them.
	empty := &gmail.MessagePart{MimeType: "multipart/mixed", Parts: []*gmail.MessagePart{
		{MimeType: "text/plain", Body: &gmail.MessagePartBody{Size: 0}},
		{MimeType: "text/html", Body: &gmail.MessagePartBody{Size: 512}},
	}}
	if !emptyTextPart(empty) {
		t.Error("expected empty text/plain part to be detected")
	}
	nonEmpty := &gmail.MessagePart{MimeType: "multipart/mixed", Parts: []*gmail.MessagePart{
		{MimeType: "text/plain", Body: &gmail.MessagePartBody{Size: 40}},
	}}
	if emptyTextPart(nonEmpty) {
		t.Error("expected non-empty text/plain part to pass")
	}
}

func TestLoadAccounts_ExpandsHomeInTokenPath(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/accounts.yaml"
	yaml := "accounts:\n  - label: x\n    email: x@y.com\n    token_path: ~/.sirsi/mailops/token-x.json\n"
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	accounts, err := LoadAccounts(path)
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".sirsi", "mailops", "token-x.json")
	if accounts[0].TokenPath != want {
		t.Errorf("TokenPath = %q, want %q", accounts[0].TokenPath, want)
	}
}
