package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

type ensureHTTPEntry struct {
	t              *testing.T
	backend        routerstore.Store
	home, session  string
	old            *router.Thread
	pid            int
	start          string
	kernelErr      error
	mu             sync.Mutex
	trace          []string
	beforeInsert   func(routerstore.ThreadRecord) error
	beforeInsertID string
	afterInbox     func()
}

func newEnsureHTTPEntry(t *testing.T) *ensureHTTPEntry {
	t.Helper()
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "SIRSI_") || strings.HasPrefix(key, "GIT_") || key == router.SessionIDEnv || key == "CODEX_SESSION_ID" || key == "CODEX_THREAD_ID" {
			t.Setenv(key, "")
		}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(t.TempDir(), "owned.db"))
	t.Setenv("SIRSI_ALLOW_SCHEMA_MIGRATE", "1")
	t.Setenv("SIRSI_ROUTER_STORE_WAKE", "1")
	backend, err := routerstore.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	host, err := routerstore.ThreadOwnerHost()
	if err != nil {
		t.Fatal(err)
	}
	f := &ensureHTTPEntry{t: t, backend: backend, home: home, session: "owned-codex-session", pid: 400, start: "generation-A"}
	f.old = &router.Thread{ThreadID: "owned-original", AgentID: "codex-pantheon", SessionID: f.session, Surface: "codex", Host: host, MachineID: router.MachineID(), PID: 400, StartTime: "generation-A", Repo: "owned-repo", Workstream: "owned-original-task", Watches: []string{"codex-pantheon"}, WakeMechanism: "automation", Status: router.ThreadStatusActive, StartedAt: time.Now().UTC(), LastSeenAt: time.Now().UTC()}
	if f.old.MachineID == "" {
		t.Fatal("fixture needs local machine identity; no invented identity")
	}
	f.seed(f.old)
	token, _, err := backend.MintScopedHostToken(host, "owned inert ensure fixture", []string{"codex-pantheon"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := routerstore.Handler(backend, routerstore.ServerOptions{Token: "private-fixture-bootstrap", RuleOfRa: "enforce", MaxWait: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.TrimPrefix(r.URL.Path, "/v1/call/")
		label := method
		var record routerstore.ThreadRecord
		if method == "UpsertThreadCAS" {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Error(readErr)
				w.WriteHeader(500)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			var envelope struct {
				Args []json.RawMessage `json:"args"`
			}
			if err := json.Unmarshal(body, &envelope); err != nil || len(envelope.Args) != 1 {
				t.Error("bad fixture RPC envelope")
				w.WriteHeader(500)
				return
			}
			raw := envelope.Args[0]
			if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
				var parts []json.RawMessage
				if err := json.Unmarshal(raw, &parts); err != nil || len(parts) != 2 {
					t.Error("bad guarded argument")
					w.WriteHeader(500)
					return
				}
				raw = parts[1]
			}
			if err := json.Unmarshal(raw, &record); err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			label = "write:" + record.ThreadID
			if record.CreateOnly {
				label = "insert:" + record.ThreadID
			}
		}
		if method == "Inbox" {
			if session, err := backend.GetSession(r.Header.Get("X-Sirsi-Session")); err == nil {
				label = "Inbox:" + session.ThreadID
			}
		}
		f.mu.Lock()
		f.trace = append(f.trace, label)
		hook := f.beforeInsert
		inject := hook != nil && record.ThreadID == f.beforeInsertID
		if inject {
			f.beforeInsert = nil
		}
		f.mu.Unlock()
		// The target upsert triggers this independently of CreateOnly, so a
		// guard-omission mutant faces exactly the same injected race.
		if inject {
			if err := hook(record); err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
		}
		if method == "Inbox" {
			// Complete the real authenticated Inbox before the test transition,
			// but do not release its response to the caller until that transition
			// is installed. No sleep or OS process is generation evidence here.
			response := httptest.NewRecorder()
			h.ServeHTTP(response, r)
			f.mu.Lock()
			f.trace = append(f.trace, "after-"+label)
			after := f.afterInbox
			f.afterInbox = nil
			f.mu.Unlock()
			if after != nil {
				after()
			}
			for key, values := range response.Header() {
				w.Header()[key] = append([]string(nil), values...)
			}
			w.WriteHeader(response.Code)
			_, _ = w.Write(response.Body.Bytes())
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SIRSI_ROUTER_DB", "")
	t.Setenv("SIRSI_ROUTER_URL", srv.URL)
	t.Setenv("SIRSI_ROUTER_TOKEN", token)
	t.Setenv("SIRSI_AGENT_ID", f.old.AgentID)
	t.Setenv("CODEX_SESSION_ID", f.session)
	t.Setenv("CODEX_THREAD_ID", f.session)
	if err := router.WriteSessionAgentMarker(f.session, f.old.AgentID); err != nil {
		t.Fatal(err)
	}
	if err := router.WriteSessionThreadMarker(f.session, f.old.ThreadID); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".agents", "idea-router"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := router.SaveRegistry(filepath.Join(repo, ".agents", "idea-router"), &router.Registry{Agents: map[string]router.AgentConfig{f.old.AgentID: {ID: f.old.AgentID, Type: "codex", Repo: repo, Cwd: repo, Workstream: f.old.Workstream, Wake: router.WakeConfig{Mechanism: "none"}}}}); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)
	previousJSON := JsonOutput
	JsonOutput = true
	t.Cleanup(func() { JsonOutput = previousJSON })
	return f
}

func (f *ensureHTTPEntry) seed(thread *router.Thread) {
	f.t.Helper()
	if err := f.seedRecord(thread); err != nil {
		f.t.Fatal(err)
	}
}

func (f *ensureHTTPEntry) seedRecord(thread *router.Thread) error {
	payload, err := json.Marshal(thread)
	if err != nil {
		return err
	}
	if ok, err := f.backend.UpsertThreadCAS(routerstore.ThreadRecord{ThreadID: thread.ThreadID, Agent: thread.AgentID, Host: thread.Host, Status: string(thread.Status), LastSeenAt: thread.LastSeenAt.Format(time.RFC3339Nano), Payload: payload}); err != nil || !ok {
		return fmt.Errorf("fixture seed: applied=%v error=%v", ok, err)
	}
	return nil
}

func (f *ensureHTTPEntry) observe() (int, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pid, f.start, f.kernelErr
}

func (f *ensureHTTPEntry) command(wait bool, args ...string) ([]byte, error) {
	f.t.Helper()
	cmd := newThreadHeartbeatCmdWithKernel(f.observe)
	if wait {
		cmd = newRouterWaitCmdWithKernel(f.observe)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.Bytes(), err
}

func (f *ensureHTTPEntry) traceCopy() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.trace...)
}

func (f *ensureHTTPEntry) record(id string) routerstore.ThreadRecord {
	f.t.Helper()
	r, err := f.backend.GetThreadRecord(id)
	if err != nil {
		f.t.Fatal(err)
	}
	return r
}

func TestAttendedEnsureCobraHTTPGenerationAndWait(t *testing.T) {
	for _, boundary := range []string{"heartbeat", "wait"} {
		t.Run(boundary, func(t *testing.T) {
			f := newEnsureHTTPEntry(t)
			wait := boundary == "wait"
			args := []string{"--ensure"}
			if wait {
				args = []string{f.old.AgentID, "--ensure", "--timeout", "0"}
			}
			if _, err := f.command(wait, args...); err != nil {
				t.Fatal("A live actual entry", err)
			}
			if strings.Contains(strings.Join(f.traceCopy(), "\n"), "insert:") {
				t.Fatal("same-generation entry churned")
			}
			old := f.record(f.old.ThreadID)
			f.pid, f.start = 500, "generation-B"
			if wait {
				// Preflight must accept this current explicit binding, then the
				// post-Wait ensure must use its returned B binding, not this A flag.
				args = append(args, "--thread", f.old.ThreadID)
			}
			out, err := f.command(wait, args...)
			if err != nil {
				t.Fatal("actual B preflight", err)
			}
			var actual router.Thread
			if wait {
				var result struct {
					Thread router.Thread `json:"thread"`
				}
				if err := json.Unmarshal(out, &result); err != nil {
					t.Fatal(err)
				}
				actual = result.Thread
			} else if err := json.Unmarshal(out, &actual); err != nil {
				t.Fatal(err)
			}
			want := attendedSuccessorID(f.old, 500, "generation-B")
			if actual.ThreadID != want || actual.PID != 500 || actual.StartTime != "generation-B" || actual.Workstream != f.old.Workstream || !reflect.DeepEqual(actual.Watches, f.old.Watches) {
				t.Fatalf("actual new binding/scope missing: %+v", actual)
			}
			if router.ReadSessionThreadMarker(f.session) != want || !reflect.DeepEqual(old, f.record(f.old.ThreadID)) {
				t.Fatal("marker not real or predecessor changed")
			}
			trace := f.traceCopy()
			insert, heartbeat, inbox, afterInbox, postHeartbeat := -1, -1, -1, -1, -1
			for i, s := range trace {
				if s == "insert:"+want {
					insert = i
				}
				if s == "write:"+want {
					if heartbeat < 0 {
						heartbeat = i
					} else {
						postHeartbeat = i
					}
				}
				if s == "Inbox:"+want {
					inbox = i
				}
				if s == "after-Inbox:"+want {
					afterInbox = i
				}
			}
			if insert < 0 || heartbeat <= insert || (wait && (inbox <= heartbeat || afterInbox <= inbox || postHeartbeat <= afterInbox)) {
				t.Fatalf("production register/heartbeat/wait ordering wrong: %v", trace)
			}
			before := len(trace)
			if wait {
				args[len(args)-1] = want // caller explicitly consumes returned B for its next invocation
			}
			if _, err := f.command(wait, args...); err != nil {
				t.Fatal("B stable actual entry", err)
			}
			for _, s := range f.traceCopy()[before:] {
				if strings.HasPrefix(s, "insert:") {
					t.Fatal("repeated same generation registration")
				}
			}
		})
	}
	for _, mode := range []string{"same-generation", "generation-change", "kernel-failure", "pinned-generation-change"} {
		t.Run("wait-after-inbox/"+mode, func(t *testing.T) {
			f := newEnsureHTTPEntry(t)
			if mode == "pinned-generation-change" {
				t.Setenv("SIRSI_THREAD_ID", f.old.ThreadID)
			}
			var beforePost routerstore.ThreadRecord
			var snapshotErr error
			f.mu.Lock()
			f.afterInbox = func() {
				before, err := f.backend.GetThreadRecord(f.old.ThreadID)
				f.mu.Lock()
				defer f.mu.Unlock()
				beforePost, snapshotErr = before, err
				switch mode {
				case "generation-change", "pinned-generation-change":
					f.pid, f.start = 500, "generation-B"
				case "kernel-failure":
					f.kernelErr = fmt.Errorf("fixture post-wait kernel unavailable")
				}
			}
			f.mu.Unlock()
			out, err := f.command(true, f.old.AgentID, "--ensure", "--thread", f.old.ThreadID, "--timeout", "0")
			f.mu.Lock()
			before, readErr := beforePost, snapshotErr
			f.mu.Unlock()
			if readErr != nil || before.ThreadID != f.old.ThreadID {
				t.Fatal("actual Inbox transition was not observed", readErr)
			}
			trace := f.traceCopy()
			inboxDone, oldWrites, successorInsert, successorWrite, successorWrites := -1, 0, -1, -1, 0
			want := f.old.ThreadID
			if mode == "generation-change" || mode == "pinned-generation-change" {
				want = attendedSuccessorID(f.old, 500, "generation-B")
			}
			for i, step := range trace {
				if step == "after-Inbox:"+f.old.ThreadID {
					inboxDone = i
				}
				if step == "write:"+f.old.ThreadID {
					oldWrites++
					if oldWrites == 1 && inboxDone >= 0 {
						t.Fatal("initial heartbeat did not precede Inbox")
					}
					if oldWrites > 1 && inboxDone < 0 {
						t.Fatal("second heartbeat did not follow completed Inbox")
					}
				}
				if strings.HasPrefix(step, "insert:") {
					if step != "insert:"+want || want == f.old.ThreadID || successorInsert >= 0 {
						t.Fatalf("unexpected/duplicate generation registration: %v", trace)
					}
					successorInsert = i
				}
				if want != f.old.ThreadID && step == "write:"+want {
					successorWrites++
					successorWrite = i
				}
			}
			if inboxDone < 0 {
				t.Fatal("completed Inbox missing", trace)
			}
			if mode == "kernel-failure" {
				if err == nil || !strings.Contains(err.Error(), "fixture post-wait kernel unavailable") || oldWrites != 1 || successorInsert >= 0 || !reflect.DeepEqual(before, f.record(f.old.ThreadID)) {
					t.Fatalf("post-Wait failure was hidden or wrote heartbeat: err=%v trace=%v", err, trace)
				}
			} else if want == f.old.ThreadID {
				if oldWrites != 2 || successorInsert >= 0 {
					t.Fatal("same generation must renew exactly before and after Inbox without registration", trace)
				}
			} else if oldWrites != 1 || successorWrites != 1 || successorInsert <= inboxDone || successorWrite <= successorInsert || !reflect.DeepEqual(before, f.record(f.old.ThreadID)) || router.ReadSessionThreadMarker(f.session) != want {
				t.Fatal("post-Inbox successor ordering/scope/marker failed", trace)
			}
			if mode == "kernel-failure" || mode == "pinned-generation-change" {
				if err == nil || bytes.Contains(out, []byte(`"thread":`)) || bytes.Contains(out, []byte(`"items":`)) || bytes.Contains(out, []byte("No open items")) {
					t.Fatalf("failed post-ensure published JSON/success: err=%v output=%s", err, out)
				}
				if mode == "pinned-generation-change" && (!strings.Contains(err.Error(), want) || os.Getenv("SIRSI_THREAD_ID") != f.old.ThreadID) {
					t.Fatal("post-Wait pinned divergence hidden or environment rewritten", err)
				}
				return
			}
			if err != nil {
				t.Fatal("successful post-Wait ensure refused", err)
			}
			var result struct {
				Thread router.Thread `json:"thread"`
			}
			if err := json.Unmarshal(out, &result); err != nil {
				t.Fatal(err)
			}
			canonical := f.record(want)
			var current router.Thread
			if err := json.Unmarshal(canonical.Payload, &current); err != nil || !reflect.DeepEqual(result.Thread, current) || result.Thread.ThreadID != want {
				t.Fatal("wait returned pre-wait/stale binding instead of post-ensure canonical result", err)
			}
		})
	}
}

func TestAttendedEnsureCobraHTTPConflictsStayByteExact(t *testing.T) {
	for _, mode := range []string{"session", "workstream", "closed", "suspended", "raced-session"} {
		t.Run(mode, func(t *testing.T) {
			f := newEnsureHTTPEntry(t)
			f.pid, f.start = 500, "generation-B"
			conflict := *f.old
			conflict.ThreadID = attendedSuccessorID(f.old, f.pid, f.start)
			conflict.PID, conflict.StartTime = f.pid, f.start
			conflict.LastSeenAt = time.Now().UTC()
			switch mode {
			case "session", "raced-session":
				conflict.SessionID = "unrelated-session"
			case "workstream":
				conflict.Workstream = "other-task"
			case "closed":
				conflict.Status = router.ThreadStatusClosed
			case "suspended":
				conflict.Status = router.ThreadStatusSuspended
			}
			if mode == "raced-session" {
				f.mu.Lock()
				f.beforeInsert = func(routerstore.ThreadRecord) error { return f.seedRecord(&conflict) }
				f.beforeInsertID = conflict.ThreadID
				f.mu.Unlock()
			} else {
				f.seed(&conflict)
			}
			payload, _ := json.Marshal(&conflict)
			if _, err := f.command(false, "--ensure"); err == nil {
				t.Fatal("conflicting occupied successor was accepted")
			}
			got := f.record(conflict.ThreadID)
			if !bytes.Equal(got.Payload, payload) || got.LastSeenAt != conflict.LastSeenAt.Format(time.RFC3339Nano) || router.ReadSessionThreadMarker(f.session) != f.old.ThreadID {
				t.Fatal("conflict or marker mutated")
			}
			for _, s := range f.traceCopy() {
				if s == "write:"+conflict.ThreadID {
					t.Fatal("conflict reached heartbeat")
				}
			}
		})
	}
}

func TestAttendedEnsureCobraHTTPCompatibleTransitionReusesBinding(t *testing.T) {
	for _, raced := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing", true: "raced"}[raced], func(t *testing.T) {
			f := newEnsureHTTPEntry(t)
			f.pid, f.start = 500, "generation-B"
			compatible := *f.old
			compatible.ThreadID = attendedSuccessorID(f.old, f.pid, f.start)
			compatible.PID, compatible.StartTime = f.pid, f.start
			compatible.LastSeenAt = time.Now().UTC()
			if raced {
				f.mu.Lock()
				f.beforeInsert = func(routerstore.ThreadRecord) error { return f.seedRecord(&compatible) }
				f.beforeInsertID = compatible.ThreadID
				f.mu.Unlock()
			} else {
				f.seed(&compatible)
			}
			before := f.record(f.old.ThreadID)
			out, err := f.command(false, "--ensure")
			if err != nil {
				t.Fatal("same transition duplicate refused", err)
			}
			var actual router.Thread
			if err := json.Unmarshal(out, &actual); err != nil {
				t.Fatal(err)
			}
			if actual.ThreadID != compatible.ThreadID || router.ReadSessionThreadMarker(f.session) != compatible.ThreadID || !reflect.DeepEqual(before, f.record(f.old.ThreadID)) {
				t.Fatal("duplicate changed lineage or selected another row")
			}
			if !raced {
				for _, call := range f.traceCopy() {
					if strings.HasPrefix(call, "insert:") {
						t.Fatal("compatible readback was rewritten by registration")
					}
				}
			}
		})
	}
}

