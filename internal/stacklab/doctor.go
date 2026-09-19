package stacklab

import (
	"fmt"
	"sort"
	"strings"
)

// Finding kinds — the ADR-066 taxonomy (exact strings; --json emits these).
const (
	KindStrandedUnbuilt  = "stranded/unbuilt"  // declared peer, no schema-valid record on origin/main
	KindUnpushedStranded = "unpushed/stranded" // record exists locally/non-main branch, no origin/main record (the io-connect class)
	KindUnpinned         = "unpinned"          // origin record exists, registry has no pin or pin hash mismatches
	KindUndeclared       = "undeclared"        // registry pin or origin record exists for a wing NOT in the roster
	KindInvalid          = "invalid"           // record fails contracts/stacklab/v2/wing.schema.json
)

// RegistryRepo is SirsiMaster/sirsi-stacklab, the ADR-066 authoritative universal registry.
const RegistryRepo = "SirsiMaster/sirsi-stacklab"

// RegistryPinsDir is where sirsi-stacklab keeps its pinned wing records — a
// pin is a byte-for-byte copy of the owning lane's origin record (confirmed
// live against the real registry 2026-09-16, see wings/pinned/PINNED.md), so
// pins are matched to a wing id by their own "id" field, never by filename:
// wings/pinned/router-wing-ra-v1.json pins stacklab.wing.m1-ra, which a
// lane-derived filename guess would miss.
const RegistryPinsDir = "wings/pinned"

// LaneRepoMap maps a wing id's lane (the id with the "stacklab.wing." prefix
// stripped) to its owning GitHub repo ("owner/repo"), per ADR-066 §6 + the
// task's explicit mapping. Confident entries only; everything else is a
// documented TODO — ADR-066 §6 was not available to this session and a wrong
// guess here would silently mis-route the check to a repo that has nothing
// to do with the lane. Doctor reports an unmapped lane as stranded/unbuilt
// with a "mapping unknown" detail rather than fabricating a repo.
var LaneRepoMap = map[string]string{
	"io-connect": "SirsiMaster/sirsi-io-connect",
	// TODO(ADR-066 §6): sne-engine — inference-engine repo name not confirmed
	//   in this session (memory names the *project* "Sirsi Inference Engine"
	//   but not its GitHub repo).
	// TODO(ADR-066 §6): pantheon.pt-wing-001 — sirsi-pantheon already carries
	//   a different wing (stacklab.wing.m1-ra, docs/router-service/stacklab/)
	//   at a non-canonical path, and the live registry additionally pins a
	//   THIRD record, pantheon-pt-wing-001.catalog.json (id
	//   stacklab.wing.pantheon-pt-wing-001.catalog) that is itself
	//   undeclared in today's roster — confirm the intended id/repo pairing
	//   against ADR-066 §6 before mapping this lane.
	// TODO(ADR-066 §6): hardware-estate — owning repo unknown (task's own
	//   mapping table marks it "?").
}

// Finding is one ADR-066 taxonomy violation for one declared peer wing (or,
// for KindUndeclared, one registry/origin record with no roster entry).
type Finding struct {
	WingID string `json:"wing_id"`
	Kind   string `json:"finding"`
	Detail string `json:"detail"`
}

// Report is the full `sirsi stacklab doctor` result.
type Report struct {
	Roster   []string  `json:"roster"`
	Findings []Finding `json:"findings"`
	// Unknown holds read failures (network/auth/parse) that make a verdict
	// impossible for that entry — never rendered as clean (router doctor's
	// IO7a: unknown is not clean).
	Unknown []string `json:"unknown,omitempty"`
}

// Clean reports whether every declared peer is built, pushed, pinned,
// declared and valid, AND nothing was left Unknown.
func (r Report) Clean() bool {
	return len(r.Findings) == 0 && len(r.Unknown) == 0
}

func wingPath(lane string) string {
	return fmt.Sprintf("contracts/stacklab/%s-wing-v1.json", lane)
}

func laneOf(wingID string) string {
	return strings.TrimPrefix(wingID, "stacklab.wing.")
}

// registryPin is one entry read from RegistryPinsDir.
type registryPin struct {
	Filename string
	Content  []byte
}

// loadRegistryPins reads every *.json file in the registry's pins directory
// and indexes it by the wing id in its own "id" field (see RegistryPinsDir
// doc). Non-JSON entries (PINNED.md, SHA256SUMS) are skipped. A per-file
// read/parse failure is recorded in unknown and that entry is skipped, not
// treated as absent.
func loadRegistryPins(reader RemoteReader) (map[string]registryPin, []string) {
	var unknown []string
	entries, exists, err := reader.ListDir(RegistryRepo, RegistryPinsDir, "main")
	if err != nil {
		return nil, append(unknown, fmt.Sprintf("list registry pins %s:%s: %v", RegistryRepo, RegistryPinsDir, err))
	}
	if !exists {
		return nil, nil
	}
	pins := make(map[string]registryPin, len(entries))
	for _, name := range entries {
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		path := RegistryPinsDir + "/" + name
		content, fexists, ferr := reader.ReadFile(RegistryRepo, path, "main")
		if ferr != nil {
			unknown = append(unknown, fmt.Sprintf("read registry pin %s:%s: %v", RegistryRepo, path, ferr))
			continue
		}
		if !fexists {
			continue // listed then vanished — races are not a verdict either way
		}
		wingID, iderr := pinWingID(content)
		if iderr != nil {
			unknown = append(unknown, fmt.Sprintf("registry pin %s: %v", path, iderr))
			continue
		}
		pins[wingID] = registryPin{Filename: name, Content: content}
	}
	return pins, unknown
}

