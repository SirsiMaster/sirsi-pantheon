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
		{Key: "doctor", Label: "System Diagnostics", Glyph: "𓁐", Args: []string{"diagnose"}},
		{Key: "guard", Label: "Guard Check", Glyph: "🛡", Args: []string{"guard"}},
		{Key: "quality", Label: "Quality Audit", Glyph: "𓆄", Args: []string{"audit"}},
		{Key: "network", Label: "Network Audit", Glyph: "🌐", Args: []string{"network"}},
		{Key: "dedup", Label: "Find Duplicates", Glyph: "🔍", Args: []string{"duplicates", "."}},
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
	last     RunReceipt
}

// RunReceipt is the terminal, verifiable outcome of the most recent action.
type RunReceipt struct {
	Key        string `json:"key,omitempty"`
	Status     string `json:"status,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
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

func (r *Runner) Status() (bool, string, RunReceipt) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running, r.current, r.last
}

// Run executes a command by key. Returns an error if already running or key is invalid.
func (r *Runner) Run(key string) error {
	actions := DefaultActions()
	for i := range actions {
		if actions[i].Key == key {
			a := actions[i]
			return r.RunArgs(a.Key, a.Label, a.Glyph, a.Args)
		}
	}
	return fmt.Errorf("unknown command: %s", key)
}

// RunArgs executes `sirsi <args...>` under a logical key, streaming output via
// the event buffer (E5). It is the generic execution path the typed action
// contract dispatches to. Returns an error if a command is already running or
// args are empty.
func (r *Runner) RunArgs(key, label, glyph string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no args for action: %s", key)
	}

	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return fmt.Errorf("command already running: %s", r.current)
	}
	r.running = true
	r.current = key
	r.mu.Unlock()

	action := &Runnable{Key: key, Label: label, Glyph: glyph, Args: args}
	r.events.Push(Event{
		Type: "run_start",
		Data: mustJSON(map[string]string{"key": key, "label": label, "glyph": glyph}),
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
	r.last = RunReceipt{Key: action.Key, Status: status, Error: errMsg, DurationMs: elapsed.Milliseconds()}
	r.running = false
	r.current = ""
	r.mu.Unlock()
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// apiRun handles POST /api/run. It accepts the typed ActionRequest JSON body
// (E5) and remains backward-compatible with the legacy ?cmd=<key> query form.
// Destructive actions are gated by the E2 confirm engine. See dispatchRun.
func (s *Server) apiRun(w http.ResponseWriter, r *http.Request) {
	s.dispatchRun(w, r)
}

// apiRunStatus handles GET /api/run/status — returns current runner state.
func (s *Server) apiRunStatus(w http.ResponseWriter, r *http.Request) {
	if s.runner == nil {
		writeJSON(w, map[string]interface{}{"running": false})
		return
	}
	running, current, receipt := s.runner.Status()
	writeJSON(w, map[string]interface{}{"running": running, "current": current, "last": receipt})
}
