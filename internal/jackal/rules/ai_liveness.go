package rules

// AI cache rules wrap the standard baseScanRule with a liveness guard:
//
//  1. minAgeDays (30 days) on the baseScanRule itself drops any path whose
//     directory mtime indicates a recent write — covers the common case of
//     a model just downloaded or actively in use.
//  2. envGuardedRule additionally suppresses findings when a known runtime
//     env var (HF_HOME, OLLAMA_MODELS, TORCH_HOME, etc.) points into the
//     scanned path. The env var presence is treated as an authoritative
//     "this path is the active runtime cache" signal — the rule emits no
//     finding for it.
//  3. livePathFns: optional per-rule path-provider functions called at scan
//     time. Used by NewHuggingFaceCacheRule to read ~/.sirsi/gemma-model.conf
//     and translate the configured model ID to its HuggingFace hub path, so
//     the running inference substrate is never classified as reclaimable cache
//     — even if the model directory mtime is older than 30 days.
//
// All guards are exclusion-shaped: if the path is live, the scan returns
// no finding for it at all. Not "demoted to caution" — gone from the result
// set entirely. "If it isn't waste, it must not be reported as waste."

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
)

// envGuardedRule wraps a baseScanRule and filters out findings whose path
// is the value (or a prefix/suffix) of any of the named env vars or live
// paths returned by livePathFns.
type envGuardedRule struct {
	*baseScanRule
	envVars     []string
	livePathFns []func(homeDir string) []string
}

func (r *envGuardedRule) Scan(ctx context.Context, opts jackal.ScanOptions) ([]jackal.Finding, error) {
	findings, err := r.baseScanRule.Scan(ctx, opts)
	if err != nil || len(findings) == 0 {
		return findings, err
	}

	homeDir := opts.HomeDir
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}

	live := liveTargetsFromEnv(r.envVars, getEnv)
	for _, fn := range r.livePathFns {
		live = append(live, fn(homeDir)...)
	}
	if len(live) == 0 {
		return findings, nil
	}

	out := findings[:0]
	for _, f := range findings {
		if isLiveTarget(f.Path, live) {
			continue
		}
		out = append(out, f)
	}
	// The slice was reused in-place — copy to avoid surprising upstream readers.
	result := make([]jackal.Finding, len(out))
	copy(result, out)
	return result, nil
}

// getEnv is the env-var lookup seam (overridable in tests).
var getEnv = os.Getenv

// readFileLines is the file-read seam (overridable in tests).
var readFileLines = func(path string) ([]string, error) {
	f, err := os.Open(path) //nolint:gosec // path comes from trusted Sirsi config dir
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			lines = append(lines, line)
		}
	}
	return lines, sc.Err()
}

// sirsiGemmaLivePaths reads the Sirsi gemma model conf files and returns the
// HuggingFace hub snapshot directory for each configured model. These paths are
// treated as live targets by the HuggingFace cache rule so that the active
// inference substrate is never classified as reclaimable cache.
//
// Each conf file holds one model ID per non-blank line, e.g.:
//
//	mlx-community/gemma-4-12B-it-qat-mxfp8
//
// The HuggingFace hub stores this at:
//
//	~/.cache/huggingface/hub/models--mlx-community--gemma-4-12B-it-qat-mxfp8/
func sirsiGemmaLivePaths(homeDir string) []string {
	confs := []string{
		filepath.Join(homeDir, ".sirsi", "gemma-model.conf"),
		filepath.Join(homeDir, ".sirsi", "gemma-model-max.conf"),
	}
	hubRoot := filepath.Join(homeDir, ".cache", "huggingface", "hub")
	seen := make(map[string]struct{})
	var out []string
	for _, conf := range confs {
		lines, err := readFileLines(conf)
		if err != nil {
			continue // conf absent or unreadable — not an error
		}
		for _, modelID := range lines {
			// mlx-community/gemma-4-12B-it-qat-mxfp8 → models--mlx-community--gemma-4-12B-it-qat-mxfp8
			dirName := "models--" + strings.ReplaceAll(modelID, "/", "--")
			p := filepath.Join(hubRoot, dirName)
			if _, ok := seen[p]; !ok {
				seen[p] = struct{}{}
				out = append(out, p)
			}
		}
	}
	return out
}

func liveTargetsFromEnv(vars []string, lookup func(string) string) []string {
	if len(vars) == 0 {
		return nil
	}
	// ExpandPath needs a real home dir to resolve a ~-prefixed env value
	// (e.g. HF_HOME=~/.cache/huggingface); with an empty home it would expand
	// to a RELATIVE path that never matches an absolute finding path, silently
	// defeating the guard. Mirror baseScanRule.Scan's os.UserHomeDir() fallback.
	homeDir, _ := os.UserHomeDir()
	var out []string
	seen := make(map[string]struct{})
	for _, v := range vars {
		val := strings.TrimSpace(lookup(v))
		if val == "" {
			continue
		}
		expanded := jackal.ExpandPath(val, homeDir)
		clean := filepath.Clean(expanded)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

// isLiveTarget reports whether candidate is, contains, or is contained by
// any of the live targets. Either direction protects the user:
//   - target inside candidate → candidate hosts the active runtime cache
//   - candidate inside target → the scanned path is a subtree of an env-pinned root
func isLiveTarget(candidate string, liveTargets []string) bool {
	c := filepath.Clean(candidate)
	for _, t := range liveTargets {
		if c == t {
			return true
		}
		if hasPrefixPath(c, t) || hasPrefixPath(t, c) {
			return true
		}
	}
	return false
}

func hasPrefixPath(child, parent string) bool {
	if parent == "" || child == "" {
		return false
	}
	if child == parent {
		return true
	}
	withSep := parent
	if !strings.HasSuffix(withSep, string(filepath.Separator)) {
		withSep += string(filepath.Separator)
	}
	return strings.HasPrefix(child, withSep)
}
