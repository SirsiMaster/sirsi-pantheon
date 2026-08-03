// Package work is a pull-model work queue between agent threads.
//
// One file per work item lives under <root>/items/, with YAML frontmatter
// recording from/to/status/opened and a free-form instructions/body section.
// Receivers poll their inbox on wake, do the work, and call Close. No daemons,
// no dispatch ledger, no launch agents — the file is the queue.
package work

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Item is one piece of work routed between two agent threads.
type Item struct {
	ID           string
	From         string
	To           string
	Title        string
	Type         string // ADR-024 §5: "proposal" | "review" | "decision" | "" — collapses the old reviews/ + decisions/ channels onto one inbox item
	Status       string // "open" or "closed"
	Opened       string // RFC3339
	Closed       string // RFC3339, empty if open
	Instructions string
	Result       string
	BlockedBy    string // optional work-item dependency id; empty means independently actionable

	// Wake-delivery truth (PR#2 wake-or-declare-unavailable). The supervisor/
	// doctor wake pass records the outcome HERE — in the item itself, not a
	// sidecar — so a stranded item is never silent. Additive frontmatter:
	// wake_status ∈ {pending|wake-attempted|wake-unavailable|armed}.
	WakeStatus      string // "" when the wake pass has never touched this item
	WakeAttemptedAt string // RFC3339, set when an adapter was invoked
	WakeAdapter     string // the adapter that fired (cli-spawn/api-call/launchagent/...)
	WakeError       string // why the item is wake-unavailable, when it is
}

// WakeAnnotation is the wake-pass outcome written onto an item's frontmatter by
// SetWake. Empty fields are REMOVED from the frontmatter (so an armed item drops
// a stale wake_error), making the annotation a full, idempotent replace of the
// wake_* block rather than an accreting append.
type WakeAnnotation struct {
	Status      string
	AttemptedAt string
	Adapter     string
	Error       string
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "untitled"
	}
	if len(s) > 60 {
		// Re-trim after the cut: truncating mid-word can leave a trailing
		// hyphen, and `router send` then prints an id the store does not have —
		// which breaks any consumer that pins the printed id (the conduit race
		// guard did; claude-home, router item 20260730-225729).
		s = strings.Trim(s[:60], "-")
	}
	return s
}

func itemsDir(root string) string { return filepath.Join(root, "items") }

// EnsureRoot creates the items/ directory if missing.
func EnsureRoot(root string) error {
	return os.MkdirAll(itemsDir(root), 0o755)
}

// quoteYAML wraps a value in double quotes and escapes embedded quotes,
// backslashes, and newlines so titles/agent ids containing YAML-sensitive
// characters (colons, leading -, &, *, !, |, etc.) round-trip cleanly.
func quoteYAML(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
	return `"` + r.Replace(v) + `"`
}

