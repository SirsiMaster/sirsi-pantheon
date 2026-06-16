package router

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ServiceOptions describes the per-repo launchd service.
type ServiceOptions struct {
	RepoRoot   string
	BinaryPath string
	Label      string
	PlistPath  string
	LogPath    string
	ErrPath    string
	PathEnv    string
}

// DefaultServiceOptions builds the launchd paths for a repo-local autorouter.
func DefaultServiceOptions(repoRoot, binaryPath string) ServiceOptions {
	label := "com.sirsi.router." + serviceSlug(repoRoot)
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(repoRoot, ".agents", "idea-router", "logs")
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		pathEnv = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/Applications/Codex.app/Contents/Resources"
	}
	return ServiceOptions{
		RepoRoot:   repoRoot,
		BinaryPath: binaryPath,
		Label:      label,
		PlistPath:  filepath.Join(home, "Library", "LaunchAgents", label+".plist"),
		LogPath:    filepath.Join(logDir, "autorouter.out.log"),
		ErrPath:    filepath.Join(logDir, "autorouter.err.log"),
		PathEnv:    pathEnv,
	}
}

// ResolveStableBinary returns an executable path suitable for a long-lived
// launchd plist. When invoked through `go run`, os.Executable points into a
// temporary go-build directory that disappears after the command exits, so we
// build a repo-local binary for the service instead.
func ResolveStableBinary(repoRoot, candidate string) (string, error) {
	if !isGoRunBinary(candidate) {
		return candidate, nil
	}
	out := filepath.Join(repoRoot, ".agents", "idea-router", "bin", "sirsi")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", fmt.Errorf("create router bin dir: %w", err)
	}
	cmd := exec.Command("go", "build", "-o", out, "./cmd/sirsi")
	cmd.Dir = repoRoot
	combined, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build stable router binary: %w\n%s", err, string(combined))
	}
	return out, nil
}

// IsGoRunBinary reports whether a path is the temporary executable produced by
// `go run`. It is exported for status reporting and tests.
func IsGoRunBinary(path string) bool {
	return isGoRunBinary(path)
}

func isGoRunBinary(path string) bool {
	cleaned := filepath.Clean(path)
	if strings.Contains(cleaned, string(filepath.Separator)+"go-build") &&
		(strings.Contains(cleaned, string(filepath.Separator)+"exe"+string(filepath.Separator)) ||
			strings.HasSuffix(filepath.Dir(cleaned), "-d")) {
		return true
	}
	if gocache := os.Getenv("GOCACHE"); gocache != "" {
		if rel, err := filepath.Rel(filepath.Clean(gocache), cleaned); err == nil && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
			return true
		}
	}
	parent := filepath.Base(filepath.Dir(cleaned))
	grandparent := filepath.Base(filepath.Dir(filepath.Dir(cleaned)))
	return strings.HasSuffix(parent, "-d") && len(grandparent) == 2 && isHexPair(grandparent)
}

func isHexPair(s string) bool {
	if len(s) != 2 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

// LaunchAgentProgram returns the first ProgramArguments entry from a rendered
// launchd plist, which is the binary launchd will execute.
func LaunchAgentProgram(plistPath string) (string, error) {
	data, err := os.ReadFile(plistPath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	inArgs := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case "<key>ProgramArguments</key>":
			inArgs = true
			continue
		case "</array>":
			if inArgs {
				return "", fmt.Errorf("ProgramArguments has no executable")
			}
		}
		if !inArgs || !strings.HasPrefix(trimmed, "<string>") || !strings.HasSuffix(trimmed, "</string>") {
			continue
		}
		value := strings.TrimPrefix(trimmed, "<string>")
		value = strings.TrimSuffix(value, "</string>")
		return xmlUnescape(value), nil
	}
	return "", fmt.Errorf("ProgramArguments not found")
}

func serviceSlug(repoRoot string) string {
	cleaned := strings.Trim(filepath.Base(repoRoot), ".")
	if cleaned == "" || cleaned == string(filepath.Separator) {
		cleaned = "repo"
	}
	var b strings.Builder
	for _, r := range strings.ToLower(cleaned) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	return strings.Trim(b.String(), "-")
}

func xmlUnescape(s string) string {
	replacer := strings.NewReplacer(
		"&apos;", "'",
		"&quot;", `"`,
		"&gt;", ">",
		"&lt;", "<",
		"&amp;", "&",
	)
	return replacer.Replace(s)
}
