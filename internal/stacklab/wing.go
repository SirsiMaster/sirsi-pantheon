// Package stacklab implements ADR-066 (Stack Lab wing authority) checks: a
// declared peer wing is canonical ONLY when it has a schema-valid record on
// its owning repo's origin/main AND a matching SHA-256 pin in the
// sirsi-stacklab registry AND a roster entry. See doctor.go for the check
// itself; this file holds the wing record shape (contracts/stacklab/v2/
// wing.schema.json) and a minimal validator.
//
// No JSON-schema library is added (go.mod has none, and the schema is small
// and stable) — this hand-rolled validator mirrors wing.schema.json's
// required fields, enums and patterns directly.
package stacklab

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// WingSchemaConst is the only accepted "schema" field value (wing.schema.json's
// "schema": {"const": "sirsi.stacklab.wing.v1"}).
const WingSchemaConst = "sirsi.stacklab.wing.v1"

var (
	wingIDPattern    = regexp.MustCompile(`^stacklab\.wing\.[a-z0-9][a-z0-9.-]*$`)
	projectIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	namespacePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	peerWingPattern  = regexp.MustCompile(`^stacklab\.wing\.`)
)

// WingRecord mirrors contracts/stacklab/v2/wing.schema.json.
type WingRecord struct {
	Schema          string `json:"schema"`
	ID              string `json:"id"`
	Owner           string `json:"owner"`
	ProjectID       string `json:"project_id"`
	RouterNamespace string `json:"router_namespace"`
	Class           string `json:"class"`
	Scope           string `json:"scope"`
	Status          string `json:"status"`
	FirstGate       string `json:"first_gate"`
	Workspace       struct {
		RepositoryRoot      string   `json:"repository_root"`
		WritableRoots       []string `json:"writable_roots"`
		EvidenceRoot        string   `json:"evidence_root"`
		SharedPayloadAccess string   `json:"shared_payload_access"`
		BoundaryPolicy      string   `json:"boundary_policy"`
	} `json:"workspace"`
	Handoffs struct {
		Inbound          string   `json:"inbound"`
		Outbound         string   `json:"outbound"`
		AllowedPeerWings []string `json:"allowed_peer_wings"`
	} `json:"handoffs"`
	Provenance struct {
		LifecycleTaskID  string   `json:"lifecycle_task_id"`
		ComponentCatalog string   `json:"component_catalog"`
		ReceiptLinks     []string `json:"receipt_links"`
	} `json:"provenance"`
	Mirrors struct {
		Repository string `json:"repository"`
		Desktop    string `json:"desktop"`
		Workspace  string `json:"workspace"`
	} `json:"mirrors"`
	NextAction string `json:"next_action"`
}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

// ValidateWing decodes and validates raw against wing.schema.json. additionalProperties:false
// is enforced via DisallowUnknownFields; every other constraint (required, enum, pattern,
// minLength/minItems) is checked by hand below.
func ValidateWing(raw []byte) (*WingRecord, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var w WingRecord
	if err := dec.Decode(&w); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	var errs []string
	add := func(cond bool, msg string) {
		if cond {
			errs = append(errs, msg)
		}
	}

	add(w.Schema != WingSchemaConst, fmt.Sprintf("schema: must equal %q", WingSchemaConst))
	add(!wingIDPattern.MatchString(w.ID), "id: must match ^stacklab\\.wing\\.[a-z0-9][a-z0-9.-]*$")
	add(strings.TrimSpace(w.Owner) == "", "owner: required, non-empty")
	add(!projectIDPattern.MatchString(w.ProjectID), "project_id: must match ^[a-z0-9][a-z0-9-]*$")
	add(!namespacePattern.MatchString(w.RouterNamespace), "router_namespace: must match ^[a-z0-9][a-z0-9-]*$")
	add(!oneOf(w.Class, "engine", "control-plane", "operator-surface", "transport", "hardware", "application"), "class: invalid enum value "+w.Class)
	add(strings.TrimSpace(w.Scope) == "", "scope: required, non-empty")
	add(!oneOf(w.Status, "intake", "active", "held", "retired"), "status: invalid enum value "+w.Status)
	add(strings.TrimSpace(w.FirstGate) == "", "first_gate: required, non-empty")

	add(len(w.Workspace.RepositoryRoot) < 2, "workspace.repository_root: minLength 2")
	add(len(w.Workspace.WritableRoots) < 1, "workspace.writable_roots: minItems 1")
	for _, r := range w.Workspace.WritableRoots {
		add(len(r) < 2, "workspace.writable_roots[]: minLength 2")
	}
	add(len(w.Workspace.EvidenceRoot) < 2, "workspace.evidence_root: minLength 2")
	add(!oneOf(w.Workspace.SharedPayloadAccess, "none", "read-only-by-identity"), "workspace.shared_payload_access: invalid enum value "+w.Workspace.SharedPayloadAccess)
	add(w.Workspace.BoundaryPolicy != "default-deny", "workspace.boundary_policy: must equal \"default-deny\"")

	add(w.Handoffs.Inbound != "router-receipt-only", "handoffs.inbound: must equal \"router-receipt-only\"")
	add(w.Handoffs.Outbound != "router-receipt-only", "handoffs.outbound: must equal \"router-receipt-only\"")
	for _, p := range w.Handoffs.AllowedPeerWings {
		add(!peerWingPattern.MatchString(p), "handoffs.allowed_peer_wings[]: must match ^stacklab\\.wing\\. — got "+p)
	}

	add(strings.TrimSpace(w.Provenance.LifecycleTaskID) == "", "provenance.lifecycle_task_id: required, non-empty")
	add(strings.TrimSpace(w.Provenance.ComponentCatalog) == "", "provenance.component_catalog: required, non-empty")
	for _, l := range w.Provenance.ReceiptLinks {
		add(strings.TrimSpace(l) == "", "provenance.receipt_links[]: minLength 1")
	}

	add(!oneOf(w.Mirrors.Repository, "current", "pending", "stale", "blocked"), "mirrors.repository: invalid enum value "+w.Mirrors.Repository)
	add(!oneOf(w.Mirrors.Desktop, "current", "pending", "stale", "blocked"), "mirrors.desktop: invalid enum value "+w.Mirrors.Desktop)
	add(!oneOf(w.Mirrors.Workspace, "current", "pending", "stale", "blocked"), "mirrors.workspace: invalid enum value "+w.Mirrors.Workspace)

	add(strings.TrimSpace(w.NextAction) == "", "next_action: required, non-empty")

	if len(errs) > 0 {
		return &w, fmt.Errorf("schema violation(s): %s", strings.Join(errs, "; "))
	}
	return &w, nil
}

// ContentSHA256 is the pin hash function: hex-encoded SHA-256 of the exact
// origin record bytes. The registry pins this value under wings/pinned/.
func ContentSHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// pinWingID extracts the "id" field of a sirsi-stacklab wings/pinned/*.json
// entry. Confirmed live against the real registry (2026-09-16): a pin is a
// byte-for-byte copy of the owning lane's origin wing record (see
// wings/pinned/PINNED.md), NOT a small pointer file — and its filename does
// not reliably derive from the lane (wings/pinned/router-wing-ra-v1.json
// pins stacklab.wing.m1-ra). So pins are matched to a roster wing id by
// content (this field), never by filename.
func pinWingID(raw []byte) (string, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", fmt.Errorf("decode registry pin: %w", err)
	}
	if strings.TrimSpace(p.ID) == "" {
		return "", fmt.Errorf("registry pin has no id field")
	}
	return p.ID, nil
}
