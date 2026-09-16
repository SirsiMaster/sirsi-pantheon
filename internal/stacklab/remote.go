package stacklab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// RemoteReader reads repository content by ref, never a working tree or
// mirror (A35). Doctor.go depends only on this interface so tests can mock
// it — no network in `go test`.
type RemoteReader interface {
	// ReadFile returns the raw bytes of repo's path at ref. exists=false,
	// err=nil means "not found at that ref" (not an error condition).
	ReadFile(repo, path, ref string) (content []byte, exists bool, err error)
	// ListDir lists directory entry names (files only) of repo's path at ref.
	// exists=false, err=nil means the directory itself does not exist.
	ListDir(repo, path, ref string) (entries []string, exists bool, err error)
	// ListBranches lists all branch names of repo.
	ListBranches(repo string) ([]string, error)
}

// GHRemoteReader reads via the `gh` CLI (assumed authenticated — matches the
// existing cmd/sirsi/routerresolved.go pattern of shelling out to `gh api`).
//
// GitHub returns 404 for a path that genuinely doesn't exist at a ref AND
// for a repo the caller cannot see at all (private + missing/insufficient
// auth, or a wrong LaneRepoMap entry) — the two are indistinguishable from
// the contents-endpoint response alone. Reporting the second case as a
// clean "not found" would make an unauthenticated run confidently claim
// every mapped wing is stranded/unbuilt: a false finding, the exact A35
// failure this doctor exists to prevent. So on any 404, repoReadable probes
// the repo itself once (cached per repo for the run) to tell "real
// path-404" from "repo unreadable"; the latter is surfaced as an error so
// the caller records it as Unknown, never as a finding.
type GHRemoteReader struct {
	mu     sync.Mutex
	probed map[string]bool // repo -> readable, memoized per run
}

// NewGHRemoteReader returns a ready-to-use GHRemoteReader.
func NewGHRemoteReader() *GHRemoteReader {
	return &GHRemoteReader{probed: map[string]bool{}}
}

// repoReadable reports whether repo itself is readable via `gh api
// repos/<repo>`, caching the result for the lifetime of this reader.
func (g *GHRemoteReader) repoReadable(repo string) (bool, error) {
	g.mu.Lock()
	if ok, cached := g.probed[repo]; cached {
		g.mu.Unlock()
		return ok, nil
	}
	g.mu.Unlock()

	var stderr bytes.Buffer
	cmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s", repo))
	cmd.Stderr = &stderr
	err := cmd.Run()
	ok := err == nil

	g.mu.Lock()
	g.probed[repo] = ok
	g.mu.Unlock()

	if !ok {
		return false, fmt.Errorf("repo %s: %w (%s)", repo, err, strings.TrimSpace(stderr.String()))
	}
	return true, nil
}

func (g *GHRemoteReader) ReadFile(repo, path, ref string) ([]byte, bool, error) {
	cmd := exec.Command("gh", "api",
		"-H", "Accept: application/vnd.github.raw",
		fmt.Sprintf("repos/%s/contents/%s?ref=%s", repo, path, ref))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "404") {
			if readable, rerr := g.repoReadable(repo); rerr != nil || !readable {
				if rerr == nil {
					rerr = fmt.Errorf("repo %s: not readable", repo)
				}
				return nil, false, fmt.Errorf("gh api read %s@%s:%s: repo unreadable, cannot distinguish from a real path-404: %w", repo, ref, path, rerr)
			}
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("gh api read %s@%s:%s: %w (%s)", repo, ref, path, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), true, nil
}

func (g *GHRemoteReader) ListDir(repo, path, ref string) ([]string, bool, error) {
	cmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/contents/%s?ref=%s", repo, path, ref))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "404") {
			if readable, rerr := g.repoReadable(repo); rerr != nil || !readable {
				if rerr == nil {
					rerr = fmt.Errorf("repo %s: not readable", repo)
				}
				return nil, false, fmt.Errorf("gh api list %s@%s:%s: repo unreadable, cannot distinguish from a real path-404: %w", repo, ref, path, rerr)
			}
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("gh api list %s@%s:%s: %w (%s)", repo, ref, path, err, strings.TrimSpace(stderr.String()))
	}
	var rows []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		return nil, false, fmt.Errorf("parse dir listing %s@%s:%s: %w", repo, ref, path, err)
	}
	var names []string
	for _, r := range rows {
		if r.Type == "file" {
			names = append(names, r.Name)
		}
	}
	return names, true, nil
}

func (g *GHRemoteReader) ListBranches(repo string) ([]string, error) {
	out, err := exec.Command("gh", "api", "--paginate", fmt.Sprintf("repos/%s/branches", repo), "--jq", ".[].name").Output()
	if err != nil {
		return nil, fmt.Errorf("gh api list branches %s: %w", repo, err)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}