// Run evaluates every wing id in roster against ADR-066 authority: origin/main
// record (schema-valid, per laneRepoMap), a matching registry pin, and
// roster membership both ways. reader must never be given a working tree or
// mirror — every call reads a ref (A35).
func Run(reader RemoteReader, roster []string, laneRepoMap map[string]string) Report {
	rosterSet := make(map[string]bool, len(roster))
	for _, id := range roster {
		rosterSet[id] = true
	}

	sortedRoster := append([]string(nil), roster...)
	sort.Strings(sortedRoster)

	rep := Report{Roster: sortedRoster}

	pins, pinsUnknown := loadRegistryPins(reader)
	rep.Unknown = append(rep.Unknown, pinsUnknown...)

	for _, wingID := range sortedRoster {
		// A35 hardening: a malformed roster entry must never become an API
		// path segment (contracts/stacklab/<lane>-wing-v1.json,
		// wings/pinned/<lane>...). Reject it before laneOf/wingPath ever run.
		if !wingIDPattern.MatchString(wingID) {
			rep.Findings = append(rep.Findings, Finding{wingID, KindInvalid,
				"roster entry does not match ^stacklab\\.wing\\.[a-z0-9][a-z0-9.-]*$ — refusing to derive a repo path from it"})
			continue
		}

		lane := laneOf(wingID)
		repo, known := laneRepoMap[lane]
		if !known {
			rep.Findings = append(rep.Findings, Finding{wingID, KindStrandedUnbuilt,
				"owning repo mapping unknown (TODO ADR-066 §6) — cannot check origin/main"})
			continue
		}

		content, exists, err := reader.ReadFile(repo, wingPath(lane), "main")
		if err != nil {
			rep.Unknown = append(rep.Unknown, fmt.Sprintf("%s: read %s@main:%s: %v", wingID, repo, wingPath(lane), err))
			continue
		}
		if !exists {
			if f, ok := classifyMissing(reader, wingID, repo, lane, &rep.Unknown); ok {
				rep.Findings = append(rep.Findings, f)
			}
			continue
		}

		if _, verr := ValidateWing(content); verr != nil {
			rep.Findings = append(rep.Findings, Finding{wingID, KindInvalid, verr.Error()})
			continue
		}

		originHash := ContentSHA256(content)
		pin, pinned := pins[wingID]
		if !pinned {
			rep.Findings = append(rep.Findings, Finding{wingID, KindUnpinned,
				fmt.Sprintf("origin/main record valid (sha256=%s) but no registry pin under %s:%s matches this id", originHash, RegistryRepo, RegistryPinsDir)})
			continue
		}
		if pinHash := ContentSHA256(pin.Content); pinHash != originHash {
			rep.Findings = append(rep.Findings, Finding{wingID, KindUnpinned,
				fmt.Sprintf("registry pin %s/%s sha256=%s does not match origin/main content sha256=%s", RegistryPinsDir, pin.Filename, pinHash, originHash)})
		}
	}

	for wingID, pin := range pins {
		if rosterSet[wingID] {
			continue
		}
		rep.Findings = append(rep.Findings, Finding{wingID, KindUndeclared,
			fmt.Sprintf("registry pin %s/%s exists but %q is not in the router wing's allowed_peer_wings", RegistryPinsDir, pin.Filename, wingID)})
	}

	sort.Slice(rep.Findings, func(i, j int) bool {
		if rep.Findings[i].WingID != rep.Findings[j].WingID {
			return rep.Findings[i].WingID < rep.Findings[j].WingID
		}
		return rep.Findings[i].Kind < rep.Findings[j].Kind
	})
	return rep
}

// classifyMissing distinguishes stranded/unbuilt (no record anywhere) from
// unpushed/stranded (a record exists on some non-main branch of the owning
// repo but never reached origin/main — the io-connect class). A failed scan
// (branch listing or a branch read) is not a verdict either way: it returns
// ok=false and only records the failure in unknown — the caller must not
// synthesize a finding from an inconclusive scan.
func classifyMissing(reader RemoteReader, wingID, repo, lane string, unknown *[]string) (finding Finding, ok bool) {
	branches, berr := reader.ListBranches(repo)
	if berr != nil {
		*unknown = append(*unknown, fmt.Sprintf("%s: list branches of %s: %v", wingID, repo, berr))
		return Finding{}, false
	}
	for _, b := range branches {
		if b == "main" {
			continue
		}
		_, exists, err := reader.ReadFile(repo, wingPath(lane), b)
		if err != nil {
			*unknown = append(*unknown, fmt.Sprintf("%s: read %s@%s:%s: %v", wingID, repo, b, wingPath(lane), err))
			continue
		}
		if exists {
			return Finding{wingID, KindUnpushedStranded,
				fmt.Sprintf("record exists on branch %q of %s but not on origin/main — open a recovery PR to publish it", b, repo)}, true
		}
	}
	return Finding{wingID, KindStrandedUnbuilt, fmt.Sprintf("no record at %s on any branch of %s — needs authoring", wingPath(lane), repo)}, true
}