// unquoteYAML reverses quoteYAML. Values that aren't double-quoted pass through.
func unquoteYAML(v string) string {
	if len(v) < 2 || v[0] != '"' || v[len(v)-1] != '"' {
		return v
	}
	inner := v[1 : len(v)-1]
	r := strings.NewReplacer(`\"`, `"`, `\n`, "\n", `\r`, "\r", `\\`, `\`)
	return r.Replace(inner)
}

// Send writes a new open item from→to and returns its ID (filename stem).
func Send(root, from, to, title, instructions string) (string, error) {
	return SendTyped(root, from, to, title, "", instructions)
}

// SendTyped is Send with an explicit message type (ADR-024 §5). msgType is one
// of "proposal"/"review"/"decision" (or "" for a plain work item) and is
// written as a `type:` frontmatter field so reviews and decisions live as
// addressed items/ entries instead of separate reviews/ + decisions/ channels.
func SendTyped(root, from, to, title, msgType, instructions string) (string, error) {
	if from == "" || to == "" {
		return "", fmt.Errorf("from and to are required")
	}
	if err := EnsureRoot(root); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	id := fmt.Sprintf("%s-%s-%s-%s", now.Format("20060102-150405"), slugify(from), slugify(to), slugify(title))
	path := filepath.Join(itemsDir(root), id+".md")
	typeLine := ""
	if msgType != "" {
		typeLine = fmt.Sprintf("type: %s\n", quoteYAML(msgType))
	}
	body := fmt.Sprintf(`---
from: %s
to: %s
title: %s
%sstatus: open
opened: %s
---

## Instructions

%s
`, quoteYAML(from), quoteYAML(to), quoteYAML(title), typeLine, now.Format(time.RFC3339), strings.TrimSpace(instructions))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return id, nil
}

// SetBlockedBy replaces an item's optional dependency edge. An empty value
// clears the edge; ledger readers resolve whether the referenced item is done.
func SetBlockedBy(root, id, blockedBy string) error {
	path := filepath.Join(itemsDir(root), id+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return fmt.Errorf("missing frontmatter")
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return fmt.Errorf("unterminated frontmatter")
	}
	fm := content[4 : 4+end]
	rest := content[4+end:]
	lines := strings.Split(fm, "\n")
	filtered := make([]string, 0, len(lines)+1)
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "blocked_by:") {
			continue
		}
		filtered = append(filtered, line)
	}
	if blockedBy = strings.TrimSpace(blockedBy); blockedBy != "" {
		filtered = append(filtered, "blocked_by: "+quoteYAML(blockedBy))
	}
	return os.WriteFile(path, []byte("---\n"+strings.Join(filtered, "\n")+rest), 0o644)
}

// Get loads one item by ID.
func Get(root, id string) (Item, error) {
	path := filepath.Join(itemsDir(root), id+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return Item{}, err
	}
	return parse(id, string(data))
}

// ListInbox returns open items addressed to the given agent, oldest first.
// If agent is empty, returns all open items.
func ListInbox(root, agent string) ([]Item, error) {
	entries, err := os.ReadDir(itemsDir(root))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var items []Item
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".md")
		it, err := Get(root, id)
		if err != nil {
			continue
		}
		if it.Status != "open" {
			continue
		}
		if agent != "" && it.To != agent {
			continue
		}
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

// ListAll returns every item regardless of status.
func ListAll(root string) ([]Item, error) {
	entries, err := os.ReadDir(itemsDir(root))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var items []Item
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".md")
		it, err := Get(root, id)
		if err != nil {
			continue
		}
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

// ErrAlreadyClosed is returned by Close when the item's file is already
// closed. Callers (the dispatch facade) distinguish it from real failures so
// a stale-open store mirror can still be healed.
var ErrAlreadyClosed = errors.New("already closed")

// Close marks an item closed and appends a result section.
func Close(root, id, result string) error {
	it, err := Get(root, id)
	if err != nil {
		return err
	}
	if it.Status == "closed" {
		return ErrAlreadyClosed
	}
	path := filepath.Join(itemsDir(root), id+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	updated := strings.Replace(string(data), "status: open", "status: closed", 1)
	updated = strings.Replace(updated, "opened: "+it.Opened, "opened: "+it.Opened+"\nclosed: "+now, 1)
	if strings.TrimSpace(result) != "" {
		updated += fmt.Sprintf("\n## Result\n\n%s\n", strings.TrimSpace(result))
	} else {
		updated += "\n## Result\n\n(closed without result)\n"
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}

// SetWake upserts the wake_* frontmatter fields on an item, idempotently. Empty
// annotation fields are removed (clearing stale state, e.g. an item that became
// armed drops its prior wake_error). It rewrites only the frontmatter block; the
// instructions/result body is untouched. Safe to call repeatedly — the wake pass
// keys re-invocation idempotency off wake_attempted_at, not off this writer.
func SetWake(root, id string, w WakeAnnotation) error {
	path := filepath.Join(itemsDir(root), id+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return fmt.Errorf("missing frontmatter")
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return fmt.Errorf("unterminated frontmatter")
	}
	fm := content[4 : 4+end]
	rest := content[4+end:] // begins with "\n---\n" + body — preserved verbatim
	lines := strings.Split(fm, "\n")

	set := func(key, val string) {
		filtered := make([]string, 0, len(lines)+1)
		for _, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), key+":") {
				continue
			}
			filtered = append(filtered, l)
		}
		if val != "" {
			filtered = append(filtered, key+": "+quoteYAML(val))
		}
		lines = filtered
	}
	set("wake_status", w.Status)
	set("wake_attempted_at", w.AttemptedAt)
	set("wake_adapter", w.Adapter)
	set("wake_error", w.Error)

	return os.WriteFile(path, []byte("---\n"+strings.Join(lines, "\n")+rest), 0o644)
}

// parse extracts an Item from frontmatter + body text.
func parse(id, content string) (Item, error) {
	it := Item{ID: id}
	if !strings.HasPrefix(content, "---\n") {
		return it, fmt.Errorf("missing frontmatter")
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return it, fmt.Errorf("unterminated frontmatter")
	}
	fm := content[4 : 4+end]
	body := content[4+end+5:]
	for _, line := range strings.Split(fm, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = unquoteYAML(strings.TrimSpace(v))
		switch k {
		case "from":
			it.From = v
		case "to":
			it.To = v
		case "title":
			it.Title = v
		case "type":
			it.Type = v
		case "status":
			it.Status = v
		case "opened":
			it.Opened = v
		case "closed":
			it.Closed = v
		case "blocked_by":
			it.BlockedBy = v
		case "wake_status":
			it.WakeStatus = v
		case "wake_attempted_at":
			it.WakeAttemptedAt = v
		case "wake_adapter":
			it.WakeAdapter = v
		case "wake_error":
			it.WakeError = v
		}
	}
	if instr, rest, ok := strings.Cut(body, "## Instructions"); ok {
		_ = instr
		if rIdx := strings.Index(rest, "\n## Result"); rIdx >= 0 {
			it.Instructions = strings.TrimSpace(rest[:rIdx])
			it.Result = strings.TrimSpace(strings.TrimPrefix(rest[rIdx:], "\n## Result"))
		} else {
			it.Instructions = strings.TrimSpace(rest)
		}
	}
	return it, nil
}
