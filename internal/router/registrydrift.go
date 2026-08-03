package router

// Registry drift detection — a check that FAILS, not a fix that copies.
//
// WHY THIS EXISTS
//   The router reads `.agents/idea-router/agents.json` from the WORKING TREE, so
//   the working tree IS the live registry and origin/main is the facade. A
//   registry fix that merges to main does not reach the router until the
//   checked-out branch carries it.
//
//   That landmine has now armed three times in six days: 57f027eb defused it by
//   committing the good registry onto the branch, #346 re-armed it by landing a
//   fix on main that never propagated, 865dbf88 defused it again by copying, and
//   on 2026-07-31 the working tree had diverged once more — claude-io's
//   launch_agent_label was present upstream and gone live.
//
//   Every remedy so far has been a COPY. A copy is the artifact that drifts.
//   Three copies in six days is the tell that the remedy is wrong, not that
//   people are careless. So this does not copy anything: it makes the drift
//   announce itself, which is the only version that cannot silently re-arm.
//
// WHAT IT DOES NOT DO
//   It does not look for a wake block, a launch_agent_label, or any other field
//   that has broken before. Enforcement must not share the bug's shape: a check
//   that enumerates the fields we already know about is blind to the next one.
//   It walks both documents generically and reports every leaf the live registry
//   is missing, whatever it is.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// RegistryPath is the registry relative to the repo root — the one file the
// router actually reads.
const RegistryPath = ".agents/idea-router/agents.json"

// LostField is one leaf that exists in the merged registry and not in the live one.
type LostField struct {
	AgentID  string `json:"agent_id"`
	Path     string `json:"path"`     // dotted path within the agent entry, e.g. "wake.launch_agent_label"
	Upstream string `json:"upstream"` // the value the live registry is missing
}

// ChangedField is one leaf present in both with differing values.
type ChangedField struct {
	AgentID  string `json:"agent_id"`
	Path     string `json:"path"`
	Upstream string `json:"upstream"`
	Live     string `json:"live"`
}

// RegistryDrift is the verdict.
//
// Checked=false is NOT "no drift" — it is "unknown", and callers must render it
// that way (IO7a, sirsi-io ADR-002): a surface with no representation for "I
// could not check" collapses every unreadable source into a clean bill of health.
type RegistryDrift struct {
	Checked bool   `json:"checked"`
	Unknown string `json:"unknown_reason,omitempty"` // why the comparison could not be made

	MissingAgents []string       `json:"missing_agents,omitempty"` // merged, absent from live — the router cannot address these
	LostFields    []LostField    `json:"lost_fields,omitempty"`    // merged value the live registry dropped
	ChangedFields []ChangedField `json:"changed_fields,omitempty"` // differ between merged and live
	AheadAgents   []string       `json:"ahead_agents,omitempty"`   // live-only: unpushed work, informational
}

// Drifted reports whether the live registry is missing something the merged one
// has. Being AHEAD is not drift — unpushed registrations are normal and are
// reported separately so they are visible without being alarming.
func (d RegistryDrift) Drifted() bool {
	return len(d.MissingAgents) > 0 || len(d.LostFields) > 0 || len(d.ChangedFields) > 0
}

// CheckRegistryDrift compares the live registry against origin/main.
func CheckRegistryDrift(repoRoot string) RegistryDrift {
	live, err := readLiveRegistry(repoRoot)
	if err != nil {
		return RegistryDrift{Unknown: fmt.Sprintf("live registry unreadable: %v", err)}
	}
	merged, err := readMergedRegistry(repoRoot)
	if err != nil {
		// Unknown, deliberately — not "clean". A detached checkout, an unfetched
		// remote or a missing git are all reasons we cannot know, and none of
		// them is evidence that the registry is fine.
		return RegistryDrift{Unknown: err.Error()}
	}
	return diffRegistries(merged, live)
}

// readLiveRegistry reads the FILE ON DISK, and nothing else.
//
// An earlier cut of this read the git index (`git show :path`) on the theory
// that it is "what a commit would carry". That was wrong, and wrong in the exact
// way this check exists to catch: the router reads the working file, so the
// working file is the live registry. On the machine this was written against,
// the index and the working file differed by 85 insertions — so an index-based
// check would have reported on an artifact nobody runs.
//
// Verify the artifact, not a nearby one that is easier to read.
func readLiveRegistry(repoRoot string) ([]byte, error) {
	return os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(RegistryPath)))
}

func readMergedRegistry(repoRoot string) ([]byte, error) {
	out, err := exec.Command("git", "-C", repoRoot, "show", "origin/main:"+RegistryPath).Output()
	if err != nil {
		return nil, fmt.Errorf("cannot read origin/main:%s (unfetched remote, or not a git checkout): %v", RegistryPath, err)
	}
	return out, nil
}

