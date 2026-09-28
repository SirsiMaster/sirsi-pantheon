// Package namespec implements the ADR-072 universal thread naming grammar —
// <agent>-<project>-<machine>[-<task>] — as pure, side-effect-free parsing,
// validation, and construction (Phase 1: grammar + validator, no enforcement).
//
// The first three hyphens delimit agent, project, and machine, each `[a-z0-9]+`.
// The task is the remainder after the third hyphen and MAY contain hyphens
// (SSA C1, 2026-09-28): `[a-z0-9]+(-[a-z0-9]+)*`. So `claude-finalwishes-m1-fw-r02`
// parses as agent=claude, project=finalwishes, machine=m1, task=fw-r02.
//
// This package performs NO authority: it does not check agent/project against a
// known set (the origin-pinned versioned schema, ADR-072 C2, is Phase 2) and it
// does not bind the machine alias to a credentialed machine-id (C3). It only
// answers "is this string a well-formed name, and what are its slots."
package namespec

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// slotRe matches a single grammar slot (agent, project, machine): one or more
// lowercase alphanumerics, no hyphens (the hyphen is the delimiter).
var slotRe = regexp.MustCompile(`^[a-z0-9]+$`)

// taskRe matches the optional task — the remainder after the third hyphen. It is
// one or more slot segments joined by hyphens, so it may itself contain hyphens.
var taskRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Name is a parsed ADR-072 thread name. Task is "" when the name has no task.
type Name struct {
	Agent   string
	Project string
	Machine string
	Task    string
}

// Parse splits s into the four grammar slots. It splits on at most the first
// three hyphens: agent, project, machine, then everything after the third hyphen
// is the task. It returns an error for a string that is not well-formed.
func Parse(s string) (Name, error) {
	if s == "" {
		return Name{}, fmt.Errorf("namespec: empty name")
	}
	parts := strings.SplitN(s, "-", 4)
	if len(parts) < 3 {
		return Name{}, fmt.Errorf("namespec: %q needs at least <agent>-<project>-<machine>", s)
	}
	n := Name{Agent: parts[0], Project: parts[1], Machine: parts[2]}
	if len(parts) == 4 {
		n.Task = parts[3]
	}
	if err := n.Validate(); err != nil {
		return Name{}, err
	}
	return n, nil
}

// Validate checks each slot against the grammar: agent, project, and machine are
// `[a-z0-9]+`; the task, when present, is `[a-z0-9]+(-[a-z0-9]+)*`.
func (n Name) Validate() error {
	for _, f := range []struct{ label, val string }{
		{"agent", n.Agent}, {"project", n.Project}, {"machine", n.Machine},
	} {
		if !slotRe.MatchString(f.val) {
			return fmt.Errorf("namespec: %s %q must match [a-z0-9]+", f.label, f.val)
		}
	}
	if n.Task != "" && !taskRe.MatchString(n.Task) {
		return fmt.Errorf("namespec: task %q must match [a-z0-9]+(-[a-z0-9]+)*", n.Task)
	}
	return nil
}

// String renders the canonical name. String and Parse round-trip: for any valid
// Name n, Parse(n.String()) == n.
func (n Name) String() string {
	base := n.Agent + "-" + n.Project + "-" + n.Machine
	if n.Task != "" {
		return base + "-" + n.Task
	}
	return base
}

// Construct builds a validated canonical Name from components. machine is
// normally the value returned by GleanMachine; task may be "".
func Construct(agent, project, machine, task string) (Name, error) {
	n := Name{Agent: agent, Project: project, Machine: machine, Task: task}
	if err := n.Validate(); err != nil {
		return Name{}, err
	}
	return n, nil
}

// GleanMachine returns the machine slot derived from THIS host's designated name:
// the hostname prefix before the first '.', lowercased (`M1.local` -> `m1`). It is
// the friendly alias only; binding it to the credentialed machine-id is C3 (P2).
func GleanMachine() (string, error) {
	h, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("namespec: cannot resolve hostname: %w", err)
	}
	return gleanFrom(h)
}

// gleanFrom derives the machine slot from a host name (split out for testability).
func gleanFrom(host string) (string, error) {
	prefix := strings.ToLower(strings.SplitN(host, ".", 2)[0])
	if !slotRe.MatchString(prefix) {
		return "", fmt.Errorf("namespec: machine slot %q gleaned from host %q is not [a-z0-9]+", prefix, host)
	}
	return prefix, nil
}
