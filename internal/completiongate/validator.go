// Package completiongate validates router completion proofs without a runtime
// dependency on Python. It mirrors the portfolio completion-gate contract.
package completiongate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var classifications = map[string]bool{"commercial-product": true, "pilot": true, "prototype": true, "internal-tool": true, "platform-foundation": true, "research": true}
var finalStatuses = map[string]bool{"ready_for_review": true, "completed": true}
var passStatuses = map[string]bool{"pass": true, "passed": true}

func Validate(repoRoot, proofArg string) error {
	repo, err := resolveExistingPath(repoRoot)
	if err != nil {
		return fmt.Errorf("resolve repo: %w", err)
	}
	contract, err := loadObject(filepath.Join(repo, ".agents", "completion.contract.json"))
	if err != nil {
		return err
	}
	requiredDocs, err := validateContract(repo, contract)
	if err != nil {
		return err
	}
	proofPath, err := selectProof(repo, proofArg)
	if err != nil {
		return err
	}
	proof, err := loadObject(proofPath)
	if err != nil {
		return err
	}
	if err := validateProof(repo, proof, requiredDocs); err != nil {
		return err
	}
	return nil
}

// resolveExistingPath gives repository identity the same symlink semantics as
// the legacy completion gate's Path.resolve(). Completion proofs may name the
// physical checkout while the operator reaches it through a worktree symlink.
func resolveExistingPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Abs(resolved)
}

func loadObject(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("missing file: %s: %w", path, err)
	}
	var value any
	if err := json.Unmarshal(b, &value); err != nil {
		return nil, fmt.Errorf("invalid JSON in %s: %w", path, err)
	}
	o, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected JSON object in %s", path)
	}
	return o, nil
}

func stringAt(o map[string]any, key, where string) (string, error) {
	v, ok := o[key].(string)
	if !ok || strings.TrimSpace(v) == "" {
		return "", fmt.Errorf("%s: missing non-empty string %q", where, key)
	}
	return v, nil
}
func listAt(o map[string]any, key, where string) ([]any, error) {
	v, ok := o[key].([]any)
	if !ok || len(v) == 0 {
		return nil, fmt.Errorf("%s: missing non-empty list %q", where, key)
	}
	return v, nil
}
func objectAt(v any, where string) (map[string]any, error) {
	o, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected object", where)
	}
	return o, nil
}
func repoPath(repo, value string) string {
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(repo, value)
}

func validateContract(repo string, c map[string]any) ([]string, error) {
	for _, key := range []string{"repo_name", "release_target"} {
		if _, err := stringAt(c, key, "contract"); err != nil {
			return nil, err
		}
	}
	classification, err := stringAt(c, "classification", "contract")
	if err != nil {
		return nil, err
	}
	if !classifications[classification] {
		return nil, fmt.Errorf("contract: invalid classification %q", classification)
	}
	canon, err := listAt(c, "canon_documents", "contract")
	if err != nil {
		return nil, err
	}
	var required []string
	for i, raw := range canon {
		where := fmt.Sprintf("contract.canon_documents[%d]", i)
		item, err := objectAt(raw, where)
		if err != nil {
			return nil, err
		}
		path, err := stringAt(item, "path", where)
		if err != nil {
			return nil, err
		}
		role, err := stringAt(item, "role", where)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(repoPath(repo, path))
		if err != nil {
			return nil, fmt.Errorf("%s: missing canon doc %q", where, path)
		}
		if !info.IsDir() && info.Size() == 0 {
			return nil, fmt.Errorf("%s: empty canon doc %q", where, path)
		}
		if map[string]bool{"prd": true, "requirements": true, "scope": true, "test-plan": true, "release-plan": true}[role] {
			required = append(required, path)
		}
	}
	done, err := listAt(c, "done_definition", "contract")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for i, raw := range done {
		where := fmt.Sprintf("contract.done_definition[%d]", i)
		item, err := objectAt(raw, where)
		if err != nil {
			return nil, err
		}
		category, err := stringAt(item, "category", where)
		if err != nil {
			return nil, err
		}
		seen[category] = true
		criteria, err := listAt(item, "criteria", where)
		if err != nil {
			return nil, err
		}
		for _, criterion := range criteria {
			if s, ok := criterion.(string); !ok || strings.TrimSpace(s) == "" {
				return nil, fmt.Errorf("%s.criteria: all criteria must be non-empty strings", where)
			}
		}
	}
	for _, category := range []string{"product", "design", "technical", "operational", "narrative"} {
		if !seen[category] {
			return nil, fmt.Errorf("contract.done_definition: missing %q category", category)
		}
	}
	commands, err := listAt(c, "required_verification_commands", "contract")
	if err != nil {
		return nil, err
	}
	for i, raw := range commands {
		where := fmt.Sprintf("contract.required_verification_commands[%d]", i)
		item, err := objectAt(raw, where)
		if err != nil {
			return nil, err
		}
		if _, err := stringAt(item, "name", where); err != nil {
			return nil, err
		}
		if _, err := stringAt(item, "command", where); err != nil {
			return nil, err
		}
	}
	return required, nil
}

