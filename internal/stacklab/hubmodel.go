package stacklab

// Model ingredients: a recipe names a model as hub:<org>/<name>@<rev>. Pantheon
// resolves it against the Apollo Stack Lab hub (GET /hub/v1/models/{org}/{name}
// ?rev=R, which serves exactly snapshot R or 404, never another revision). Any
// doubt refuses: a malformed or unpinned ref, an unknown revision, a hub that
// answers with a different repo or revision, a listing without sha256s, or no
// hub reachable at all. There is no fallback to Hugging Face or to "current".

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Same id and revision grammar as the hub (router fleet.go reRepo, checkout.go reRev).
var (
	reHubRepo = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	reHubRev  = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9_.-]{0,127}$`)
	reSHA256  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// HubFile and HubModel mirror the hub's remoteFile/remoteRepo listing.
type HubFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type HubModel struct {
	Repo     string    `json:"repo"`
	Revision string    `json:"revision"`
	Files    []HubFile `json:"files"`
	Source   string    `json:"source,omitempty"` // the hub URL that answered
}

// ParseHubRef splits hub:<org>/<name>@<rev>. The revision is required: an
// unpinned model is not an admissible ingredient.
func ParseHubRef(ref string) (repo, rev string, err error) {
	body, ok := strings.CutPrefix(ref, "hub:")
	if !ok {
		return "", "", fmt.Errorf("model ingredient %q is not a hub: reference", ref)
	}
	repo, rev, ok = strings.Cut(body, "@")
	if !ok || !reHubRepo.MatchString(repo) || !reHubRev.MatchString(rev) || strings.Contains(repo, "..") {
		return "", "", fmt.Errorf("model ingredient %q must be hub:<org>/<name>@<rev>", ref)
	}
	return repo, rev, nil
}

// HubClient talks to the hub with this Mac's Stack Lab account.
type HubClient struct {
	URLs  []string
	Token string
	HTTP  *http.Client
}

// HubAccountPath is the account the Apollo router issues (0600, machine/token/hub_urls).
func HubAccountPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".sirsi", "stacklab", "account.json")
}

// LoadHubClient reads the Stack Lab account. A missing or incomplete account is
// an error: without it no model can be verified, so nothing is admitted.
func LoadHubClient(path string) (*HubClient, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("Stack Lab account: %w", err)
	}
	var a struct {
		Token   string   `json:"token"`
		HubURLs []string `json:"hub_urls"`
	}
	if err := json.Unmarshal(raw, &a); err != nil || a.Token == "" || len(a.HubURLs) == 0 {
		return nil, fmt.Errorf("Stack Lab account %s has no token or hub_urls", path)
	}
	return &HubClient{URLs: a.HubURLs, Token: a.Token, HTTP: &http.Client{Timeout: 90 * time.Second}}, nil
}

// Resolve returns the exact listing for ref. Only a transport failure moves on
// to the next hub URL; any HTTP answer from a reachable hub is final.
func (c *HubClient) Resolve(ctx context.Context, ref string) (HubModel, error) {
	repo, rev, err := ParseHubRef(ref)
	if err != nil {
		return HubModel{}, err
	}
	if c == nil || len(c.URLs) == 0 {
		return HubModel{}, fmt.Errorf("%s: no Stack Lab hub configured", ref)
	}
	org, name, _ := strings.Cut(repo, "/")
	path := "/hub/v1/models/" + url.PathEscape(org) + "/" + url.PathEscape(name) + "?rev=" + url.QueryEscape(rev)
	var last error
	for _, base := range c.URLs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
		if err != nil {
			return HubModel{}, err
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			last = err
			continue
		}
		m, err := decodeHubListing(resp, repo, rev)
		if err != nil {
			return HubModel{}, fmt.Errorf("%s: %w", ref, err)
		}
		m.Source = base
		return m, nil
	}
	return HubModel{}, fmt.Errorf("%s: no Stack Lab hub answered: %v", ref, last)
}

func decodeHubListing(resp *http.Response, repo, rev string) (HubModel, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return HubModel{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return HubModel{}, fmt.Errorf("hub refused (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var m HubModel
	if err := json.Unmarshal(body, &m); err != nil {
		return HubModel{}, fmt.Errorf("hub listing: %w", err)
	}
	if m.Repo != repo || m.Revision != rev {
		return HubModel{}, fmt.Errorf("hub answered %s@%s, not the pinned %s@%s", m.Repo, m.Revision, repo, rev)
	}
	if len(m.Files) == 0 {
		return HubModel{}, errors.New("hub listing has no files")
	}
	for _, f := range m.Files {
		if f.Name == "" || !reSHA256.MatchString(f.SHA256) {
			return HubModel{}, fmt.Errorf("hub listing file %q has no sha256", f.Name)
		}
	}
	return m, nil
}

// AdmitModelIngredients is the Ma'at gate: every ref must resolve exactly, or
// the whole set is refused.
func AdmitModelIngredients(ctx context.Context, c *HubClient, refs []string) ([]HubModel, error) {
	out := make([]HubModel, 0, len(refs))
	for _, ref := range refs {
		m, err := c.Resolve(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("Ma'at refuses model ingredient: %w", err)
		}
		out = append(out, m)
	}
	return out, nil
}
