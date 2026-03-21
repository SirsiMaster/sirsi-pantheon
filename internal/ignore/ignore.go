// Package ignore provides .anubisignore support — exclude paths from scanning.
// Syntax is identical to .gitignore: glob patterns, comments (#), negation (!).
package ignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// IgnoreList contains compiled ignore patterns.
type IgnoreList struct {
	patterns []pattern
}

type pattern struct {
	glob   string
	negate bool
}

// Load reads an .anubisignore file and returns a compiled IgnoreList.
func Load(path string) (*IgnoreList, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	list := &IgnoreList{}
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		p := pattern{glob: line}
		if strings.HasPrefix(line, "!") {
			p.negate = true
			p.glob = line[1:]
		}

		list.patterns = append(list.patterns, p)
	}

	return list, scanner.Err()
}

// LoadFromDir searches for .anubisignore in the given directory and parents.
func LoadFromDir(dir string) *IgnoreList {
	// Check current directory
	path := filepath.Join(dir, ".anubisignore")
	if list, err := Load(path); err == nil {
		return list
	}

	// Check home directory
	home, _ := os.UserHomeDir()
	path = filepath.Join(home, ".anubisignore")
	if list, err := Load(path); err == nil {
		return list
	}

	// Check config directory
	path = filepath.Join(home, ".config", "anubis", ".anubisignore")
	if list, err := Load(path); err == nil {
		return list
	}

	return &IgnoreList{} // Empty — ignore nothing
}

// ShouldIgnore returns true if the given path matches any ignore pattern.
func (l *IgnoreList) ShouldIgnore(path string) bool {
	if len(l.patterns) == 0 {
		return false
	}

	ignored := false
	for _, p := range l.patterns {
		matched := matchPattern(p.glob, path)
		if matched {
			if p.negate {
				ignored = false // Negation: un-ignore
			} else {
				ignored = true
			}
		}
	}
	return ignored
}

// matchPattern checks if a path matches a glob-style pattern.
func matchPattern(glob, path string) bool {
	// Expand ~ in patterns
	if strings.HasPrefix(glob, "~/") {
		home, _ := os.UserHomeDir()
		glob = filepath.Join(home, glob[2:])
	}

	// Direct path match
	if matched, _ := filepath.Match(glob, path); matched {
		return true
	}

	// Match against basename
	base := filepath.Base(path)
	if matched, _ := filepath.Match(glob, base); matched {
		return true
	}

	// Check if path contains the pattern as a directory component
	if strings.Contains(path, glob) {
		return true
	}

	return false
}

// DefaultIgnoreContent returns the default .anubisignore template.
func DefaultIgnoreContent() string {
	return `# .anubisignore — Exclude paths from Anubis scanning
# Syntax is identical to .gitignore

# Project-specific caches you want to keep
# node_modules
# .venv

# Custom model directories
# ~/models/production

# Mounted network drives
# /Volumes/NAS

# Specific IDE workspace
# .idea/workspace.xml
`
}
