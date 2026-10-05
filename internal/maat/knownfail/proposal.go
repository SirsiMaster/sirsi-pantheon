package knownfail

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const proposalSchema = "sirsi.maat.known-failure-proposal.v1"

var proposalID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,80}$`)

// Proposal is a host-local Ma'at evidence object.  It records a newly observed
// recurring failure without silently editing the shipped Stack Lab catalog.  A
// proposal is deliberately not considered by Match: a local observation must
// be reviewed and promoted before it changes fabric-wide recognition.
type Proposal struct {
	Schema        string `json:"schema"`
	ID            string `json:"id"`
	Title         string `json:"title,omitempty"`
	Signature     string `json:"signature"`
	Cause         string `json:"cause"`
	Status        string `json:"status"`
	CreatedAtUTC  string `json:"created_at_utc"`
	CatalogSHA256 string `json:"catalog_sha256"`
}

// DefaultProposalDir is the node-local Ma'at intake.  The environment override
// is intentionally useful for tests and managed deployments; it never changes
// the immutable catalog embedded in the released product.
func DefaultProposalDir() string {
	if dir := strings.TrimSpace(os.Getenv("SIRSI_MAAT_KNOWN_FAILURE_PROPOSALS_DIR")); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".sirsi", "maat", "known-failures", "proposals")
}

// CatalogSHA256 pins a proposal to the exact built-in catalog it was assessed
// against.  Later promotion can therefore distinguish a genuinely new report
// from one made against a different catalog generation.
func CatalogSHA256() string {
	digest := sha256.Sum256(catalogJSON)
	return hex.EncodeToString(digest[:])
}

func validateProposal(p Proposal) error {
	if p.Schema != proposalSchema {
		return fmt.Errorf("known-failure proposal: unsupported schema %q", p.Schema)
	}
	if !proposalID.MatchString(p.ID) {
		return fmt.Errorf("known-failure proposal id %q must be lowercase letters, digits, or single hyphens", p.ID)
	}
	if strings.TrimSpace(p.Signature) == "" || strings.TrimSpace(p.Cause) == "" {
		return fmt.Errorf("known-failure proposal %q: signature and cause are required", p.ID)
	}
	if _, err := regexp.Compile("(?i)" + p.Signature); err != nil {
		return fmt.Errorf("known-failure proposal %q: bad signature: %w", p.ID, err)
	}
	if p.Status != "proposed" || p.CreatedAtUTC == "" || p.CatalogSHA256 != CatalogSHA256() {
		return fmt.Errorf("known-failure proposal %q: invalid provenance", p.ID)
	}
	return nil
}

// Propose records a create-only, read-back-verified local proposal.  It never
// overwrites a prior proposal and never edits a source checkout.
func Propose(dir string, entry Entry) (Proposal, string, error) {
	p := Proposal{
		Schema:        proposalSchema,
		ID:            strings.TrimSpace(entry.ID),
		Title:         strings.TrimSpace(entry.Title),
		Signature:     strings.TrimSpace(entry.Signature),
		Cause:         strings.TrimSpace(entry.Cause),
		Status:        "proposed",
		CreatedAtUTC:  time.Now().UTC().Format(time.RFC3339),
		CatalogSHA256: CatalogSHA256(),
	}
	if err := validateProposal(p); err != nil {
		return Proposal{}, "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Proposal{}, "", fmt.Errorf("create Ma'at known-failure proposal directory: %w", err)
	}
	path := filepath.Join(dir, p.ID+".json")
	bytes, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return Proposal{}, "", err
	}
	bytes = append(bytes, '\n')
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return Proposal{}, "", fmt.Errorf("known-failure proposal %q already exists at %s", p.ID, path)
		}
		return Proposal{}, "", fmt.Errorf("create Ma'at known-failure proposal: %w", err)
	}
	written := 0
	for written < len(bytes) {
		n, writeErr := f.Write(bytes[written:])
		written += n
		if writeErr != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return Proposal{}, "", fmt.Errorf("write Ma'at known-failure proposal: %w", writeErr)
		}
		if n == 0 {
			_ = f.Close()
			_ = os.Remove(path)
			return Proposal{}, "", fmt.Errorf("write Ma'at known-failure proposal: short write")
		}
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return Proposal{}, "", fmt.Errorf("sync Ma'at known-failure proposal: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return Proposal{}, "", fmt.Errorf("close Ma'at known-failure proposal: %w", err)
	}
	readback, err := os.ReadFile(path)
	if err != nil {
		return Proposal{}, "", fmt.Errorf("read back Ma'at known-failure proposal: %w", err)
	}
	if string(readback) != string(bytes) {
		return Proposal{}, "", fmt.Errorf("Ma'at known-failure proposal readback differs from written bytes")
	}
	var verified Proposal
	if err := json.Unmarshal(readback, &verified); err != nil {
		return Proposal{}, "", fmt.Errorf("decode Ma'at known-failure proposal readback: %w", err)
	}
	if err := validateProposal(verified); err != nil {
		return Proposal{}, "", err
	}
	return verified, path, nil
}

// ReadProposals returns valid local proposals in a deterministic order.  Any
// malformed item is an integrity error rather than a silently ignored issue.
func ReadProposals(dir string) ([]Proposal, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Ma'at known-failure proposals: %w", err)
	}
	var proposals []Proposal
	for _, item := range entries {
		if item.IsDir() || !strings.HasSuffix(item.Name(), ".json") {
			return nil, fmt.Errorf("unexpected Ma'at known-failure proposal entry %q", item.Name())
		}
		bytes, err := os.ReadFile(filepath.Join(dir, item.Name()))
		if err != nil {
			return nil, err
		}
		var proposal Proposal
		if err := json.Unmarshal(bytes, &proposal); err != nil {
			return nil, fmt.Errorf("decode Ma'at known-failure proposal %q: %w", item.Name(), err)
		}
		if err := validateProposal(proposal); err != nil {
			return nil, err
		}
		if item.Name() != proposal.ID+".json" {
			return nil, fmt.Errorf("Ma'at known-failure proposal filename %q does not bind id %q", item.Name(), proposal.ID)
		}
		proposals = append(proposals, proposal)
	}
	sort.Slice(proposals, func(i, j int) bool { return proposals[i].ID < proposals[j].ID })
	return proposals, nil
}