func selectProof(repo, value string) (string, error) {
	if value != "" {
		return repoPath(repo, value), nil
	}
	latest := filepath.Join(repo, ".agents", "completion.proof.json")
	if _, err := os.Stat(latest); err == nil {
		return latest, nil
	}
	entries, err := os.ReadDir(filepath.Join(repo, ".agents", "proofs"))
	if err != nil {
		return "", fmt.Errorf("no proof supplied and no completion proof exists")
	}
	var paths []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			paths = append(paths, filepath.Join(repo, ".agents", "proofs", e.Name()))
		}
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("no proof supplied and no completion proof exists")
	}
	sort.Slice(paths, func(i, j int) bool {
		a, _ := os.Stat(paths[i])
		b, _ := os.Stat(paths[j])
		return a.ModTime().After(b.ModTime())
	})
	return paths[0], nil
}

func validateProof(repo string, p map[string]any, requiredDocs []string) error {
	for _, key := range []string{"work_item_id", "agent_id"} {
		if _, err := stringAt(p, key, "proof"); err != nil {
			return err
		}
	}
	proofRepo, err := stringAt(p, "repo", "proof")
	if err != nil {
		return err
	}
	abs, err := resolveExistingPath(proofRepo)
	if err != nil {
		return err
	}
	if abs != repo {
		return fmt.Errorf("proof.repo %q does not match repo %q", proofRepo, repo)
	}
	status, err := stringAt(p, "status", "proof")
	if err != nil {
		return err
	}
	if !map[string]bool{"draft": true, "blocked": true, "ready_for_review": true, "completed": true}[status] {
		return fmt.Errorf("proof: invalid status %q", status)
	}
	classification, err := stringAt(p, "classification", "proof")
	if err != nil {
		return err
	}
	if !classifications[classification] {
		return fmt.Errorf("proof: invalid classification %q", classification)
	}
	canon, err := listAt(p, "canon_read", "proof")
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, raw := range canon {
		where := fmt.Sprintf("proof.canon_read[%d]", i)
		item, err := objectAt(raw, where)
		if err != nil {
			return err
		}
		path, err := stringAt(item, "path", where)
		if err != nil {
			return err
		}
		if _, err := stringAt(item, "read_at", where); err != nil {
			return err
		}
		seen[path] = true
		if _, err := os.Stat(repoPath(repo, path)); err != nil {
			return fmt.Errorf("%s: referenced missing canon doc %q", where, path)
		}
	}
	if finalStatuses[status] {
		for _, path := range requiredDocs {
			if !seen[path] {
				return fmt.Errorf("proof.canon_read missing required canon doc %q", path)
			}
		}
	}
	trace, err := listAt(p, "requirements_trace", "proof")
	if err != nil {
		return err
	}
	for i, raw := range trace {
		where := fmt.Sprintf("proof.requirements_trace[%d]", i)
		item, err := objectAt(raw, where)
		if err != nil {
			return err
		}
		doc, err := stringAt(item, "doc", where)
		if err != nil {
			return err
		}
		for _, key := range []string{"section", "claim"} {
			if _, err := stringAt(item, key, where); err != nil {
				return err
			}
		}
		for _, key := range []string{"files_changed", "evidence"} {
			if _, err := listAt(item, key, where); err != nil {
				return err
			}
		}
		if !seen[doc] {
			return fmt.Errorf("%s: doc %q was not listed in canon_read", where, doc)
		}
	}
	closure, err := objectAt(p["closure_evidence"], "proof.closure_evidence")
	if err != nil {
		return err
	}
	for _, key := range []string{"product", "design", "technical", "operational", "narrative"} {
		if _, err := stringAt(closure, key, "proof.closure_evidence"); err != nil {
			return err
		}
	}
	verify, err := listAt(p, "verification", "proof")
	if err != nil {
		return err
	}
	for i, raw := range verify {
		where := fmt.Sprintf("proof.verification[%d]", i)
		item, err := objectAt(raw, where)
		if err != nil {
			return err
		}
		for _, key := range []string{"name", "command", "evidence"} {
			if _, err := stringAt(item, key, where); err != nil {
				return err
			}
		}
		state, err := stringAt(item, "status", where)
		if err != nil {
			return err
		}
		if finalStatuses[status] && !passStatuses[strings.ToLower(state)] {
			return fmt.Errorf("%s: final proof requires passing status, got %q", where, state)
		}
	}
	working, err := objectAt(p["working_status"], "proof.working_status")
	if err != nil {
		return err
	}
	for _, key := range []string{"environment", "how_verified", "observed_result", "verified_at"} {
		if _, err := stringAt(working, key, "proof.working_status"); err != nil {
			return err
		}
	}
	handoffs, ok := p["handoffs"]
	if !ok {
		handoffs = []any{}
	}
	handoffList, ok := handoffs.([]any)
	if !ok {
		return fmt.Errorf("proof.handoffs: expected list")
	}
	for i, raw := range handoffList {
		where := fmt.Sprintf("proof.handoffs[%d]", i)
		item, err := objectAt(raw, where)
		if err != nil {
			return err
		}
		for _, key := range []string{"from", "to", "router_item", "status"} {
			if _, err := stringAt(item, key, where); err != nil {
				return err
			}
		}
	}
	blockers, ok := p["blockers"]
	if !ok {
		blockers = []any{}
	}
	blockerList, ok := blockers.([]any)
	if !ok {
		return fmt.Errorf("proof.blockers: expected list")
	}
	if status == "blocked" {
		if len(blockerList) == 0 {
			return fmt.Errorf("proof.blockers: blocked status requires at least one blocker")
		}
	}
	return nil
}
