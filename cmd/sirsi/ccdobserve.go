package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type ccdRunner struct {
	pid   int
	start float64
}

// The initial observation is completed before either the kill or archive pass.
// Keep child-process cleanup mechanics separate; no failed initial lookup may
// become an empty live-session set. Tests replace only this owned observation.
var ccdCollectRunners = func() ([]ccdRunner, error) {
	return ccdObserveRunners(func() ([]byte, error) {
		return exec.Command("pgrep", "-f", "claude.app/Contents/MacOS/claude ").Output()
	}, ccdPidStart)
}

func ccdObserveRunners(query func() ([]byte, error), start func(int) (float64, bool)) ([]ccdRunner, error) {
	body, err := query()
	if err != nil {
		var exit *exec.ExitError
		// pgrep's documented no-match exit is not an observation error, but
		// partial output or diagnostics with that exit is not absence proof.
		if !errors.As(err, &exit) || exit.ExitCode() != 1 || len(strings.TrimSpace(string(body))) != 0 || len(exit.Stderr) != 0 {
			return nil, fmt.Errorf("process query failed: %w", err)
		}
		return nil, nil
	}
	seen := map[int]bool{}
	var out []ccdRunner
	for _, field := range strings.Fields(string(body)) {
		pid, err := strconv.Atoi(field)
		if err != nil || pid <= 0 || seen[pid] {
			return nil, fmt.Errorf("invalid or duplicate runner PID in observation")
		}
		seen[pid] = true
		when, ok := start(pid)
		if !ok || when <= 0 {
			return nil, fmt.Errorf("runner %d start identity unavailable", pid)
		}
		out = append(out, ccdRunner{pid: pid, start: when})
	}
	return out, nil
}
