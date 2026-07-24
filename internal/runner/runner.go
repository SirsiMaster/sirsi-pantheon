// Package runner productizes the self-hosted GitHub Actions runner install
// (ADR-042/ADR-044): the proven ~/.sirsi/actions-runner/install-runner.sh
// pattern as a Go verb, so `sirsi runner install <repo>` registers this Mac
// as a repo's build worker and `sirsi runner status` reports the fleet.
//
// The shell seed encoded two hard-won gotchas this port preserves exactly:
//  1. the donor copy must strip .runner_migrated — stale instance state makes
//     config.sh believe it is already configured and silently skip setup;
//  2. svc.sh requires ./runsvc.sh beside it — copy bin/runsvc.sh up before
//     `svc.sh install` or the launchd service install fails.
package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// DefaultOwner is assumed when `install` gets a bare repo name.
	DefaultOwner = "SirsiMaster"
	// RunnerName is the fleet-wide runner identity on this machine (instance 1).
	RunnerName = "m5-sirsi"
	// labels advertised to GitHub; workflows target `self-hosted`.
	labels = "self-hosted,macOS,ARM64,m5"
	// donorDir under Base holds the proven runner software we clone per repo.
	donorDir = "sirsi-pantheon"
)

// Base returns the actions-runner root (~/.sirsi/actions-runner).
func Base() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".sirsi", "actions-runner")
}

// ParseRepoArg splits "owner/repo" and defaults a bare "repo" to DefaultOwner.
func ParseRepoArg(arg string) (owner, repo string) {
	if i := strings.IndexByte(arg, '/'); i >= 0 {
		return arg[:i], arg[i+1:]
	}
	return DefaultOwner, arg
}

// instanceSuffix is "" for the first runner and "-N" for the Nth (N>=2), so a
// second runner on the same repo — the way to parallelize a serialized CI
// queue (owner directive 2026-07-24: add a second m5 runner) — gets a distinct
// dir and GitHub runner name without colliding with the first. Instance 1 is
// suffix-free, so the single-runner path is byte-identical to before.
func instanceSuffix(instance int) string {
	if instance <= 1 {
		return ""
	}
	return fmt.Sprintf("-%d", instance)
}

// instanceDir is the per-instance donor-clone directory under Base().
func instanceDir(repo string, instance int) string {
	return filepath.Join(Base(), repo+instanceSuffix(instance))
}

// instanceRunnerName is the GitHub runner name for this instance.
func instanceRunnerName(instance int) string {
	return RunnerName + instanceSuffix(instance)
}

// runnerFile is the subset of .runner (written by config.sh) we read back.
type runnerFile struct {
	GitHubURL string `json:"gitHubUrl"`
	AgentName string `json:"agentName"`
}

// ParseRunnerFile extracts "owner/repo" and the agent name from a .runner
// file's bytes. gitHubUrl looks like https://github.com/Owner/Repo. The
// actions-runner writes the file with a UTF-8 BOM, which Go's JSON decoder
// rejects — strip it or every real fleet file silently fails to parse.
func ParseRunnerFile(data []byte) (ownerRepo, agentName string, err error) {
	data = []byte(strings.TrimSpace(strings.TrimPrefix(string(data), "\uFEFF")))
	var rf runnerFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return "", "", fmt.Errorf("parse .runner: %w", err)
	}
	u := strings.TrimSuffix(rf.GitHubURL, "/")
	const host = "github.com/"
	i := strings.Index(u, host)
	if i < 0 {
		return "", "", fmt.Errorf(".runner gitHubUrl %q has no github.com path", rf.GitHubURL)
	}
	return u[i+len(host):], rf.AgentName, nil
}

// APIRunner is one runner row from GET repos/{owner}/{repo}/actions/runners.
type APIRunner struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Busy   bool   `json:"busy"`
}

// ClassifyStatus grades a locally-installed runner against the repo's API
// list: online/offline pass through; a missing row is "unregistered" (the
// local dir has state but GitHub no longer knows the runner).
func ClassifyStatus(rows []APIRunner, name string) (status string, busy bool) {
	for _, r := range rows {
		if r.Name == name {
			return r.Status, r.Busy
		}
	}
	return "unregistered", false
}

