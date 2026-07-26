#!/usr/bin/env python3
"""ADR-024 acceptance tests 4/5/6 for the router supervisor hook.

Run: python3 .claude/hooks/router_inbox_check_test.py
"""

import importlib.util
import os
import types
import unittest
from pathlib import Path
from unittest import mock

# Load the hyphenless-but-underscored hook module by path.
_spec = importlib.util.spec_from_file_location(
    "router_inbox_check", str(Path(__file__).with_name("router_inbox_check.py"))
)
hook = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(hook)


def fake_runner(returncode: int, stdout: str = ""):
    """Return a subprocess.run stand-in that always yields this result."""
    def _run(*_args, **_kwargs):
        return types.SimpleNamespace(returncode=returncode, stdout=stdout)
    return _run


class TestSupervisorMode(unittest.TestCase):
    def test_off(self):
        self.assertEqual(hook.supervisor_mode({"SIRSI_SUPERVISOR": "0"}), "off")

    def test_enforce(self):
        self.assertEqual(hook.supervisor_mode({"SIRSI_SUPERVISOR": "enforce"}), "enforce")

    def test_default_on(self):
        self.assertEqual(hook.supervisor_mode({}), "on")


class TestWatcherArmedKeysOnPgrep(unittest.TestCase):
    def test_alive(self):
        # pgrep found a matching process (rc 0, a pid on stdout) => armed.
        self.assertTrue(hook.watcher_armed("thr-abc", runner=fake_runner(0, "54321\n")))

    def test_gone(self):
        # pgrep found nothing (rc 1, empty) => not armed.
        self.assertFalse(hook.watcher_armed("thr-abc", runner=fake_runner(1, "")))


class TestShouldArm(unittest.TestCase):
    # Acceptance test 5 (F1): wakeup with the watcher GONE => re-assert exactly one.
    def test_f1_rearm_when_gone(self):
        self.assertTrue(hook.should_arm("thr-abc", "on", runner=fake_runner(1, "")))

    # Acceptance test 6 (F2): an OS watcher is ALIVE (pgrep hit) => do NOT arm a
    # duplicate. The decision consults pgrep only — it never sees a TaskList, so
    # a falsely-empty TaskList cannot cause a duplicate.
    def test_f2_no_duplicate_when_alive(self):
        self.assertFalse(hook.should_arm("thr-abc", "on", runner=fake_runner(0, "54321\n")))

    # Acceptance test 4: SIRSI_SUPERVISOR=0 suppresses managed arming entirely,
    # even when no watcher exists. (The spec stays visible — that's `register`'s
    # job, not this hook's.)
    def test_supervisor_off_suppresses_arming(self):
        self.assertFalse(hook.should_arm("thr-abc", "off", runner=fake_runner(1, "")))

    def test_no_thread_no_arm(self):
        self.assertFalse(hook.should_arm("", "on", runner=fake_runner(1, "")))


class TestPortfolioAgentForCwd(unittest.TestCase):
    def test_pantheon(self):
        self.assertEqual(hook.portfolio_agent_for_cwd("/Users/x/Development/sirsi-pantheon/cmd"), "claude-pantheon")

    def test_other_repos(self):
        self.assertEqual(hook.portfolio_agent_for_cwd("/Users/x/Development/assiduous"), "claude-assiduous")
        self.assertEqual(hook.portfolio_agent_for_cwd("/Users/x/Development/FinalWishes/web"), "claude-finalwishes")

    def test_bare_home_no_match(self):
        # Bare home (no portfolio repo in path) => None, caller falls back.
        self.assertIsNone(hook.portfolio_agent_for_cwd("/Users/x"))


class TestResolveAgentByCwd(unittest.TestCase):
    AGENTS = {"agents": {
        "claude-home": {"type": "claude", "cwd": str(Path.home())},
        "claude-pantheon": {"type": "claude", "cwd": str(Path.home() / "Development" / "sirsi-pantheon")},
    }}

    # ADR-024 refinement (item 210348): a home cwd must resolve to claude-home,
    # NEVER default to claude-pantheon (which caused a false cross-heartbeat).
    def test_home_resolves_to_claude_home_not_pantheon(self):
        self.assertEqual(hook.resolve_agent_by_cwd(Path.home(), self.AGENTS), "claude-home")

    def test_longest_prefix_wins(self):
        cwd = Path.home() / "Development" / "sirsi-pantheon" / "cmd"
        self.assertEqual(hook.resolve_agent_by_cwd(cwd, self.AGENTS), "claude-pantheon")

    def test_no_match_returns_none_not_default(self):
        # Empty agents => no confident match => None (caller no-ops), NOT pantheon.
        self.assertIsNone(hook.resolve_agent_by_cwd(Path("/var"), {"agents": {}}))


