package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// patchSupervisor rewrites the test registry's service row, `supervisor`, so a
// test can add fields to it (mail_to) without restating the whole registry.
func patchSupervisor(t *testing.T, f *Facade, with string) {
	t.Helper()
	p := filepath.Join(f.root, "agents.json")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	old := `"supervisor": {"id":"supervisor","type":"service"`
	if !strings.Contains(string(b), old) {
		t.Fatal("test registry lost its supervisor service row")
	}
	if err := os.WriteFile(p, []byte(strings.Replace(string(b), old, `"supervisor": {`+with+`"id":"supervisor","type":"service"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A service with mail_to is not a place work lands: a reply addressed to it
// (the exact shape of `router respond` to a card the service sent) is delivered
// to the declared custodian instead, and mail already stranded drains there.
func TestServiceMailToRedirectsAndDrains(t *testing.T) {
	f := testFacade(t)

	// Stranded before the redirect existed: a reply sitting in the service inbox.
	stranded, err := f.Send("b", "supervisor", "old reply to a service card", "decision", "x")
	if err != nil {
		t.Fatal(err)
	}

	patchSupervisor(t, f, `"mail_to":"a",`)

	// New mail addressed to the service lands with the custodian.
	res, err := f.Send("b", "supervisor", "new reply to a service card", "decision", "y")
	if err != nil {
		t.Fatalf("send to a redirected service: %v", err)
	}
	it, err := f.store.Get(res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if it.To != "a" {
		t.Fatalf("reply to the service landed in %q, want custodian a", it.To)
	}

	// The already-stranded item moves with its id and history; nothing is lost.
	out, err := f.DrainAliases("a", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].ID != stranded.ID || out[0].Err != nil {
		t.Fatalf("drain results = %+v, want exactly the stranded item, no error", out)
	}
	moved, err := f.store.Get(stranded.ID)
	if err != nil {
		t.Fatal(err)
	}
	if moved.To != "a" || !strings.Contains(moved.Instructions, "drained from redirected mailbox supervisor → a") {
		t.Fatalf("after drain: to=%q body=%q", moved.To, moved.Instructions)
	}
	if left, _ := f.store.Inbox("supervisor"); len(left) != 0 {
		t.Fatalf("service inbox still holds %d items", len(left))
	}
}

// The committed registry itself must load and send Horus's mail to Ra. This reads
// only the file (nil store), so a typo in agents.json fails CI here instead of
// failing every send on the fleet.
func TestCommittedRegistryRoutesHorusMailToRa(t *testing.T) {
	f := &Facade{root: filepath.Join("..", "..", ".agents", "idea-router")}
	aliases, err := f.Aliases()
	if err != nil {
		t.Fatalf("committed agents.json does not load: %v", err)
	}
	if aliases["horus"] != "ra" {
		t.Fatalf("horus mail_to = %q, want ra", aliases["horus"])
	}
}

// Negative controls: the registry defects mail_to must refuse.
func TestServiceMailToRegistryDefectsRefused(t *testing.T) {
	cases := map[string]string{
		"target not declared": `"mail_to":"nobody",`,
		"points at itself":    `"mail_to":"supervisor",`,
	}
	for name, with := range cases {
		t.Run(name, func(t *testing.T) {
			f := testFacade(t)
			patchSupervisor(t, f, with)
			if _, err := f.Aliases(); err == nil {
				t.Fatalf("%s: Aliases accepted a bad mail_to", name)
			}
		})
	}

	t.Run("mail_to on a non-service", func(t *testing.T) {
		f := testFacade(t)
		p := filepath.Join(f.root, "agents.json")
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		// Agent b is a lane in the test registry; a lane must not redirect its inbox.
		s := strings.Replace(string(b), `"b": {`, `"b": {"mail_to":"a",`, 1)
		if s == string(b) {
			t.Skip("test registry has no `b` lane row to patch")
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := f.Aliases(); err == nil {
			t.Fatal("a lane's mail_to must be refused: only a service may redirect")
		}
	})
}