func TestAttendedEnsureCobraHTTPTerminalAndPinnedBindingConsumption(t *testing.T) {
	for _, pinned := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty-env", true: "pinned-old-env"}[pinned], func(t *testing.T) {
			f := newEnsureHTTPEntry(t)
			f.old.Status = router.ThreadStatusReaped
			f.old.LastSeenAt = f.old.LastSeenAt.Add(time.Second)
			f.seed(f.old)
			before := f.record(f.old.ThreadID)
			f.pid, f.start = 500, "generation-B"
			if pinned {
				t.Setenv("SIRSI_THREAD_ID", f.old.ThreadID)
			}
			out, err := f.command(false, "--ensure")
			if err != nil {
				t.Fatal("terminal bootstrap heartbeat", err)
			}
			var next router.Thread
			if err := json.Unmarshal(out, &next); err != nil {
				t.Fatal(err)
			}
			if next.ThreadID == f.old.ThreadID || !reflect.DeepEqual(before, f.record(f.old.ThreadID)) {
				t.Fatal("terminal original revived")
			}
			if pinned {
				if os.Getenv("SIRSI_THREAD_ID") != f.old.ThreadID {
					t.Fatal("hidden environment rewrite")
				}
				count := len(f.traceCopy())
				if _, err := f.command(false, "--ensure"); err == nil {
					t.Fatal("old pinned env silently adopted successor")
				}
				if len(f.traceCopy()) != count {
					t.Fatal("pinned mismatch contacted store")
				}
				t.Setenv("SIRSI_THREAD_ID", next.ThreadID) // actual caller explicitly consumes returned binding
			}
			if _, err := f.command(true, f.old.AgentID, "--ensure", "--timeout", "0"); err != nil {
				t.Fatal("actual wait after binding consumption", err)
			}
			found := false
			for _, s := range f.traceCopy() {
				if s == "Inbox:"+next.ThreadID {
					found = true
				}
			}
			if !found {
				t.Fatal("wait did not use actual returned session binding")
			}
		})
	}
}