class TestAdoptOrRegister_AnchorPidIdentity(unittest.TestCase):
    """Finding 20260602-032542: a long-lived claude session whose prior thread
    record idled past 300s gets a NEW thread_id minted each wakeup, leaving
    phantom pid=0/os=unknown records. The fix: filter by (agent_id, anchor pid),
    not by idle window. Same session = same record forever.

    These tests stub `thread list --json` + `register --json` via a programmable
    fake runner that returns different bodies per call. No host side effects.
    """

    def _runner(self, list_body: str, register_body: str = '{"thread_id":"thr-NEW","watcher":{"arm_instruction":"arm"}}'):
        calls = {"n": 0}
        def _run(args, **kwargs):
            calls["n"] += 1
            verb = args[2] if len(args) > 2 else ""
            if verb == "list":
                return types.SimpleNamespace(returncode=0, stdout=list_body)
            if verb == "register":
                # echo any --thread passed so tests can assert which id was reused.
                tid = "thr-NEW"
                if "--thread" in args:
                    tid = args[args.index("--thread") + 1]
                return types.SimpleNamespace(returncode=0, stdout='{"thread_id":"%s","watcher":{"arm_instruction":"arm"}}' % tid)
            return types.SimpleNamespace(returncode=0, stdout="")
        return _run, calls

    def test_adopts_record_with_matching_anchor_pid(self):
        """Same pid as the live claude session => adopt that thread, never mint."""
        list_body = '[{"thread":{"agent_id":"claude-home","status":"active","thread_id":"thr-LIVE","pid":12345},"idle_seconds":1000.0}]'
        runner, _ = self._runner(list_body)
        original_anchor = hook.claude_session_pid
        hook.claude_session_pid = lambda: 12345  # type: ignore[assignment]
        try:
            tid, _ = hook.adopt_or_register("claude-home", Path("/tmp"), runner=runner)
        finally:
            hook.claude_session_pid = original_anchor  # type: ignore[assignment]
        # The record idled 1000s — past the old 300s window — but anchor pid
        # matches, so it MUST be adopted (the bug fix).
        self.assertEqual(tid, "thr-LIVE")

    def test_does_not_adopt_record_on_different_pid_even_if_fresh(self):
        """A fresh record on a DIFFERENT pid is NOT this session — must mint."""
        list_body = '[{"thread":{"agent_id":"claude-home","status":"active","thread_id":"thr-OTHER","pid":99999},"idle_seconds":5.0}]'
        runner, _ = self._runner(list_body)
        original_anchor = hook.claude_session_pid
        hook.claude_session_pid = lambda: 12345  # type: ignore[assignment]
        try:
            tid, _ = hook.adopt_or_register("claude-home", Path("/tmp"), runner=runner)
        finally:
            hook.claude_session_pid = original_anchor  # type: ignore[assignment]
        # No --thread is passed to register => server mints; our stub returns thr-NEW.
        self.assertEqual(tid, "thr-NEW")

    def test_fallback_to_freshness_only_when_anchor_unresolvable(self):
        """When claude_session_pid() returns None (subprocess timeout), fall
        back to the legacy `idle < 300s` heuristic so we still adopt rather
        than always-mint."""
        list_body = '[{"thread":{"agent_id":"claude-home","status":"active","thread_id":"thr-FRESH","pid":12345},"idle_seconds":10.0}]'
        runner, _ = self._runner(list_body)
        original_anchor = hook.claude_session_pid
        hook.claude_session_pid = lambda: None  # type: ignore[assignment]
        try:
            tid, _ = hook.adopt_or_register("claude-home", Path("/tmp"), runner=runner)
        finally:
            hook.claude_session_pid = original_anchor  # type: ignore[assignment]
        self.assertEqual(tid, "thr-FRESH")

    def test_no_fresh_no_anchor_match_mints(self):
        """Stale record + anchor mismatch => mint a fresh thread."""
        list_body = '[{"thread":{"agent_id":"claude-home","status":"active","thread_id":"thr-STALE","pid":99999},"idle_seconds":1000.0}]'
        runner, _ = self._runner(list_body)
        original_anchor = hook.claude_session_pid
        hook.claude_session_pid = lambda: 12345  # type: ignore[assignment]
        try:
            tid, _ = hook.adopt_or_register("claude-home", Path("/tmp"), runner=runner)
        finally:
            hook.claude_session_pid = original_anchor  # type: ignore[assignment]
        self.assertEqual(tid, "thr-NEW")


def chain_runner(tree: dict[int, tuple[int, str]]):
    """subprocess.run stand-in backed by a {pid: (ppid, comm)} process tree.

    Responds to `ps -p <pid> -o ppid=,comm=` like the real ps would.
    """
    def _run(args, **_kwargs):
        pid = int(args[2])  # ["ps", "-p", "<pid>", "-o", "ppid=,comm="]
        if pid not in tree:
            return types.SimpleNamespace(returncode=1, stdout="")
        ppid, comm = tree[pid]
        return types.SimpleNamespace(returncode=0, stdout=f"{ppid} {comm}\n")
    return _run


