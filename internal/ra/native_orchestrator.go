package ra

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ErrExternalProviderUnavailable is returned for task and broadcast. Those
// operations intentionally require a separately configured developer provider;
// Ra never substitutes a local command or a Python SDK for one.
var ErrExternalProviderUnavailable = errors.New("external Ra provider is not configured")

const ExternalProviderEnv = "SIRSI_RA_PROVIDER_EXECUTABLE"

// ExternalProvider is an explicitly configured developer-only executable.
// Pantheon invokes it directly, never through a shell or Python interpreter.
type ExternalProvider struct {
	Executable string
}

// ExternalProviderFromEnv resolves the opt-in provider boundary. The default
// product has no provider and therefore fails closed for task and broadcast.
func ExternalProviderFromEnv() (*ExternalProvider, error) {
	path := strings.TrimSpace(os.Getenv(ExternalProviderEnv))
	if path == "" {
		return nil, ErrExternalProviderUnavailable
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("external Ra provider path must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("external Ra provider: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return nil, fmt.Errorf("external Ra provider must be an executable regular file")
	}
	return &ExternalProvider{Executable: path}, nil
}

// Run invokes the provider directly with the stable operation/argument
// contract. It is used only for task and broadcast after explicit opt-in.
func (p *ExternalProvider) Run(ctx context.Context, operation string, args []string) ([]NativeResult, error) {
	if p == nil || p.Executable == "" {
		return nil, ErrExternalProviderUnavailable
	}
	if operation != "task" && operation != "broadcast" {
		return nil, fmt.Errorf("external Ra provider cannot run %q", operation)
	}
	start := time.Now()
	out, err := nativeRun(ctx, "", append([]string{p.Executable, operation}, args...)...)
	status := "pass"
	if err != nil {
		status = "fail"
	}
	result := NativeResult{Repo: "external-provider", Operation: operation, Status: status, Output: string(out), Duration: time.Since(start)}
	if err != nil {
		return []NativeResult{result}, fmt.Errorf("external Ra provider %s: %w", operation, err)
	}
	return []NativeResult{result}, nil
}

// RunNativeFleetWithProvider preserves native fleet operations and delegates
// provider operations only through the explicit developer-only boundary.
func RunNativeFleetWithProvider(ctx context.Context, repos []NativeRepo, operation string, args []string, provider *ExternalProvider) ([]NativeResult, error) {
	if operation == "task" || operation == "broadcast" {
		return provider.Run(ctx, operation, args)
	}
	return RunNativeFleet(ctx, repos, operation)
}

// Command is one explicit process invocation. Dir is relative to the owning
// repository when non-empty. Ra does not invoke a shell for fleet checks.
type Command struct {
	Dir  string
	Args []string
}

// NativeRepo is the Go-owned, non-provider fleet contract.
type NativeRepo struct {
	Name, Path, Description string
	Health, Test, Lint      []Command
}

type NativeResult struct {
	Repo, Operation, Status, Output string
	Duration                        time.Duration
}

var nativeRun = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

// DefaultFleetRepos preserves the former Python orchestrator's exact repository
// and command intent, using explicit Go-owned command steps.
func DefaultFleetRepos(home string) []NativeRepo {
	dev := filepath.Join(home, "Development")
	npm := func(args ...string) []Command { return []Command{{Args: append([]string{"npm"}, args...)}} }
	return []NativeRepo{
		{Name: "pantheon", Path: filepath.Join(dev, "sirsi-pantheon"), Description: "Infrastructure hygiene CLI",
			Health: []Command{{Args: []string{"git", "status", "--short"}}, {Args: []string{"git", "log", "--oneline", "-3"}}, {Args: []string{"go", "build", "./cmd/pantheon/"}}},
			Test:   []Command{{Args: []string{"go", "test", "-short", "./..."}}}, Lint: []Command{{Args: []string{"gofmt", "-l", "./internal/", "./cmd/"}}}},
		{Name: "nexus", Path: filepath.Join(dev, "SirsiNexusApp"), Description: "Platform monorepo",
			Health: []Command{{Args: []string{"git", "status", "--short"}}, {Args: []string{"git", "log", "--oneline", "-3"}}, {Dir: "packages/sirsi-portal-app", Args: []string{"npx", "vite", "build"}}},
			Test:   []Command{{Dir: "ui", Args: []string{"yarn", "test", "--passWithNoTests"}}, {Dir: "packages/sirsi-portal-app", Args: []string{"npx", "tsc", "--noEmit"}}},
			Lint:   []Command{{Dir: "ui", Args: []string{"yarn", "lint"}}, {Dir: "packages/sirsi-portal-app", Args: []string{"npx", "eslint", ".", "--max-warnings", "999"}}}},
		{Name: "finalwishes", Path: filepath.Join(dev, "FinalWishes"), Description: "Estate planning application",
			Health: []Command{{Args: []string{"git", "status", "--short"}}, {Args: []string{"git", "log", "--oneline", "-3"}}, {Args: []string{"npm", "run", "build", "--if-present"}}},
			Test:   npm("test", "--if-present"), Lint: npm("run", "lint", "--if-present")},
		{Name: "assiduous", Path: filepath.Join(dev, "Assiduous"), Description: "Real estate platform",
			Health: []Command{{Args: []string{"git", "status", "--short"}}, {Args: []string{"git", "log", "--oneline", "-3"}}, {Args: []string{"npm", "run", "build", "--if-present"}}},
			Test:   npm("test", "--if-present"), Lint: npm("run", "lint", "--if-present")},
	}
}

// RunNativeFleet executes health, test, lint, or nightly with no Python or
// agent SDK. Repositories run concurrently; commands inside each repository
// remain ordered, matching the former health and Nexus multi-step checks.
func RunNativeFleet(ctx context.Context, repos []NativeRepo, operation string) ([]NativeResult, error) {
	if operation == "task" || operation == "broadcast" {
		return nil, fmt.Errorf("%w: %s", ErrExternalProviderUnavailable, operation)
	}
	if operation == "nightly" {
		var all []NativeResult
		for _, phase := range []string{"health", "lint", "test"} {
			results, err := RunNativeFleet(ctx, repos, phase)
			all = append(all, results...)
			if err != nil {
				return all, err
			}
		}
		return all, nil
	}
	if operation != "health" && operation != "test" && operation != "lint" {
		return nil, fmt.Errorf("native Ra does not execute provider operation %q", operation)
	}
	results := make([]NativeResult, len(repos))
	var wg sync.WaitGroup
	for i, repo := range repos {
		wg.Add(1)
		go func(i int, repo NativeRepo) {
			defer wg.Done()
			start := time.Now()
			result := NativeResult{Repo: repo.Name, Operation: operation, Status: "pass"}
			if _, err := os.Stat(repo.Path); err != nil {
				result.Status, result.Output = "skip", fmt.Sprintf("directory unavailable: %s", repo.Path)
				result.Duration = time.Since(start)
				results[i] = result
				return
			}
			steps := repo.Health
			if operation == "test" {
				steps = repo.Test
			}
			if operation == "lint" {
				steps = repo.Lint
			}
			if len(steps) == 0 {
				result.Status, result.Output = "skip", "no configured command"
			} else {
				var output strings.Builder
				for _, step := range steps {
					if len(step.Args) == 0 {
						result.Status, result.Output = "fail", "invalid empty command"
						break
					}
					dir := repo.Path
					if step.Dir != "" {
						dir = filepath.Join(dir, step.Dir)
					}
					out, err := nativeRun(ctx, dir, step.Args...)
					fmt.Fprintf(&output, "$ %s\n%s", strings.Join(step.Args, " "), out)
					if err != nil {
						result.Status = "fail"
						break
					}
				}
				result.Output = output.String()
			}
			result.Duration = time.Since(start)
			results[i] = result
		}(i, repo)
	}
	wg.Wait()
	return results, nil
}
