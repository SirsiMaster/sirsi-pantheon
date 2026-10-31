package stacklab

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// MissingGitHubCLIError identifies the one local dependency Stack Lab needs
// before it can read canonical origin and registry records.
type MissingGitHubCLIError struct{}

func (MissingGitHubCLIError) Error() string {
	return "GitHub CLI (gh) is unavailable; install it with Homebrew (brew install gh) or add it to PATH"
}

// GHRemoteReader reads through the installed `gh` CLI. Executable lookup is
// per reader invocation and does not modify the process PATH.
type GHRemoteReader struct {
	mu      sync.Mutex
	probed  map[string]bool // repo -> readable, memoized per run
	resolve func() (string, error)
	run     func(binary string, args ...string) ([]byte, []byte, error)
}

// NewGHRemoteReader returns a ready-to-use GHRemoteReader.
func NewGHRemoteReader() *GHRemoteReader {
	return newGHRemoteReader(resolveGitHubCLI, runGitHubCommand)
}

func newGHRemoteReader(resolve func() (string, error), run func(string, ...string) ([]byte, []byte, error)) *GHRemoteReader {
	return &GHRemoteReader{probed: map[string]bool{}, resolve: resolve, run: run}
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

	_, err := g.execute("api", fmt.Sprintf("repos/%s", repo))
	ok := err == nil

	g.mu.Lock()
	g.probed[repo] = ok
	g.mu.Unlock()

	if err != nil {
		return false, fmt.Errorf("repo %s: %w", repo, err)
	}
	return true, nil
}

func (g *GHRemoteReader) execute(args ...string) ([]byte, error) {
	binary, err := g.resolve()
	if err != nil {
		return nil, err
	}
	stdout, stderr, err := g.run(binary, args...)
	if err != nil {
		return stdout, &ghCommandError{cause: err, stderr: string(stderr)}
	}
	return stdout, nil
}

func (g *GHRemoteReader) ReadFile(repo, path, ref string) ([]byte, bool, error) {
	out, err := g.execute("api",
		"-H", "Accept: application/vnd.github.raw",
		fmt.Sprintf("repos/%s/contents/%s?ref=%s", repo, path, ref))
	if err != nil {
		if isGHNotFound(err) {
			if readable, rerr := g.repoReadable(repo); rerr != nil || !readable {
				if rerr == nil {
					rerr = fmt.Errorf("repo %s: not readable", repo)
				}
				return nil, false, fmt.Errorf("gh api read %s@%s:%s: repo unreadable, cannot distinguish from a real path-404: %w", repo, ref, path, rerr)
			}
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("gh api read %s@%s:%s: %w", repo, ref, path, err)
	}
	return out, true, nil
}

func (g *GHRemoteReader) ListDir(repo, path, ref string) ([]string, bool, error) {
	out, err := g.execute("api", fmt.Sprintf("repos/%s/contents/%s?ref=%s", repo, path, ref))
	if err != nil {
		if isGHNotFound(err) {
			if readable, rerr := g.repoReadable(repo); rerr != nil || !readable {
				if rerr == nil {
					rerr = fmt.Errorf("repo %s: not readable", repo)
				}
				return nil, false, fmt.Errorf("gh api list %s@%s:%s: repo unreadable, cannot distinguish from a real path-404: %w", repo, ref, path, rerr)
			}
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("gh api list %s@%s:%s: %w", repo, ref, path, err)
	}
	var rows []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
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
	out, err := g.execute("api", "--paginate", fmt.Sprintf("repos/%s/branches", repo), "--jq", ".[].name")
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

type ghCommandError struct {
	cause  error
	stderr string
}

func (e *ghCommandError) Error() string { return e.cause.Error() }
func (e *ghCommandError) Unwrap() error { return e.cause }

func isGHNotFound(err error) bool {
	var commandErr *ghCommandError
	return errors.As(err, &commandErr) && strings.Contains(commandErr.stderr, "404")
}

func runGitHubCommand(binary string, args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func resolveGitHubCLI() (string, error) {
	fallbacks := []string(nil)
	if runtime.GOOS == "darwin" {
		fallbacks = []string{"/opt/homebrew/bin/gh", "/usr/local/bin/gh"}
	}
	return resolveGitHubCLIFrom(os.Getenv("PATH"), runtime.GOOS, fallbacks)
}

func resolveGitHubCLIFrom(pathEnv, goos string, macOSFallbacks []string) (string, error) {
	candidates := filepath.SplitList(pathEnv)
	absoluteCandidates := make([]string, 0, len(candidates)+len(macOSFallbacks))
	for _, dir := range candidates {
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		absoluteCandidates = append(absoluteCandidates, filepath.Join(dir, "gh"))
	}
	if goos == "darwin" {
		absoluteCandidates = append(absoluteCandidates, macOSFallbacks...)
	}
	for _, candidate := range absoluteCandidates {
		resolved, err := canonicalExecutable(candidate)
		if err == nil {
			return resolved, nil
		}
	}
	return "", MissingGitHubCLIError{}
}

func canonicalExecutable(candidate string) (string, error) {
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("not an executable regular file")
	}
	return resolved, nil
}