// agentsByID parses either registry shape — a bare list, or an object with an
// "agents" key — into id → entry. Tolerating both matters because a check that
// only understands today's shape fails silently the day the shape changes.
func agentsByID(raw []byte) (map[string]map[string]any, error) {
	var anyDoc any
	if err := json.Unmarshal(raw, &anyDoc); err != nil {
		return nil, err
	}
	out := map[string]map[string]any{}

	// The SHAPE decides which key is authoritative, because that is what
	// LoadRegistry does — and this check is worthless unless it indexes agents
	// exactly as the loader addresses them.
	//
	//   map-shaped : `for id, cfg := range reg.Agents { cfg.ID = id }` —
	//                UNCONDITIONAL. The map key always wins, even when an inner
	//                "id" disagrees with it.
	//   list-shaped: the inner "id" is the only key there is.
	//
	// An earlier cut mirrored the loader with an `if inner id is empty` guard.
	// The loader has no such condition, so the mirror was wrong in the one case
	// that matters: an entry keyed `ghost` carrying inner id `phantom` is
	// addressed by the router as ghost and was indexed here as phantom —
	// inventing a missing agent AND hiding a real lost field, two wrong answers
	// from one line. Do not reintroduce a condition the loader does not have.
	// (claude-home, probing PR #416 rather than reading it.)
	addMap := func(m map[string]any) {
		for key, e := range m {
			em, ok := e.(map[string]any)
			if !ok {
				continue
			}
			entry := make(map[string]any, len(em)+1)
			for k, v := range em {
				entry[k] = v
			}
			entry["id"] = key // the map key wins, unconditionally
			out[key] = entry
		}
	}
	addList := func(l []any) {
		for _, e := range l {
			em, ok := e.(map[string]any)
			if !ok {
				continue
			}
			if id, _ := em["id"].(string); id != "" {
				out[id] = em
			}
		}
	}

	switch v := anyDoc.(type) {
	case []any:
		addList(v)
	case map[string]any:
		switch inner := v["agents"].(type) {
		case map[string]any:
			addMap(inner)
		case []any:
			addList(inner)
		default:
			addMap(v) // bare map of id -> entry
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no agent entries found")
	}
	return out, nil
}

func diffRegistries(mergedRaw, liveRaw []byte) RegistryDrift {
	var d RegistryDrift
	merged, err := agentsByID(mergedRaw)
	if err != nil {
		return RegistryDrift{Unknown: fmt.Sprintf("merged registry unparseable: %v", err)}
	}
	live, err := agentsByID(liveRaw)
	if err != nil {
		return RegistryDrift{Unknown: fmt.Sprintf("live registry unparseable: %v", err)}
	}

	for id, mEntry := range merged {
		lEntry, present := live[id]
		if !present {
			d.MissingAgents = append(d.MissingAgents, id)
			continue
		}
		walkLeaves(mEntry, "", func(path string, up any) {
			lv, found := lookup(lEntry, path)
			switch {
			case !found:
				d.LostFields = append(d.LostFields, LostField{AgentID: id, Path: path, Upstream: scalar(up)})
			case scalar(lv) != scalar(up):
				d.ChangedFields = append(d.ChangedFields, ChangedField{
					AgentID: id, Path: path, Upstream: scalar(up), Live: scalar(lv),
				})
			}
		})
	}
	for id := range live {
		if _, ok := merged[id]; !ok {
			d.AheadAgents = append(d.AheadAgents, id)
		}
	}

	// Set here rather than in the caller: the function that knows the comparison
	// actually happened is the one that must say so. (Caught by
	// TestBothRegistryShapesParse — the flag was set by the wrapper, so a direct
	// call reported a successful comparison as "unknown".)
	d.Checked = true

	sort.Strings(d.MissingAgents)
	sort.Strings(d.AheadAgents)
	sort.Slice(d.LostFields, func(i, j int) bool { return keyOf(d.LostFields[i]) < keyOf(d.LostFields[j]) })
	sort.Slice(d.ChangedFields, func(i, j int) bool {
		return d.ChangedFields[i].AgentID+d.ChangedFields[i].Path < d.ChangedFields[j].AgentID+d.ChangedFields[j].Path
	})
	return d
}

func keyOf(l LostField) string { return l.AgentID + "\x00" + l.Path }

// walkLeaves visits every scalar leaf, so a field nobody has thought about yet
// is covered the day it is added.
func walkLeaves(v any, prefix string, fn func(path string, val any)) {
	switch t := v.(type) {
	case map[string]any:
		for k, sub := range t {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			walkLeaves(sub, p, fn)
		}
	case []any:
		if prefix != "" {
			fn(prefix, t) // compare lists whole; order matters in a registry
		}
	default:
		if prefix != "" {
			fn(prefix, t)
		}
	}
}

func lookup(m map[string]any, path string) (any, bool) {
	var cur any = m
	for _, part := range strings.Split(path, ".") {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = mm[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func scalar(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}