class TestClaudeSessionPidWalk(unittest.TestCase):
    """The per-resume thread-mint root cause: claude_session_pid must resolve the
    STABLE claude-CLI pid by walking the ancestry, not a fixed depth."""

    def test_walks_through_ephemeral_shells_to_claude(self):
        # python's parent shell (200) -> wrapper (300) -> claude (400) -> launchd
        tree = {
            200: (300, "zsh"),
            300: (400, "login"),
            400: (1, "/Users/x/.../claude.app/Contents/MacOS/claude"),
        }
        got = hook.claude_session_pid(runner=chain_runner(tree), start_pid=200)
        self.assertEqual(got, 400)

    def test_stable_across_resumes_different_ephemeral_shell(self):
        # The whole point: a DIFFERENT fresh shell each resume (201 vs 200) must
        # still resolve to the SAME claude pid (400) — so adopt matches, no mint.
        tree_a = {200: (300, "zsh"), 300: (400, "login"),
                  400: (1, "/x/MacOS/claude")}
        tree_b = {201: (301, "zsh"), 301: (400, "login"),
                  400: (1, "/x/MacOS/claude")}
        a = hook.claude_session_pid(runner=chain_runner(tree_a), start_pid=200)
        b = hook.claude_session_pid(runner=chain_runner(tree_b), start_pid=201)
        self.assertEqual(a, 400)
        self.assertEqual(b, 400)
        self.assertEqual(a, b)  # stable identity → same thread adopted

    def test_skips_transient_claude_helper_children(self):
        # "Claude Helper (Renderer)" CONTAINS "claude" but is the per-prompt
        # transient — matching it reproduces the mint-per-emission bug. The walk
        # must pass through it to the durable claude process.
        tree = {200: (300, "zsh"), 300: (400, "Claude Helper (Renderer)"),
                400: (1, "/x/MacOS/claude")}
        self.assertEqual(
            hook.claude_session_pid(runner=chain_runner(tree), start_pid=200), 400)

    def test_no_claude_ancestor_returns_none(self):
        tree = {200: (300, "zsh"), 300: (1, "launchd")}
        got = hook.claude_session_pid(runner=chain_runner(tree), start_pid=200)
        self.assertIsNone(got)

    def test_does_not_loop_on_cycle(self):
        tree = {200: (300, "zsh"), 300: (200, "bash")}  # cycle
        got = hook.claude_session_pid(runner=chain_runner(tree), start_pid=200)
        self.assertIsNone(got)

    def test_ps_failure_midwalk_returns_none_not_ephemeral_pid(self):
        # If ps fails partway, return None (caller uses freshness) — never a
        # wrong/ephemeral pid that would mint a fresh thread.
        tree = {200: (300, "zsh")}  # 300 absent → lookup fails
        got = hook.claude_session_pid(runner=chain_runner(tree), start_pid=200)
        self.assertIsNone(got)


class TestIdentityOverrideNarrowsOnly(unittest.TestCase):
    """SIRSI_ROUTER_AGENT may narrow, never contradict (item 20260724-205100)."""

    AGENTS = {"agents": {
        "claude-home": {"type": "claude", "cwd": "/Users/x"},
        "claude-deck": {"type": "claude", "cwd": "/Users/x/Development/deck"},
        "claude-pantheon": {"type": "claude", "cwd": "/Users/x/Development/sirsi-pantheon"},
    }}

    def _apply(self, resolved, cwd, override):
        with mock.patch.dict(os.environ, {"SIRSI_ROUTER_AGENT": override}, clear=False):
            with mock.patch.object(hook.Path, "resolve", lambda self: self):
                return hook.apply_identity_override(resolved, hook.Path(cwd), self.AGENTS)

    def test_machine_global_cannot_hijack_a_home_session(self):
        # The live trap: a user-level settings.json env block exported
        # SIRSI_ROUTER_AGENT=claude-deck into EVERY session, relabeling the
        # home-rooted router conduit and making it heartbeat claude-deck's thread.
        self.assertEqual(
            self._apply("claude-home", "/Users/x", "claude-deck"), "claude-home")

    def test_override_cannot_rescue_an_unresolved_session(self):
        # No confident cwd match and the override's cwd does not contain this
        # one → still no-op. A session may not assert an identity it cannot prove.
        self.assertIsNone(
            self._apply(None, "/tmp/somewhere", "claude-pantheon"))

    def test_override_may_narrow_to_an_agent_that_owns_the_cwd(self):
        self.assertEqual(
            self._apply("claude-home", "/Users/x/Development/deck/sub", "claude-deck"),
            "claude-deck")

    def test_unregistered_override_is_ignored(self):
        self.assertEqual(
            self._apply("claude-home", "/Users/x", "claude-nope"), "claude-home")

    def test_empty_override_is_a_no_op(self):
        self.assertEqual(self._apply("claude-home", "/Users/x", ""), "claude-home")



if __name__ == "__main__":
    unittest.main(verbosity=2)
