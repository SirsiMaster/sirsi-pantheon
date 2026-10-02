package main

import (
	"fmt"
	"io/fs"
	"os"
	"testing"
)

// When neither ps nor the kernel lookup is permitted (strict sandbox), the
// anchor falls back to the immediate parent; any other lookup failure still
// refuses (both directions).
func TestResolveAnchorPIDFallsBackToParentWhenExecDenied(t *testing.T) {
	old := lookupAnchorProcess
	defer func() { lookupAnchorProcess = old }()

	lookupAnchorProcess = func(pid int) (anchorProcess, error) {
		return anchorProcess{}, fmt.Errorf("inspect pid %d: %w", pid, fs.ErrPermission)
	}
	got, err := resolveAnchorPID("worker")
	if err != nil || got != os.Getppid() {
		t.Fatalf("denied exec: want parent %d, got %d err=%v", os.Getppid(), got, err)
	}

	lookupAnchorProcess = func(pid int) (anchorProcess, error) {
		return anchorProcess{}, fmt.Errorf("inspect pid %d: no such process", pid)
	}
	if got, err := resolveAnchorPID("worker"); err == nil {
		t.Fatalf("a non-permission failure must still refuse, got anchor %d", got)
	}
}
