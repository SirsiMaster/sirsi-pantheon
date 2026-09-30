package dispatch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Aliases returns agents.json's retired-name map (ADR-072 C5): a retired
// mailbox name → the declared agent that now owns its mail. A missing or empty
// "aliases" key is an empty map. An alias that is itself a declared agent, or
// whose successor is not a declared agent, is a registry defect and is refused
// here so no caller can route through it.
func (f *Facade) Aliases() (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(f.root, "agents.json"))
	if err != nil {
		return nil, fmt.Errorf("dispatch: read agents.json: %w", err)
	}
	var reg struct {
		Agents  map[string]json.RawMessage `json:"agents"`
		Aliases map[string]string          `json:"aliases"`
	}
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("dispatch: parse agents.json: %w", err)
	}
	out := make(map[string]string, len(reg.Aliases))
	for old, succ := range reg.Aliases {
		old, succ = strings.TrimSpace(old), strings.TrimSpace(succ)
		if _, declared := reg.Agents[old]; declared {
			return nil, fmt.Errorf("dispatch: alias %q is also a declared agent — retire it from \"agents\" first", old)
		}
		if err := f.ValidateAgent("alias successor", succ); err != nil {
			return nil, fmt.Errorf("dispatch: alias %q → %q: %w", old, succ, err)
		}
		out[old] = succ
	}
	return out, nil
}

// resolveRecipient maps a retired name to its successor (one hop; successors are
// declared agents, so there is no chain). Any other name is returned unchanged.
func (f *Facade) resolveRecipient(to string) (string, error) {
	aliases, err := f.Aliases()
	if err != nil {
		return "", err
	}
	if succ, ok := aliases[strings.TrimSpace(to)]; ok {
		return succ, nil
	}
	return to, nil
}

// Reassign moves an open, unclaimed item to another declared agent, keeping its
// id and history. Two cases are allowed, and nothing else:
//   - a hand-off: actor is the item's current recipient;
//   - a drain: the current recipient is a retired alias and to is its declared
//     successor — no discretion, so any declared actor may run it.
//
// Everything else is refused: reassigning someone else's mail is not a Ra power.
func (f *Facade) Reassign(actor, id, to string) error {
	if err := f.ValidateAgent("acting agent", actor); err != nil {
		return err
	}
	if err := f.ValidateAgent("recipient", to); err != nil {
		return err
	}
	it, err := f.store.Get(id)
	if err != nil {
		return err
	}
	aliases, err := f.Aliases()
	if err != nil {
		return err
	}
	kind := ""
	switch {
	case it.To == actor:
		kind = "handed off"
	case aliases[it.To] == to:
		kind = "drained from retired alias"
	default:
		return fmt.Errorf("dispatch: %s may not reassign %s (addressed to %s): only the recipient may hand off, and alias mail drains only to its declared successor", actor, id, it.To)
	}
	note := fmt.Sprintf("\n\n---\n_Router: %s %s → %s by %s at %s (ADR-072 C5; same item id, reply as usual)._\n",
		kind, it.To, to, actor, time.Now().UTC().Format(time.RFC3339))
	if err := f.store.ReassignItem(id, it.To, to, note); err != nil {
		return err
	}
	f.store.NotifyAgent(to)
	return nil
}

// DrainResult is one alias item's outcome in DrainAliases.
type DrainResult struct {
	ID, From, To string
	Err          error
}

// DrainAliases moves every open item addressed to a retired alias to its
// declared successor. dryRun reports what would move without moving it. A
// claimed item (leased by someone) is reported with its error and skipped.
func (f *Facade) DrainAliases(actor string, dryRun bool) ([]DrainResult, error) {
	if err := f.ValidateAgent("acting agent", actor); err != nil {
		return nil, err
	}
	aliases, err := f.Aliases()
	if err != nil {
		return nil, err
	}
	var out []DrainResult
	for old, succ := range aliases {
		items, lerr := f.store.Inbox(old)
		if lerr != nil {
			return out, fmt.Errorf("dispatch: list %s: %w", old, lerr)
		}
		for _, it := range items {
			r := DrainResult{ID: it.ID, From: old, To: succ}
			if !dryRun {
				r.Err = f.Reassign(actor, it.ID, succ)
			}
			out = append(out, r)
		}
	}
	return out, nil
}