func TestAttendedEnsureCobraHTTPMarkerAndScopeRefusals(t *testing.T) {
	for _, mode := range []string{"marker-failure", "wait-marker-failure", "foreign-thread", "foreign-wait", "foreign-wait-thread", "suspended", "same-generation-terminal", "unknown-kernel", "ambiguous-kernel"} {
		t.Run(mode, func(t *testing.T) {
			f := newEnsureHTTPEntry(t)
			args := []string{"--ensure"}
			wait := false
			f.pid, f.start = 500, "generation-B"
			switch mode {
			case "foreign-thread":
				args = append(args, "--thread", "unrelated")
			case "foreign-wait":
				wait = true
				args = []string{"unrelated", "--ensure", "--timeout", "0"}
			case "foreign-wait-thread":
				wait = true
				args = []string{f.old.AgentID, "--ensure", "--thread", "unrelated", "--timeout", "0"}
			case "wait-marker-failure":
				wait = true
				args = []string{f.old.AgentID, "--ensure", "--timeout", "0"}
			case "suspended":
				f.old.Status = router.ThreadStatusSuspended
				f.old.LastSeenAt = f.old.LastSeenAt.Add(time.Second)
				f.seed(f.old)
			case "same-generation-terminal":
				f.old.Status = router.ThreadStatusClosed
				f.old.LastSeenAt = f.old.LastSeenAt.Add(time.Second)
				f.seed(f.old)
				f.pid, f.start = 400, "generation-A"
			case "unknown-kernel":
				f.kernelErr = io.ErrUnexpectedEOF
			case "ambiguous-kernel":
				f.kernelErr = fmt.Errorf("ambiguous durable hosts")
			}
			before := f.record(f.old.ThreadID)
			if mode == "marker-failure" || mode == "wait-marker-failure" {
				// Directory substitution is owned fixture state, not a permission skip.
				var out bytes.Buffer
				// Only the first observation substitutes; the second must return the same generation.
				calls := 0
				observe := func() (int, string, error) {
					calls++
					if calls == 1 {
						p := filepath.Join(f.home, ".claude", "run", "thread-by-session", f.session)
						if err := os.Remove(p); err != nil {
							return 0, "", err
						}
						if err := os.Mkdir(p, 0700); err != nil {
							return 0, "", err
						}
					}
					return f.pid, f.start, nil
				}
				cmd := newThreadHeartbeatCmdWithKernel(observe)
				if wait {
					cmd = newRouterWaitCmdWithKernel(observe)
				}
				cmd.SetOut(&out)
				cmd.SetErr(&out)
				cmd.SetArgs(args)
				if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "session marker failed") {
					t.Fatal("marker failure lost", err)
				}
			} else if _, err := f.command(wait, args...); err == nil {
				t.Fatal("scope/retired/unknown entry accepted")
			}
			if !reflect.DeepEqual(before, f.record(f.old.ThreadID)) {
				t.Fatal("refusal mutated predecessor")
			}
			for _, s := range f.traceCopy() {
				if strings.HasPrefix(s, "write:") || strings.HasPrefix(s, "Inbox:") {
					t.Fatalf("refusal reached heartbeat/wait: %v", f.traceCopy())
				}
			}
		})
	}
}

func TestAttendedEnsureStrictKernelDoesNotUseParentFallback(t *testing.T) {
	previous := lookupAnchorProcess
	t.Cleanup(func() { lookupAnchorProcess = previous })
	lookupAnchorProcess = func(int) (anchorProcess, error) { return anchorProcess{}, fs.ErrPermission }
	if _, _, err := strictCodexAnchor(); err == nil {
		t.Fatal("strict ensure borrowed the immediate-parent permission fallback")
	}
}
