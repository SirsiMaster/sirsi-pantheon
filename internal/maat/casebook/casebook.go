// Package casebook turns Ma'at's factual decision feed into a local System One
// view: searchable, classified, prioritized cases with explicit evidence and
// actor/resource relationships. It never recomputes or overrides Ma'at policy.
package casebook

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

type Status string

const (
	StatusOpen     Status = "open"
	StatusResolved Status = "resolved"
)

type Priority string

const (
	PriorityUrgent Priority = "urgent"
	PriorityHigh   Priority = "high"
	PriorityNormal Priority = "normal"
)

// ResolutionPath is the controlled route that prevents an open Ma'at case from
// becoming a passive status. It can request a new producing decision, a
// deliberately confirmed owner conclusion, or a named closed repair registry
// entry; it never carries an executable command.
type ResolutionPath struct {
	Kind string `json:"kind"`
	// ActionID is a closed Ma'at action identifier, never a command. Native
	// surfaces may map it only to a matching in-process or CLI registry entry.
	ActionID             string           `json:"action_id,omitempty"`
	Title                string           `json:"title"`
	Detail               string           `json:"detail"`
	Evidence             string           `json:"evidence,omitempty"`
	RequiresConfirmation bool             `json:"requires_confirmation"`
	Steps                []ResolutionStep `json:"steps,omitempty"`
}

// ResolutionStep is one visible, non-authorizing part of a recovery route.
// A Casebook consumer may guide an operator through these levels, but it must
// never interpret a producer hint as permission to execute a command.
type ResolutionStep struct {
	Level                int    `json:"level"`
	Title                string `json:"title"`
	Detail               string `json:"detail"`
	Evidence             string `json:"evidence,omitempty"`
	RequiresConfirmation bool   `json:"requires_confirmation"`
}

// Case is a stable, operator-readable projection of exactly one recorded
// Ma'at decision. It has no independent lifecycle or write authority.
type Case struct {
	ID                   string                  `json:"id"`
	Time                 string                  `json:"time"`
	Host                 string                  `json:"host"`
	Kind                 string                  `json:"kind"`
	Category             string                  `json:"category"`
	Status               Status                  `json:"status"`
	Priority             Priority                `json:"priority"`
	Requester            string                  `json:"requester"`
	Resource             string                  `json:"resource,omitempty"`
	Affected             string                  `json:"affected,omitempty"`
	Determination        string                  `json:"determination"`
	Assessed             string                  `json:"assessed"`
	Why                  string                  `json:"why"`
	Evidence             string                  `json:"evidence,omitempty"`
	Resolution           string                  `json:"resolution,omitempty"`
	NextAction           *ResolutionPath         `json:"next_action,omitempty"`
	SystemOne            *maat.MaatVerdict       `json:"system_one,omitempty"`
	SystemOneCalibration *maat.CalibrationRecord `json:"system_one_calibration,omitempty"`
}

// Node and Edge form a deliberately small evidence graph. The dashboard can
// render it or use it to drill from a decision to its actors and evidence.
type Node struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type Summary struct {
	Total    int `json:"total"`
	Open     int `json:"open"`
	Urgent   int `json:"urgent"`
	High     int `json:"high"`
	Resolved int `json:"resolved"`
}

type View struct {
	Cases   []Case  `json:"cases"`
	Summary Summary `json:"summary"`
	Graph   Graph   `json:"graph"`
}

type Query struct {
	Text   string
	Kind   string
	Status Status
	Limit  int
}

// Build faithfully projects decisions; classification is descriptive and is
// never used to change the source decision's determination.
func Build(decisions []maat.Decision) View {
	acceptances := make(map[string]maat.Decision)
	for _, decision := range decisions {
		if decision.Kind == "diagnostic owner acceptance" && decision.ResolutionFor != "" {
			if previous, found := acceptances[decision.ResolutionFor]; !found || decision.Time > previous.Time {
				acceptances[decision.ResolutionFor] = decision
			}
		}
	}
	cases := make([]Case, 0, len(decisions))
	for _, decision := range decisions {
		if decision.Kind == "diagnostic owner acceptance" && decision.ResolutionFor != "" {
			if hasOwnerReview(decisions, decision.ResolutionFor) {
				continue
			}
		}
		c := project(decision)
		if acceptance, accepted := acceptances[decision.Evidence]; accepted {
			c.Status = StatusResolved
			c.Resolution = acceptance.Why
		}
		c.NextAction = resolutionPath(c)
		cases = append(cases, c)
	}
	sort.SliceStable(cases, func(i, j int) bool { return cases[i].Time > cases[j].Time })
	return viewFor(cases)
}