// StatusRow is one fleet entry — field names are a published contract
// (the board producer consumes this JSON shape verbatim).
type StatusRow struct {
	Repo   string `json:"repo"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Busy   bool   `json:"busy"`
}

// InstalledDirs lists Base() subdirectories holding a configured runner
// (a .runner file), sorted by name.
func InstalledDirs(base string) ([]string, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(base, e.Name(), ".runner")); err == nil {
			dirs = append(dirs, filepath.Join(base, e.Name()))
		}
	}
	return dirs, nil
}

// CurrentGitHubRepo returns "owner/repo" for the cwd's origin remote, or ""
// when the cwd isn't a git repo / origin isn't GitHub. Handles both SSH
// (git@github.com:o/r.git) and HTTPS (https://github.com/o/r.git) forms.
func CurrentGitHubRepo() string {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return ParseGitHubRemote(strings.TrimSpace(string(out)))
}

// ParseGitHubRemote extracts "owner/repo" from a GitHub remote URL ("" if not
// GitHub). Pure — the testable half of CurrentGitHubRepo.
func ParseGitHubRemote(url string) string {
	url = strings.TrimSuffix(url, ".git")
	for _, prefix := range []string{"git@github.com:", "https://github.com/", "http://github.com/", "ssh://git@github.com/"} {
		if rest, ok := strings.CutPrefix(url, prefix); ok {
			if strings.Count(rest, "/") == 1 {
				return rest
			}
			return ""
		}
	}
	return ""
}

// gh shells the gh CLI (which owns GitHub auth — we never reimplement it),
// capturing stdout.
func gh(args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

// RepoRunners fetches the repo's registered runners via gh.
func RepoRunners(ownerRepo string) ([]APIRunner, error) {
	out, err := gh("api", "repos/"+ownerRepo+"/actions/runners", "-q", "{\"runners\": .runners}")
	if err != nil {
		return nil, fmt.Errorf("runners API for %s: %w", ownerRepo, err)
	}
	var wrap struct {
		Runners []APIRunner `json:"runners"`
	}
	if err := json.Unmarshal(out, &wrap); err != nil {
		return nil, fmt.Errorf("decode runners for %s: %w", ownerRepo, err)
	}
	return wrap.Runners, nil
}

// Status reports every locally-installed runner graded against GitHub.
func Status() ([]StatusRow, error) {
	dirs, err := InstalledDirs(Base())
	if err != nil {
		return nil, err
	}
	var rows []StatusRow
	for _, d := range dirs {
		data, err := os.ReadFile(filepath.Join(d, ".runner"))
		if err != nil {
			rows = append(rows, StatusRow{Repo: filepath.Base(d), Status: "unreadable"})
			continue
		}
		ownerRepo, name, err := ParseRunnerFile(data)
		if err != nil || ownerRepo == "" {
			// Surface, don't skip — a silently-dropped dir hid the BOM bug.
			rows = append(rows, StatusRow{Repo: filepath.Base(d), Status: "unreadable"})
			continue
		}
		apiRows, err := RepoRunners(ownerRepo)
		if err != nil {
			rows = append(rows, StatusRow{Repo: ownerRepo, Name: name, Status: "unreachable"})
			continue
		}
		st, busy := ClassifyStatus(apiRows, name)
		rows = append(rows, StatusRow{Repo: ownerRepo, Name: name, Status: st, Busy: busy})
	}
	return rows, nil
}

// run executes a command in dir, surfacing stderr; stdout is discarded
// (config.sh/svc.sh chatter) unless the caller needs it.
func run(dir, bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Install registers this Mac as a runner for owner/repo: clone the donor
// runner software (stripping ALL instance state), fetch a registration token
// via gh, configure, install the launchd service, start it, then poll the
// runners API until the runner reports online (or ~60s passes).
func Install(owner, repo string, progress func(string)) error {
	return InstallInstance(owner, repo, 1, progress)
}

// InstallInstance installs the Nth runner for owner/repo. instance 1 is the
// canonical single runner (name m5-sirsi, dir <repo>); instance >=2 adds a
// parallel runner (name m5-sirsi-N, dir <repo>-N) to widen a serialized CI
// queue. Every instance clones the SAME donor software and is otherwise
// identical — labels, launchd service shape, online-gated success.
func InstallInstance(owner, repo string, instance int, progress func(string)) error {
	if progress == nil {
		progress = func(string) {}
	}
	base := Base()
	src := filepath.Join(base, donorDir)
	dst := instanceDir(repo, instance)
	name := instanceRunnerName(instance)
	ownerRepo := owner + "/" + repo
	tag := repo + instanceSuffix(instance)

	if _, err := os.Stat(filepath.Join(dst, ".runner")); err == nil {
		progress(fmt.Sprintf("[%s] already configured", tag))
		return nil
	}
	if _, err := os.Stat(filepath.Join(src, "config.sh")); err != nil {
		return fmt.Errorf("donor runner software missing at %s — install a runner there once by hand first", src)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}

	// Gotcha 1: strip ALL instance state — .runner_migrated especially, which
	// makes config.sh believe the copy is already configured and skip setup.
	progress(fmt.Sprintf("[%s] copying runner software", tag))
	if err := run("", "rsync", "-a",
		"--exclude", ".runner", "--exclude", ".credentials*", "--exclude", ".env",
		"--exclude", ".path", "--exclude", ".service", "--exclude", ".runner_migrated",
		"--exclude", "_work", "--exclude", "_diag", "--exclude", "runsvc.sh",
		src+"/", dst+"/"); err != nil {
		return fmt.Errorf("copy runner software: %w", err)
	}
	// Gotcha 2: svc.sh needs bin/runsvc.sh present, then a copy beside it.
	if _, err := os.Stat(filepath.Join(dst, "bin", "runsvc.sh")); err != nil {
		if err := run("", "cp", filepath.Join(src, "bin", "runsvc.sh"), filepath.Join(dst, "bin", "runsvc.sh")); err != nil {
			return fmt.Errorf("stage bin/runsvc.sh: %w", err)
		}
	}

	progress(fmt.Sprintf("[%s] requesting registration token", tag))
	tok, err := gh("api", "-X", "POST", "repos/"+ownerRepo+"/actions/runners/registration-token", "-q", ".token")
	if err != nil {
		return fmt.Errorf("registration token for %s: %w", ownerRepo, err)
	}
	token := strings.TrimSpace(string(tok))
	if token == "" {
		return fmt.Errorf("empty registration token for %s", ownerRepo)
	}

	progress(fmt.Sprintf("[%s] configuring runner %s", tag, name))
	if err := run(dst, "./config.sh",
		"--url", "https://github.com/"+ownerRepo, "--token", token,
		"--name", name, "--labels", labels, "--work", "_work", "--unattended"); err != nil {
		return fmt.Errorf("config.sh for %s: %w", ownerRepo, err)
	}
	if err := run(dst, "cp", "./bin/runsvc.sh", "./runsvc.sh"); err != nil {
		return fmt.Errorf("stage runsvc.sh: %w", err)
	}
	_ = run(dst, "chmod", "+x", "./runsvc.sh")

	progress(fmt.Sprintf("[%s] installing launchd service", tag))
	_ = run(dst, "./svc.sh", "install") // idempotent-noisy; start is the real gate
	if err := run(dst, "./svc.sh", "start"); err != nil {
		return fmt.Errorf("svc.sh start for %s: %w", ownerRepo, err)
	}

	progress(fmt.Sprintf("[%s] waiting for runner to come online", tag))
	deadline := time.Now().Add(60 * time.Second)
	for {
		rows, err := RepoRunners(ownerRepo)
		if err == nil {
			if st, _ := ClassifyStatus(rows, name); st == "online" {
				progress(fmt.Sprintf("[%s] runner %s: online", tag, name))
				return nil
			}
		}
		if time.Now().After(deadline) {
			st := "not found"
			if rows, err := RepoRunners(ownerRepo); err == nil {
				st, _ = ClassifyStatus(rows, name)
			}
			return fmt.Errorf("[%s] runner %s did not come online within 60s (last status: %s)", tag, name, st)
		}
		time.Sleep(3 * time.Second)
	}
}
