package stacklab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
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
type GHRemoteReader struct{}

func (GHRemoteReader) ReadFile(repo, path, ref string) ([]byte, bool, error) {
	cmd := exec.Command("gh", "api",
		"-H", "Accept: application/vnd.github.raw",
		fmt.Sprintf("repos/%s/contents/%s?ref=%s", repo, path, ref))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "404") {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("gh api read %s@%s:%s: %w (%s)", repo, ref, path, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), true, nil
}

func (GHRemoteReader) ListDir(repo, path, ref string) ([]string, bool, error) {
	cmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/contents/%s?ref=%s", repo, path, ref))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "404") {
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

func (GHRemoteReader) ListBranches(repo string) ([]string, error) {
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
