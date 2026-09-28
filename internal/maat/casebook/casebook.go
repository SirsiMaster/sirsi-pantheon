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

// ResolutionPath is the non-authorizing route that prevents an open Ma'at
// case from becoming a passive status. It can request a new producing decision
// or a deliberately confirmed owner conclusion, but never performs a repair.
type ResolutionPath struct {
	Kind                 string `json:"kind"`
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
	if c.Kind == "diagnostic owner review" && c.Evidence != "" {
		return &ResolutionPath{
			Kind: "owner_acceptance", Title: "Record an owner acceptance",
			Detail:   "Review the exact diagnostic evidence, document the owner conclusion, and confirm it. This records a conclusion; it does not claim a system repair.",
			Evidence: c.Evidence, RequiresConfirmation: true,
		}
	}
	if c.SystemOne != nil && c.SystemOne.Gate == maat.GateEscalate {
		return &ResolutionPath{
			Kind: "system_one_review", Title: "Open an evidence-bound review",
			Detail:   "This System One screen cannot bind a sensitive or uncertain result. Record a review for the exact screen evidence, then explicitly accept its documented conclusion.",
			Evidence: c.Evidence,
		}
	}
	return &ResolutionPath{
		Kind: "owner_review", Title: "Create an evidence-bound owner review",
		Detail:   "Record the exact evidence and owner review before accepting a conclusion. Ma'at does not close an open case by assumption.",
		Evidence: c.Evidence,
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
	return action.Kind + "\n" + action.Title + "\n" + action.Detail + "\n" + action.Evidence
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
