package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// This is an opt-in attended heartbeat boundary, not a dispatcher or a watcher.
// Provider session/markers are scope evidence within the existing local-host
// trust boundary; canonical registration retains its existing authorization.
type attendedEnsureCaller struct {
	session, agent, thread, machine string
}

type attendedEnsureOps struct {
	load      func(string) (*router.Thread, error)
	anchor    func() (int, string, error)
	register  func(*router.Thread) (*router.Thread, error)
	persist   func(string, string) error
	heartbeat func(string, router.HeartbeatUpdate) (*router.Thread, error)
}

// Same predecessor and proven generation select one normal registration even
// if two surface ticks overlap. This ID is an idempotency name, not authority.
func attendedSuccessorID(old *router.Thread, pid int, start string) string {
	b, _ := json.Marshal([]any{old.ThreadID, old.AgentID, old.SessionID, old.MachineID, pid, start})
	sum := sha256.Sum256(b)
	return fmt.Sprintf("thr-%x", sum[:8])
}

func ensureAttendedHeartbeat(c attendedEnsureCaller, explicit string, upd router.HeartbeatUpdate, op attendedEnsureOps) (*router.Thread, error) {
	if c.session == "" || c.agent == "" || c.thread == "" || c.machine == "" || (explicit != "" && explicit != c.thread) {
		return nil, fmt.Errorf("attended ensure requires this exact registered caller session; unrelated --thread is refused")
	}
	old, err := op.load(c.thread)
	if err != nil {
		return nil, err
	}
	if old == nil || old.ThreadID != c.thread || old.AgentID != c.agent || old.SessionID != c.session || old.Surface != "codex" || old.MachineID != c.machine || old.Host == "" {
		return nil, fmt.Errorf("attended ensure cannot adopt an unknown, foreign or unrelated binding")
	}
	if old.Status == router.ThreadStatusSuspended {
		return nil, fmt.Errorf("attended ensure cannot resume a suspended session")
	}
	pid, start, err := op.anchor()
	if err != nil {
		return nil, err
	}
	if pid <= 1 || start == "" {
		return nil, fmt.Errorf("attended ensure has no verified durable host generation")
	}
	current := old
	changed := old.PID != pid || old.StartTime != start
	if old.Status.IsTerminal() && !changed {
		return nil, fmt.Errorf("attended ensure cannot renew a terminal binding in the same generation")
	}
	if changed {
		// A changed native generation gets a fresh normal registration; never
		// repoint or revive the predecessor and never let session-key reuse pick
		// an unrelated sibling. No grant, lane result or
		// terminal error is copied onto a successor; no force/override path.
		intent := &router.Thread{
			EnsureCompatible: true,
			ThreadID:         attendedSuccessorID(old, pid, start),
			AgentID:          old.AgentID, Surface: old.Surface, SessionID: old.SessionID,
			Repo: old.Repo, Workstream: old.Workstream, Watches: append([]string(nil), old.Watches...),
			WakeMechanism: old.WakeMechanism, CurrentItem: old.CurrentItem,
			PID: pid, StartTime: start, Host: old.Host, MachineID: old.MachineID,
		}
		registered, registerErr := op.register(intent)
		if registerErr != nil {
			return nil, registerErr
		}
		if registered == nil || registered.ThreadID == "" {
			return nil, fmt.Errorf("attended registration returned no binding")
		}
		current, err = op.load(registered.ThreadID)
		if err != nil {
			return nil, err
		}
		if current != nil && current.ThreadID == old.ThreadID {
			return nil, fmt.Errorf("attended registration attempted predecessor reuse")
		}
	}
	if current == nil || current.Status.IsTerminal() || current.Status == router.ThreadStatusSuspended ||
		current.AgentID != old.AgentID || current.SessionID != old.SessionID || current.Surface != old.Surface ||
		current.MachineID != old.MachineID || current.Host != old.Host || current.PID != pid || current.StartTime != start ||
		current.Repo != old.Repo || current.Workstream != old.Workstream || current.WakeMechanism != old.WakeMechanism ||
		current.CurrentItem != old.CurrentItem ||
		!reflect.DeepEqual(current.Watches, old.Watches) {
		return nil, fmt.Errorf("attended registration readback does not preserve exact caller scope/generation")
	}
	// Recheck native identity after registration/readback, before publishing a
	// new marker or heartbeat. OS lookup failure never falls back to a helper.
	latestPID, latestStart, err := op.anchor()
	if err != nil {
		return nil, err
	}
	if latestPID != pid || latestStart != start {
		return nil, fmt.Errorf("durable host changed during attended preflight; retry a fresh ensure")
	}
	if changed {
		if err := op.persist(c.session, current.ThreadID); err != nil {
			return nil, fmt.Errorf("attended binding persisted but session marker failed: %w", err)
		}
	}
	upd.ExpectedPID, upd.ExpectedStart = pid, start
	return op.heartbeat(current.ThreadID, upd)
}