func hasOwnerReview(decisions []maat.Decision, evidence string) bool {
	for _, decision := range decisions {
		if decision.Kind == "diagnostic owner review" && decision.Evidence == evidence {
			return true
		}
	}
	return false
}

func Search(decisions []maat.Decision, query Query) View {
	if query.Limit <= 0 {
		query.Limit = 50
	}
	needle := strings.ToLower(strings.TrimSpace(query.Text))
	kind := strings.ToLower(strings.TrimSpace(query.Kind))
	var matched []Case
	for _, c := range Build(decisions).Cases {
		if kind != "" && strings.ToLower(c.Kind) != kind && strings.ToLower(c.Category) != kind {
			continue
		}
		if query.Status != "" && c.Status != query.Status {
			continue
		}
		if needle != "" && !matches(c, needle) {
			continue
		}
		matched = append(matched, c)
		if len(matched) == query.Limit {
			break
		}
	}
	return viewFor(matched)
}

func project(d maat.Decision) Case {
	determination := strings.ToLower(strings.TrimSpace(d.Determination))
	c := Case{
		Time: d.Time, Host: d.Host, Kind: d.Kind, Category: classify(d),
		Requester: d.Requester, Resource: d.Resource, Affected: d.Affected,
		Determination: d.Determination, Assessed: d.Assessed, Why: d.Why, Evidence: d.Evidence,
		SystemOne: d.SystemOne, SystemOneCalibration: d.SystemOneCalibration,
		Status: statusFor(determination), Priority: priorityFor(determination),
	}
	// A calibration is a completed historical comparison between an existing
	// auto-pass and an independent outcome. A block can be urgent in the
	// original screen's case; it is not an unresolved action on this receipt.
	if d.SystemOneCalibration != nil {
		c.Status = StatusResolved
		c.Priority = PriorityNormal
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{d.Time, d.Host, d.Kind, d.Requester, d.Resource, d.Determination, d.Evidence}, "\x00")))
	c.ID = "maat-case-" + hex.EncodeToString(sum[:8])
	return c
}

func classify(d maat.Decision) string {
	kind := strings.ToLower(d.Kind)
	switch {
	case strings.Contains(kind, "system one"):
		return "governance"
	case strings.Contains(kind, "cede"):
		return "allocation"
	case strings.Contains(kind, "reservation"):
		return "allocation"
	case strings.Contains(kind, "conflict"):
		return "contention"
	case strings.Contains(kind, "guard"), strings.Contains(kind, "window"), strings.Contains(kind, "ci"):
		return "governance"
	default:
		return "assessment"
	}
}

func statusFor(determination string) Status {
	switch determination {
	case "grant", "granted", "release", "released", "pass", "passed", "resolved", "complete", "completed", "accepted":
		return StatusResolved
	default:
		return StatusOpen
	}
}

func priorityFor(determination string) Priority {
	switch determination {
	case "fail", "failed", "block", "blocked", "refuse", "refused", "decline", "declined":
		return PriorityUrgent
	case "pending", "queue", "queued", "counter":
		return PriorityHigh
	default:
		return PriorityNormal
	}
}

