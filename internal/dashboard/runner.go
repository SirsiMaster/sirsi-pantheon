package dashboard

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
)

// Runnable defines a command the dashboard can execute.
type Runnable struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Glyph string   `json:"glyph"`
	Args  []string `json:"-"`
}

// DefaultActions returns the set of commands available from the dashboard.
func DefaultActions() []Runnable {
	return []Runnable{
		{Key: "scan", Label: "Scan", Glyph: "𓁢", Args: []string{"scan"}},
		{Key: "ghosts", Label: "Ghost Hunt", Glyph: "𓂓", Args: []string{"ghosts"}},
		{Key: "doctor", Label: "Doctor", Glyph: "𓁐", Args: []string{"doctor"}},
		{Key: "guard", Label: "Guard Check", Glyph: "🛡", Args: []string{"guard", "--once"}},
		{Key: "quality", Label: "Quality Audit", Glyph: "𓆄", Args: []string{"quality"}},
		{Key: "network", Label: "Network Audit", Glyph: "🌐", Args: []string{"network"}},
		{Key: "dedup", Label: "Find Duplicates", Glyph: "🔍", Args: []string{"dedup", "."}},
		{Key: "hardware", Label: "Hardware", Glyph: "⚡", Args: []string{"hardware"}},
	}
}

// Runner manages command execution from the dashboard.
// Only one command runs at a time — queuing is not supported.
type Runner struct {
	mu       sync.Mutex
	running  bool
	current  string
	events   *EventBuffer
	sirsiBin string
	notifyDB *notify.Store
}

// NewRunner creates a command runner that pushes output to the event buffer.
func NewRunner(events *EventBuffer, sirsiBin string, nStore *notify.Store) *Runner {
	return &Runner{
		events:   events,
		sirsiBin: sirsiBin,
		notifyDB: nStore,
	}
}

// IsRunning reports whether a command is currently executing.
func (r *Runner) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// Current returns the key of the currently running command, or "".
func (r *Runner) Current() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.current
}

// Run executes a command by key. Returns an error if already running or key is invalid.
func (r *Runner) Run(key string) error {
	actions := DefaultActions()
	var action *Runnable
	for i := range actions {
		if actions[i].Key == key {
			action = &actions[i]
			break
		}
	}
	if action == nil {
		return fmt.Errorf("unknown command: %s", key)
	}

	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return fmt.Errorf("command already running: %s", r.current)
	}
	r.running = true
	r.current = key
	r.mu.Unlock()

	r.events.Push(Event{
		Type: "run_start",
		Data: mustJSON(map[string]string{"key": key, "label": action.Label, "glyph": action.Glyph}),
	})

	go r.execute(action)
	return nil
}

func (r *Runner) execute(action *Runnable) {
	start := time.Now()
	cmd := exec.Command(r.sirsiBin, action.Args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.finish(action, start, fmt.Errorf("pipe: %w", err))
		return
	}
	cmd.Stderr = cmd.Stdout // merge stderr into stdout

	if startErr := cmd.Start(); startErr != nil {
		r.finish(action, start, fmt.Errorf("start: %w", startErr))
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		r.events.Push(Event{
			Type: "run_output",
			Data: mustJSON(map[string]string{"key": action.Key, "line": line}),
		})
	}

	err = cmd.Wait()
	r.finish(action, start, err)
}

func (r *Runner) finish(action *Runnable, start time.Time, err error) {
	elapsed := time.Since(start)
	status := "success"
	errMsg := ""
	if err != nil {
		status = "error"
		errMsg = err.Error()
	}

	r.events.Push(Event{
		Type: "run_complete",
		Data: mustJSON(map[string]interface{}{
			"key":         action.Key,
			"label":       action.Label,
			"status":      status,
			"error":       errMsg,
			"duration_ms": elapsed.Milliseconds(),
		}),
	})

	// Record notification for history
	if r.notifyDB != nil {
		n := notify.Notification{
			Source:     "sirsi",
			Action:     action.Key,
			DurationMs: elapsed.Milliseconds(),
		}
		if err != nil {
			n.Severity = notify.SeverityError
			n.Summary = fmt.Sprintf("%s failed (%s)", action.Label, elapsed.Truncate(time.Second))
		} else {
			n.Severity = notify.SeveritySuccess
			n.Summary = fmt.Sprintf("%s completed (%s)", action.Label, elapsed.Truncate(time.Second))
		}
		_ = r.notifyDB.Record(n)
	}

	r.mu.Lock()
	r.running = false
	r.current = ""
	r.mu.Unlock()
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// apiRun handles POST /api/run?cmd=<key> — starts a command.
func (s *Server) apiRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if s.runner == nil {
		writeError(w, "runner not available", http.StatusServiceUnavailable)
		return
	}

	key := r.URL.Query().Get("cmd")
	if key == "" {
		writeError(w, "missing cmd parameter", http.StatusBadRequest)
		return
	}

	if err := s.runner.Run(key); err != nil {
		writeError(w, err.Error(), http.StatusConflict)
		return
	}

	writeJSON(w, map[string]string{"status": "started", "cmd": key})
}

// apiRunStatus handles GET /api/run/status — returns current runner state.
func (s *Server) apiRunStatus(w http.ResponseWriter, r *http.Request) {
	if s.runner == nil {
		writeJSON(w, map[string]interface{}{"running": false})
		return
	}
	writeJSON(w, map[string]interface{}{
		"running": s.runner.IsRunning(),
		"current": s.runner.Current(),
	})
}
