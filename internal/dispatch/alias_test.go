package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// aliasFacade is testFacade with a retired alias "old-b" → "b" (ADR-072 C5).
func aliasFacade(t *testing.T) *Facade {
	t.Helper()
	f := testFacade(t)
	p := filepath.Join(f.root, "agents.json")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.TrimSpace(string(data))
	s = strings.TrimSuffix(s, "}") + `, "aliases": {"old-b": "b"}}`
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestSendToAliasDeliversToSuccessor(t *testing.T) {
	f := aliasFacade(t)
	res, err := f.Send("a", "old-b", "for the retired name", "", "x")
	if err != nil {
		t.Fatalf("send to alias: %v", err)
	}
	it, err := f.store.Get(res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if it.To != "b" {
		t.Fatalf("alias send landed in %q, want successor b", it.To)
	}
	// Negative control: an unknown, undeclared name is still refused, not aliased.
	if _, err := f.Send("a", "nobody", "t", "", "x"); err == nil {
		t.Fatal("send to an undeclared non-alias must be refused")
	}
}

func TestReassignRules(t *testing.T) {
	f := aliasFacade(t)
	res, err := f.Send("a", "b", "hand me off", "", "x")
	if err != nil {
		t.Fatal(err)
	}
	// A third party may not move someone else's mail.
	if err = f.Reassign("claude-home", res.ID, "a"); err == nil {
		t.Fatal("non-recipient reassign must be refused")
	}
	// The recipient may hand off; id is kept and the note says who moved it.
	if err = f.Reassign("b", res.ID, "a"); err != nil {
		t.Fatalf("recipient hand-off: %v", err)
	}
	it, err := f.store.Get(res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if it.To != "a" || !strings.Contains(it.Instructions, "handed off b → a by b") {
		t.Fatalf("after hand-off: to=%q body=%q", it.To, it.Instructions)
	}
	// Reassigning to an undeclared agent is refused.
	if err := f.Reassign("a", res.ID, "nobody"); err == nil {
		t.Fatal("reassign to undeclared agent must be refused")
	}
}

func TestDrainAliasesMovesOnlyUnclaimedAliasMail(t *testing.T) {
	f := aliasFacade(t)
	// Items that reached the retired name before the alias existed.
	var ids []string
	for _, title := range []string{"one", "two"} {
		id, err := f.store.Send("a", "old-b", title, "", "x")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	claimed, err := f.store.Send("a", "old-b", "claimed", "", "x")
	if err != nil {
		t.Fatal(err)
	}
	// ClaimNext takes the oldest open item; make "claimed" the only candidate.
	for _, id := range ids {
		if err = f.store.CloseItem(id, "temp"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = f.store.ClaimNext("old-b", time.Minute); err != nil {
		t.Fatal(err)
	}
	fresh, err := f.store.Send("a", "old-b", "fresh", "", "x")
	if err != nil {
		t.Fatal(err)
	}

	dry, err := f.DrainAliases("a", true)
	if err != nil {
		t.Fatal(err)
	}
	if it, _ := f.store.Get(fresh); it.To != "old-b" {
		t.Fatalf("dry run moved an item: to=%q", it.To)
	}
	if len(dry) == 0 {
		t.Fatal("dry run reported nothing")
	}

	res, err := f.DrainAliases("a", false)
	for _, r := range res {
		if r.ID == claimed && r.Err == nil {
			t.Fatal("claimed item reported as moved")
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	if it, _ := f.store.Get(fresh); it.To != "b" {
		t.Fatalf("drain did not move open alias mail: to=%q", it.To)
	}
	if it, _ := f.store.Get(claimed); it.To != "old-b" {
		t.Fatalf("drain moved a CLAIMED item: to=%q", it.To)
	}
}

func TestAliasesRefusesBadRegistry(t *testing.T) {
	for name, aliases := range map[string]string{
		"alias is a declared agent": `{"a": "b"}`,
		"successor undeclared":      `{"old-x": "nobody"}`,
	} {
		f := testFacade(t)
		p := filepath.Join(f.root, "agents.json")
		data, _ := os.ReadFile(p)
		s := strings.TrimSuffix(strings.TrimSpace(string(data)), "}") + `, "aliases": ` + aliases + `}`
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := f.Aliases(); err == nil {
			t.Errorf("%s: Aliases() = nil error, want refusal", name)
		}
		// A broken alias map must stop sends rather than misroute them.
		if _, err := f.Send("a", "b", "t", "", "x"); err == nil {
			t.Errorf("%s: send must fail closed on a broken alias map", name)
		}
	}
}