func resolutionPath(c Case) *ResolutionPath {
	if c.Status == StatusResolved {
		return nil
	}
	if c.Kind == "failure memory preflight" && c.Evidence != "" {
		return &ResolutionPath{
			Kind: "failure_memory_recheck", Title: "Resolve the retained failure-memory finding",
			Detail:               "Use the exact retained preflight receipt and its guidance. Ma'at does not execute a producer-supplied repair or treat a historical observation as current.",
			Evidence:             c.Evidence,
			RequiresConfirmation: true,
			Steps: []ResolutionStep{
				{Level: 1, Title: "Inspect the retained evidence", Detail: "Open the exact failure-memory receipt and confirm its component, profile, operation, incident identity, and recovery guidance.", Evidence: c.Evidence},
				{Level: 2, Title: "Use the authorized bounded recovery", Detail: "Choose the approved recovery path for this operation; recovery guidance is evidence, not an executable command or blanket permission.", Evidence: c.Evidence, RequiresConfirmation: true},
				{Level: 3, Title: "Run a fresh exact-scope preflight", Detail: "Re-run the same failure-memory preflight after the bounded recovery and explicitly retain the new factual result in Casebook.", Evidence: c.Evidence, RequiresConfirmation: true},
			},
		}
	}
	if c.Kind == "diagnostic owner review" && c.Evidence != "" {
		return &ResolutionPath{
			Kind: "owner_acceptance", Title: "Record an owner acceptance",
			Detail:   "Review the exact diagnostic evidence, document the owner conclusion, and confirm it. This records a conclusion; it does not claim a system repair.",
			Evidence: c.Evidence, RequiresConfirmation: true,
		}
	}
	if c.SystemOne != nil && !c.SystemOne.Floor.Passed {
		return failedFloorRecovery(c)
	}
	if c.SystemOne != nil && c.SystemOne.Gate == maat.GateEscalate {
		return &ResolutionPath{
			Kind: "system_one_review", Title: "Open an evidence-bound review",
			Detail:   "This System One screen cannot bind a sensitive or uncertain result. Record a review for the exact screen evidence, then explicitly accept its documented conclusion.",
			Evidence: c.Evidence,
		}
	}
	if repair, ok := systemOneRepair(c.SystemOne, c.Evidence); ok {
		return repair
	}
	return &ResolutionPath{
		Kind: "owner_review", Title: "Create an evidence-bound owner review",
		Detail:   "Record the exact evidence and owner review before accepting a conclusion. Ma'at does not close an open case by assumption.",
		Evidence: c.Evidence,
	}
}

// systemOneRepair projects only a singular, closed Ma'at repair reference. A
// screen with no repair, a mismatched reference, or multiple independent
// repairs still receives the evidence-bound review route above; Casebook never
// turns a producer hint into an executable command.
func systemOneRepair(verdict *maat.MaatVerdict, evidence string) (*ResolutionPath, bool) {
	if verdict == nil {
		return nil, false
	}
	ids := map[string]bool{}
	for _, finding := range verdict.Findings {
		if finding.RepairID != "" {
			ids[finding.RepairID] = true
		}
	}
	if len(ids) != 1 || !ids[maat.SystemOneRepairLaunchdDisabled] {
		return nil, false
	}
	return &ResolutionPath{
		Kind: "maat_repair", ActionID: maat.SystemOneRepairLaunchdDisabled,
		Title: "Restore the managed launchd labels", Evidence: evidence,
		Detail:               "Ma'at will re-read the currently disabled Sirsi labels, restore only the verified managed set, re-check the same diagnostic, and retain the outcome. Then create a fresh System One screen.",
		RequiresConfirmation: true,
		Steps: []ResolutionStep{
			{Level: 1, Title: "Review the retained finding", Detail: "Confirm this screen describes the disabled managed LaunchAgent state.", RequiresConfirmation: true},
			{Level: 2, Title: "Apply Ma'at's bounded recovery", Detail: "Enable and bootstrap only labels that are both managed and disabled at preflight.", RequiresConfirmation: true},
			{Level: 3, Title: "Re-screen the Mac", Detail: "Inspect the recorded recovery receipt, then create a new System One screen so the original observation is not treated as current."},
		},
	}, true
}

