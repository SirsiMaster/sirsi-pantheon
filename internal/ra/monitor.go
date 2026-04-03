package ra

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// WindowStatus describes the current state of a single Ra spawned window.
type WindowStatus struct {
	Name     string
	PID      int
	State    string // "running", "completed", "failed", "crashed"
	ExitCode int
	LogTail  string // last 10 lines
	Duration time.Duration
}

// DeploymentStatus describes the aggregate state of all Ra windows.
type DeploymentStatus struct {
	Windows   []WindowStatus
	StartedAt time.Time
	AllDone   bool
}

// WindowResult holds the final outcome of a completed Ra window.
type WindowResult struct {
	Name     string
	ExitCode int
	LogText  string
	Duration time.Duration
}

// deploymentMeta is the on-disk format for deployment.json.
type deploymentMeta struct {
	StartedAt string   `json:"started_at"`
	Scopes    []string `json:"scopes"`
}

// Monitor reads the current state of all Ra windows from raDir.
// raDir is expected to be ~/.config/ra/ with subdirs: pids/, logs/, exits/.
func Monitor(raDir string) (*DeploymentStatus, error) {
	meta, err := readDeploymentMeta(raDir)
	if err != nil {
		return nil, fmt.Errorf("ra monitor: %w", err)
	}

	startedAt, _ := time.Parse(time.RFC3339, meta.StartedAt)
	now := time.Now()

	var windows []WindowStatus
	allDone := true

	for _, scope := range meta.Scopes {
		ws := WindowStatus{
			Name:     scope,
			Duration: now.Sub(startedAt),
		}

		// Read PID.
		pidFile := filepath.Join(raDir, "pids", scope+".pid")
		pid, pidErr := readPIDFile(pidFile)
		if pidErr != nil {
			ws.State = "crashed"
			ws.LogTail = readLogTail(raDir, scope)
			windows = append(windows, ws)
			continue
		}
		ws.PID = pid

		// Check if process is alive (signal 0 = test existence).
		alive := isProcessAlive(pid)

		if alive {
			ws.State = "running"
			allDone = false
		} else {
			// Process is dead. Check exit file.
			exitFile := filepath.Join(raDir, "exits", scope+".exit")
			exitCode, exitErr := readExitFile(exitFile)
			if exitErr != nil {
				ws.State = "crashed"
			} else {
				ws.ExitCode = exitCode
				if exitCode == 0 {
					ws.State = "completed"
				} else {
					ws.State = "failed"
				}
			}
		}

		ws.LogTail = readLogTail(raDir, scope)
		windows = append(windows, ws)
	}

	return &DeploymentStatus{
		Windows:   windows,
		StartedAt: startedAt,
		AllDone:   allDone,
	}, nil
}

// WaitAll polls Monitor every 5 seconds until all windows are done or timeout expires.
func WaitAll(raDir string, timeout time.Duration) (*DeploymentStatus, error) {
	deadline := time.Now().Add(timeout)

	for {
		status, err := Monitor(raDir)
		if err != nil {
			return nil, err
		}
		if status.AllDone {
			return status, nil
		}
		if time.Now().After(deadline) {
			return status, fmt.Errorf("ra wait: timeout after %s", timeout)
		}
		time.Sleep(5 * time.Second)
	}
}

// CollectResults reads all log files and exit codes into WindowResult structs.
func CollectResults(raDir string) ([]WindowResult, error) {
	meta, err := readDeploymentMeta(raDir)
	if err != nil {
		return nil, fmt.Errorf("ra collect: %w", err)
	}

	startedAt, _ := time.Parse(time.RFC3339, meta.StartedAt)
	now := time.Now()

	var results []WindowResult
	for _, scope := range meta.Scopes {
		wr := WindowResult{
			Name:     scope,
			Duration: now.Sub(startedAt),
		}

		// Read exit code.
		exitFile := filepath.Join(raDir, "exits", scope+".exit")
		exitCode, err := readExitFile(exitFile)
		if err != nil {
			wr.ExitCode = -1
		} else {
			wr.ExitCode = exitCode
		}

		// Read full log.
		logFile := filepath.Join(raDir, "logs", scope+".log")
		data, err := os.ReadFile(logFile)
		if err == nil {
			wr.LogText = string(data)
		}

		results = append(results, wr)
	}

	return results, nil
}

// readDeploymentMeta loads deployment.json from raDir.
func readDeploymentMeta(raDir string) (*deploymentMeta, error) {
	data, err := os.ReadFile(filepath.Join(raDir, "deployment.json"))
	if err != nil {
		return nil, fmt.Errorf("read deployment.json: %w", err)
	}

	var meta deploymentMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parse deployment.json: %w", err)
	}
	return &meta, nil
}

// readPIDFile reads and parses a PID file.
func readPIDFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// readExitFile reads and parses an exit code file.
func readExitFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// readLogTail returns the last 10 lines of a scope's log file.
func readLogTail(raDir, scope string) string {
	logFile := filepath.Join(raDir, "logs", scope+".log")
	data, err := os.ReadFile(logFile)
	if err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > 10 {
		lines = lines[len(lines)-10:]
	}
	return strings.Join(lines, "\n")
}