func strictCodexAnchor() (int, string, error) {
	pid, err := resolveDurableAnchor(os.Getppid(), "codex", lookupAnchorProcess)
	if err != nil {
		return 0, "", err
	}
	pid, err = refineDesktopAnchor(pid, "codex", os.Getenv("CODEX_INTERNAL_ORIGINATOR_OVERRIDE"), lookupAnchorChildren)
	if err != nil {
		return 0, "", err
	}
	start := router.PIDStartTimeOf(pid)
	if start == "" || router.PIDStateOf(pid, start) != router.PIDAlive || router.PIDStartTimeOf(pid) != start {
		return 0, "", fmt.Errorf("cannot prove current durable Codex host generation")
	}
	return pid, start, nil
}

// The command's only injected dependency is native kernel observation. Marker,
// resolver, normal registration/readback and fenced heartbeat stay real.
func runAttendedEnsure(root, explicit string, upd router.HeartbeatUpdate, observe func() (int, string, error)) (*router.Thread, error) {
	if !routercfg.StoreWake() {
		return nil, fmt.Errorf("attended ensure requires the existing canonical registry mode; explicit legacy mode is not overridden")
	}
	// Do not reinterpret another provider's environment as a Codex session.
	if strings.TrimSpace(os.Getenv(router.SessionIDEnv)) != "" {
		return nil, fmt.Errorf("attended ensure supports the actual Codex session only")
	}
	session := router.CurrentSessionID()
	agent := router.ReadSessionAgentMarker(session)
	if declared := strings.TrimSpace(os.Getenv("SIRSI_AGENT_ID")); declared != "" && declared != agent {
		return nil, fmt.Errorf("attended ensure caller agent differs from session binding")
	}
	thread := router.ReadSessionThreadMarker(session)
	if declared := strings.TrimSpace(os.Getenv("SIRSI_THREAD_ID")); declared != "" && declared != thread {
		return nil, fmt.Errorf("attended ensure explicit runtime thread differs from session binding")
	}
	load := func(id string) (*router.Thread, error) {
		store, err := routerstore.Resolve()
		if err != nil {
			return nil, err
		}
		defer store.Close()
		record, err := store.GetThreadRecord(id)
		if err != nil {
			return nil, err
		}
		var t router.Thread
		if err := json.Unmarshal(record.Payload, &t); err != nil {
			return nil, err
		}
		if t.ThreadID != record.ThreadID || t.AgentID != record.Agent || string(t.Status) != record.Status {
			return nil, fmt.Errorf("attended canonical record/payload identity mismatch")
		}
		return &t, nil
	}
	return ensureAttendedHeartbeat(attendedEnsureCaller{session, agent, thread, router.MachineID()}, explicit, upd, attendedEnsureOps{
		load: load, anchor: observe,
		register: func(t *router.Thread) (*router.Thread, error) { return router.RegisterThread(root, t) },
		persist:  router.WriteSessionThreadMarker,
		heartbeat: func(id string, u router.HeartbeatUpdate) (*router.Thread, error) {
			return router.Heartbeat(root, id, u)
		},
	})
}