func failedFloorRecovery(c Case) *ResolutionPath {
	failed := make([]string, 0, len(c.SystemOne.Floor.Checks))
	for _, check := range c.SystemOne.Floor.Checks {
		if check.Passed {
			continue
		}
		item := check.Name
		if detail := strings.TrimSpace(check.Detail); detail != "" {
			item += ": " + detail
		}
		failed = append(failed, item)
	}
	sort.Strings(failed)
	correction := "Use the retained floor-check detail to make the bounded correction; Ma'at will not execute a producer-supplied command."
	for _, finding := range c.SystemOne.Findings {
		if hint := strings.TrimSpace(finding.FixHint); hint != "" {
			correction = hint
			break
		}
	}
	return &ResolutionPath{
		Kind: "system_one_floor_recovery", Title: "Resolve failed deterministic checks",
		Detail:   "Follow the three recovery levels against the retained evidence, then record the result. The original screen remains open until a new evidence-bound decision is recorded.",
		Evidence: c.Evidence, RequiresConfirmation: true,
		Steps: []ResolutionStep{
			{Level: 1, Title: "Inspect the failed evidence", Detail: strings.Join(failed, "; "), Evidence: c.Evidence},
			{Level: 2, Title: "Apply the bounded correction", Detail: correction, Evidence: c.Evidence},
			{Level: 3, Title: "Re-screen and record review", Detail: "Re-run the exact System One screen after the correction, inspect the new evidence, then explicitly record an owner review or acceptance.", Evidence: c.Evidence, RequiresConfirmation: true},
		},
	}
}

func matches(c Case, needle string) bool {
	return strings.Contains(strings.ToLower(strings.Join([]string{
		c.Time, c.Host, c.Kind, c.Category, c.Requester, c.Resource,
		c.Affected, c.Determination, c.Assessed, c.Why, c.Evidence, c.Resolution, resolutionText(c.NextAction),
		calibrationText(c.SystemOneCalibration),
	}, "\n")), needle)
}

func calibrationText(record *maat.CalibrationRecord) string {
	if record == nil {
		return ""
	}
	return strings.Join([]string{record.ScreenEvidence, record.FrontierEvidence, string(record.ScreenGate), string(record.FrontierGate)}, "\n")
}

func resolutionText(action *ResolutionPath) string {
	if action == nil {
		return ""
	}
	parts := []string{action.Kind, action.Title, action.Detail, action.Evidence}
	for _, step := range action.Steps {
		parts = append(parts, step.Title, step.Detail, step.Evidence)
	}
	return strings.Join(parts, "\n")
}

func viewFor(cases []Case) View {
	v := View{Cases: cases, Graph: graphFor(cases)}
	for _, c := range cases {
		v.Summary.Total++
		if c.Status == StatusResolved {
			v.Summary.Resolved++
		} else {
			v.Summary.Open++
		}
		if c.Priority == PriorityUrgent {
			v.Summary.Urgent++
		}
		if c.Priority == PriorityHigh {
			v.Summary.High++
		}
	}
	return v
}

func graphFor(cases []Case) Graph {
	nodes := map[string]Node{}
	var edges []Edge
	add := func(id, kind, label string) {
		if id != "" && label != "" {
			nodes[id] = Node{ID: id, Kind: kind, Label: label}
		}
	}
	for _, c := range cases {
		caseID := "decision:" + c.ID
		add(caseID, "decision", c.Kind+" · "+c.Determination)
		for _, relation := range []struct{ kind, prefix, label string }{
			{"requested_by", "agent:", c.Requester},
			{"affects", "agent:", c.Affected},
			{"assesses", "resource:", c.Resource},
			{"evidenced_by", "evidence:", c.Evidence},
		} {
			if relation.label == "" {
				continue
			}
			to := relation.prefix + relation.label
			kind := strings.TrimSuffix(relation.prefix, ":")
			add(to, kind, relation.label)
			edges = append(edges, Edge{From: caseID, To: to, Kind: relation.kind})
		}
		if calibration := c.SystemOneCalibration; calibration != nil {
			for _, evidence := range []string{calibration.ScreenEvidence, calibration.FrontierEvidence} {
				if evidence == "" {
					continue
				}
				to := "evidence:" + evidence
				add(to, "evidence", evidence)
				edges = append(edges, Edge{From: caseID, To: to, Kind: "calibrated_against"})
			}
		}
	}
	keys := make([]string, 0, len(nodes))
	for key := range nodes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := Graph{Nodes: make([]Node, 0, len(keys)), Edges: edges}
	for _, key := range keys {
		result.Nodes = append(result.Nodes, nodes[key])
	}
	return result
}
