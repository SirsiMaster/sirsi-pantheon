#!/usr/bin/env python3
"""Router supervisor hook (ADR-024).

SessionStart/prompt: register the thread (handshake), heartbeat, and — keyed on
OS truth — re-assert the ONE prescribed watcher by injecting the router's
`arm_instruction` into context when no live watcher exists for this thread.
Stop: optional backstop that blocks until the watcher is detected alive.

ADR-024 retires the per-claude caffeinator and the auto fs-watcher: the `/loop`
Monitor is the single liveness/wake mechanism for a claude surface. `register`
no longer spawns a watcher; it RETURNS the spec, and this hook hands the
agent the spec's `arm_instruction`.
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
from pathlib import Path


# The portfolio router is shared and lives in sirsi-pantheon. Sessions started
# outside any repo (bare ~) still belong to a portfolio agent, so we fall back
# to this shared root (Decision 4: fire from any cwd, incl. non-pantheon).
DEFAULT_ROUTER_REPO = Path("~/Development/sirsi-pantheon").expanduser()


def find_repo_root(start: Path) -> Path | None:
    for candidate in [start, *start.parents]:
        if (candidate / ".agents" / "idea-router" / "state.json").is_file():
            return candidate
    return None


def portfolio_agent_for_cwd(cwd: str) -> str | None:
    """Map an actual cwd to its portfolio agent id (mirrors the established
    SessionStart inline mapping). Returns None if the cwd is not in a known
    portfolio repo, so the caller can fall back to agents.json matching."""
    pairs = [
        ("sirsi-pantheon", "claude-pantheon"),
        ("assiduous", "claude-assiduous"),
        ("FinalWishes", "claude-finalwishes"),
        ("SirsiNexusApp", "claude-nexus"),
        ("porch-and-alley", "claude-porch-and-alley"),
        ("homebrew-tools", "claude-homebrew-tools"),
    ]
    for needle, agent in pairs:
        if needle in cwd:
            return agent
    return None


def load_json(path: Path) -> dict:
    with path.open("r", encoding="utf-8") as handle:
        return json.load(handle)


def resolve_agent_by_cwd(cwd: Path, agents: dict) -> str | None:
    """Resolve the session's agent by matching the ACTUAL cwd against agents.json
    cwd entries, longest-prefix wins (most specific). Returns None when no claude
    agent's cwd contains the cwd.

    Critically (ADR-024 refinement, claude-home item 210348): this MUST NOT
    default to claude-pantheon. A home/non-portfolio cwd that resolved to
    claude-pantheon caused a claude-home session to falsely heartbeat
    claude-pantheon's thread — a cross-agent liveness lie. No confident match =>
    the caller no-ops, never guesses an agent."""
    try:
        cwd_resolved = cwd.resolve()
    except (OSError, RuntimeError):
        return None
    best_agent, best_len = None, -1
    for agent_id, config in agents.get("agents", {}).items():
        if config.get("type") != "claude":
            continue
        c = config.get("cwd")
        if not c:
            continue
        try:
            base = Path(c).expanduser().resolve()
        except (OSError, RuntimeError):
            continue
        if cwd_resolved == base or base in cwd_resolved.parents:
            n = len(base.parts)
            if n > best_len:
                best_len, best_agent = n, agent_id
    return best_agent


def resolve_agent(cwd: Path, agents: dict) -> str | None:
    """Authoritative agents.json longest-prefix match, then the portfolio
    substring map as a confident fallback. None => no-op (never default)."""
    return resolve_agent_by_cwd(cwd, agents) or portfolio_agent_for_cwd(str(cwd))


def pending_items(state: dict, agent_id: str) -> list[str]:
    pending = state.get("pending") or {}
    items = list(pending.get(agent_id) or [])
    if agent_id == "claude-pantheon":
        for item in state.get("pending_for_claude") or []:
            if item not in items:
                items.append(item)
    return items


def pull_model_open_items(router_root: Path, agent_id: str) -> list[str]:
    """Count open items addressed to agent_id under items/ — the one inbox
    (ADR-024 §5). reviews/ and decisions/ are NOT polled."""
    items_dir = router_root / "items"
    if not items_dir.is_dir():
        return []
    matches: list[str] = []
    for path in items_dir.glob("*.md"):
        try:
            text = path.read_text(encoding="utf-8")
        except OSError:
            continue
        if not text.startswith("---\n"):
            continue
        end = text.find("\n---\n", 4)
        if end < 0:
            continue
        frontmatter = text[4:end]
        to_val = status_val = ""
        for line in frontmatter.splitlines():
            key, sep, value = line.partition(":")
            if not sep:
                continue
            key = key.strip()
            value = value.strip().strip('"')
            if key == "to":
                to_val = value
            elif key == "status":
                status_val = value
        if status_val == "open" and to_val == agent_id:
            matches.append(path.stem)
    return matches


# Bound on the ancestry walk. Deep enough for any real hook chain, shallow
# enough that a pathological/looping tree can never spin.
MAX_ANCESTRY_HOPS = 12


def claude_session_pid(runner=subprocess.run, start_pid: int | None = None) -> int | None:
    """PID of the long-lived `claude` process that owns this session.

    Walks UP the ancestry until it finds the actual claude process, instead of
    assuming a fixed chain depth. The prior implementation took EXACTLY the
    grandparent, on the assumption the chain is always claude → shell → python3.
    That holds on some surfaces and not others: where the chain is deeper (the
    desktop app), the grandparent is a TRANSIENT per-prompt process, so the
    anchor changed on every hook fire, `adopt_or_register`'s (agent_id, pid)
    match never hit, and a NEW thread was minted every emission — claude-deck
    accreted 7 active records while every other agent held exactly 1
    (measured 2026-07-24, claude-home item 20260724-202920).

    Returns None when no claude ancestor is found, which routes callers to the
    freshest-active fallback (adopt rather than always-mint). Returning a WRONG
    pid is the failure mode this replaces — a wrong-but-present anchor silently
    bypasses that fallback, which is gated on `anchor is None`.
    """
    pid = os.getppid() if start_pid is None else start_pid
    for _ in range(MAX_ANCESTRY_HOPS):
        try:
            out = runner(
                ["ps", "-p", str(pid), "-o", "ppid=,comm="],
                capture_output=True, text=True, timeout=2,
            )
        except (subprocess.TimeoutExpired, OSError):
            return None
        if out.returncode != 0 or not out.stdout.strip():
            return None
        parts = out.stdout.strip().split(None, 1)
        if not parts:
            return None
        # `comm` is the executable name. Match the DURABLE claude process only:
        # the CLI reports `claude`, the desktop app `Claude`, wrappers
        # `claude-code`. "Claude Helper (Renderer)" / "(GPU)" are the TRANSIENT
        # per-prompt children — matching one of those is precisely the bug this
        # replaces, so `helper` is excluded explicitly.
        comm = parts[1].strip() if len(parts) > 1 else ""
        name = os.path.basename(comm).lower()
        if name.startswith("claude") and "helper" not in name:
            return pid
        try:
            pid = int(parts[0])
        except ValueError:
            return None
        if pid <= 1:  # reached init without finding claude
            return None
    return None


def supervisor_mode(env: dict | None = None) -> str:
    """Resolve the supervisor mode (ADR-024 §4).

      "off"     — SIRSI_SUPERVISOR=0: suppress managed arming + Stop-gate.
                  The register spec stays visible (handled by `register` itself).
      "enforce" — SIRSI_SUPERVISOR=enforce: arming injection + the Stop-gate
                  backstop (off by default).
      "on"      — default: arming injection on, Stop-gate off.
    """
    env = env if env is not None else os.environ
    v = (env.get("SIRSI_SUPERVISOR") or "").strip().lower()
    if v == "0":
        return "off"
    if v == "enforce":
        return "enforce"
    return "on"


def watcher_armed(thread_id: str, runner=subprocess.run) -> bool:
    """OS-truth liveness check (ADR-024 §3, F2): is a watcher process alive for
    THIS thread? Keys on `pgrep -f thr-<thread_id>` — the same (agent_id, pid)
    identity ADR-022 reaps on — NEVER the harness TaskList (it falsely reports
    empty) and NEVER the shared `DIR=` loop body (it matches OTHER agents'
    loops on a shared host, a false 'already armed')."""
    if not thread_id:
        return False
    try:
        out = runner(["pgrep", "-f", thread_id], capture_output=True, text=True, timeout=2)
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return False
    return out.returncode == 0 and bool((out.stdout or "").strip())


def should_arm(thread_id: str, mode: str, runner=subprocess.run) -> bool:
    """Check-then-arm decision (ADR-024 §3). Arm iff the supervisor is not off,
    we have a thread, and NO live watcher exists for it (OS truth). This is the
    single gate behind F1 (re-assert when gone) and F2 (never duplicate when an
    OS watcher is alive — keyed on pgrep, never TaskList)."""
    if mode == "off" or not thread_id:
        return False
    return not watcher_armed(thread_id, runner=runner)


def register_handshake(agent_id: str, repo_path: Path, thread_id: str | None,
                       anchor: int | None, runner=subprocess.run) -> tuple[str | None, str]:
    """Run `thread register --json` (the ADR-024 handshake) and return
    (thread_id, arm_instruction). register no longer spawns a watcher; it
    returns the spec the surface must arm."""
    args = ["sirsi", "thread", "register", "--agent", agent_id,
            "--surface", "claude", "--repo", str(repo_path), "--json"]
    if thread_id:
        args += ["--thread", thread_id]
    if anchor:
        args += ["--anchor-pid", str(anchor)]
    try:
        out = runner(args, capture_output=True, text=True, timeout=3)
        data = json.loads(out.stdout) if out.returncode == 0 and out.stdout.strip() else {}
    except (FileNotFoundError, subprocess.TimeoutExpired, json.JSONDecodeError):
        return thread_id, ""
    tid = data.get("thread_id") or thread_id
    arm = (data.get("watcher") or {}).get("arm_instruction", "")
    return tid, arm


def _pid_alive(pid) -> bool:
    """OS-truth: is this pid running? (signal 0 probe)"""
    try:
        os.kill(int(pid), 0)
        return True
    except (OSError, ValueError, TypeError):
        return False


def adopt_or_register(agent_id: str, repo_path: Path, runner=subprocess.run) -> tuple[str | None, str]:
    """Adopt the existing active thread anchored on THIS session's pid if one
    exists; else register a new one. Returns (thread_id, arm_instruction).
    No watcher is spawned here — register is a pure handshake (ADR-024 D2).

    Identity is **(agent_id, anchor pid)** — the durable session identity per
    ADR-022 §4 / ADR-024 §2. The prior implementation keyed on "freshest active
    record within 300s of last register", which mints a new thread_id once the
    prior record idles past the threshold (a single live claude session that
    sits ≥5min between hook fires). That is the phantom pid=0/os=unknown
    accretion source claude-pantheon characterized in finding 20260602-032542.
    Filter by anchor pid: same session = same record, every wakeup.
    """
    try:
        out = runner(["sirsi", "thread", "list", "--json"], capture_output=True, text=True, timeout=2)
        threads = json.loads(out.stdout) if out.returncode == 0 and out.stdout.strip() else []
    except (FileNotFoundError, subprocess.TimeoutExpired, json.JSONDecodeError):
        threads = []

    anchor = claude_session_pid()
    existing = None

    # Primary path: adopt the thread anchored on THIS session's pid. Durable
    # across wakeups regardless of idle gap — same long-lived claude CLI = same
    # pid = same record. (agent_id, pid) is the OS-truth identity.
    if anchor is not None:
        for t in threads:
            th = t.get("thread") or {}
            if (th.get("agent_id") == agent_id
                    and th.get("status") == "active"
                    and th.get("pid") == anchor):
                existing = th.get("thread_id")
                break

    # Fallback: only when anchor is unresolvable (subprocess timeout / parse
    # failure in claude_session_pid). Picks the freshest active record so we
    # still adopt rather than always-mint. The 300s window mirrors the prior
    # behavior but is now reachable ONLY in the anchor-unknown corner case, not
    # on every routine wakeup.
    if existing is None and anchor is None:
        fresh = [
            t for t in threads
            if (t.get("thread") or {}).get("agent_id") == agent_id
            and (t.get("thread") or {}).get("status") == "active"
            and t.get("idle_seconds", 1e9) < 300
        ]
        if fresh:
            fresh.sort(key=lambda t: t.get("idle_seconds", 1e9))
            existing = (fresh[0].get("thread") or {}).get("thread_id")

    # Machine-level reuse (owner directive 2026-07-23): ONE record per
    # (agent, surface, machine). A new session pid must not mint while a prior
    # session's record is adoptable — that was the per-restart mint churn that
    # accreted 310 claude-home records (ADR-043 context). Adopt the freshest
    # ACTIVE record whose anchor pid is DEAD (its session ended → orphaned,
    # not concurrent); register --thread re-anchors it to this pid. A record
    # with a LIVE foreign pid is a genuinely concurrent session — only then
    # is a fresh mint correct.
    if existing is None and anchor is not None:
        orphaned = []
        for t in threads:
            th = t.get("thread") or {}
            if th.get("agent_id") != agent_id or th.get("status") != "active":
                continue
            pid = th.get("pid")
            if pid and pid != anchor and not _pid_alive(pid):
                orphaned.append(t)
        if orphaned:
            orphaned.sort(key=lambda t: t.get("idle_seconds", 1e9))
            existing = (orphaned[0].get("thread") or {}).get("thread_id")

    return register_handshake(agent_id, repo_path, existing, anchor, runner=runner)


def heartbeat(thread_id: str, runner=subprocess.run) -> None:
    if not thread_id:
        return
    try:
        runner(["sirsi", "thread", "heartbeat", "--thread", thread_id, "--quiet"],
               capture_output=True, timeout=2)
    except (FileNotFoundError, subprocess.TimeoutExpired):
        pass


def main() -> int:
    mode = sys.argv[1] if len(sys.argv) > 1 else "session"
    cwd = Path.cwd()
    repo_override = os.environ.get("SIRSI_ROUTER_REPO_ROOT")
    if repo_override:
        repo_root = Path(repo_override).expanduser().resolve()
    else:
        # Walk up from cwd; fall back to the shared portfolio router so the
        # supervisor fires even from a non-pantheon / bare-home cwd (Decision 4).
        repo_root = find_repo_root(cwd) or DEFAULT_ROUTER_REPO
    if repo_root is None:
        return 0

    router_root = repo_root / ".agents" / "idea-router"
    try:
        state = load_json(router_root / "state.json")
        agents = load_json(router_root / "agents.json")
    except (OSError, json.JSONDecodeError):
        return 0

    # Resolve the agent by ACTUAL cwd. If we cannot confidently identify the
    # session's agent, NO-OP — never default to claude-pantheon and falsely
    # heartbeat its thread from another agent's session (ADR-024 refinement).
    agent_id = resolve_agent(cwd, agents)

    # Explicit identity override (SIRSI_ROUTER_AGENT). A home-rooted session that
    # works on a specific repo (e.g. Pantheon from the desktop app, cwd=$HOME)
    # can declare its true agent rather than falling to claude-home. Honored
    # ONLY when it names a registered agent AND the cwd did not already resolve
    # to a repo-specific agent — so a global setting can rescue home sessions
    # without ever mislabeling a session launched from another repo's dir.
    override = os.environ.get("SIRSI_ROUTER_AGENT", "").strip()
    if override and override in agents.get("agents", {}) and agent_id in (None, "claude-home"):
        agent_id = override

    if not agent_id:
        return 0
    sup = supervisor_mode()

    # Register handshake + heartbeat (no caffeinator, no fs-watcher — ADR-024).
    thread_id, arm_instruction = adopt_or_register(agent_id, repo_root)
    heartbeat(thread_id)

    # Stop-gate backstop (off by default; only under SIRSI_SUPERVISOR=enforce).
    if mode == "stop":
        if sup == "enforce" and thread_id and not watcher_armed(thread_id):
            print(f"router-supervisor: thread {thread_id} has no live /loop watcher — "
                  f"arm it before stopping (SIRSI_SUPERVISOR=enforce).", file=sys.stderr)
            return 2
        return 0

    # Check-then-arm (F1): re-assert the ONE watcher every SessionStart/wakeup.
    # Keyed on OS truth (pgrep), so we never duplicate (F2) and never collide
    # with other agents' loops. Suppressed only when SIRSI_SUPERVISOR=0.
    if arm_instruction and should_arm(thread_id, sup):
        print(f"router-supervisor:{agent_id} arm your watcher (thread {thread_id}): {arm_instruction}")

    legacy = pending_items(state, agent_id)
    pull = pull_model_open_items(router_root, agent_id)
    total = len(legacy) + len(pull)
    if total == 0:
        return 0

    prefix = "router-inbox" if mode == "prompt" else "router"
    noun = "item" if total == 1 else "items"
    parts = []
    if legacy:
        parts.append(f"{len(legacy)} legacy")
    if pull:
        parts.append(f"{len(pull)} pull-model")
    print(f"{prefix}:{agent_id} has {total} pending inbox {noun} ({' + '.join(parts)})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
