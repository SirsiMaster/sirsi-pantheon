#!/usr/bin/env python3
"""Read-only Git/registry census. Never stages, commits, resets or removes work."""
import argparse
import collections
import datetime
import hashlib
import json
import os
import pathlib
import subprocess


def git(root, *args):
    return subprocess.run(
        ["git", "--no-optional-locks", "-C", str(root), *args],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=120,
        env={**os.environ, "GIT_OPTIONAL_LOCKS": "0", "GIT_TERMINAL_PROMPT": "0"},
    )


def text(result):
    return result.stdout.decode("utf-8", "replace").strip()


def status_entries(raw):
    # -z has no pathname quoting. Rename records contain an extra old-name token.
    parts = raw.split(b"\0")
    result, i = [], 0
    while i < len(parts):
        item = parts[i]
        i += 1
        if not item:
            continue
        code = item[:2].decode("ascii", "replace")
        path = item[3:].decode("utf-8", "replace")
        result.append((code, path))
        if "R" in code or "C" in code:
            i += 1
    return result


def worktrees(root):
    r = git(root, "worktree", "list", "--porcelain", "-z")
    if r.returncode:
        return []
    return [pathlib.Path(p[9:].decode("utf-8", "replace"))
            for p in r.stdout.split(b"\0") if p.startswith(b"worktree ")]


def inspect(root, owners, home):
    head_before = git(root, "rev-parse", "HEAD")
    if head_before.returncode:
        return {"path": str(root), "owners": sorted(owners), "state": "UNREADABLE"}
    # The home directory is not an invitation to crawl private, non-project files.
    untracked = "no" if root == home else "all"
    state = git(root, "status", "--porcelain=v1", "-z", "--no-renames",
                "--untracked-files=" + untracked)
    if state.returncode:
        return {"path": str(root), "owners": sorted(owners), "state": "STATUS_ERROR"}
    entries = status_entries(state.stdout)
    codes = collections.Counter(code for code, _ in entries)
    branch = text(git(root, "symbolic-ref", "--short", "-q", "HEAD"))
    upstream = git(root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
    divergence = None
    if not upstream.returncode:
        count = git(root, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
        if not count.returncode:
            ahead, behind = map(int, text(count).split())
            divergence = {"ahead": ahead, "behind": behind, "upstream": text(upstream)}
    return {
        "path": str(root), "owners": sorted(owners),
        "head": text(head_before), "branch": branch or "DETACHED",
        "state": "DIRTY" if entries else "CLEAN",
        "status_entry_count": len(entries), "status_codes": dict(codes),
        "status_sha256": hashlib.sha256(state.stdout).hexdigest(),
        "sample_paths": [p for _, p in entries[:20]],
        "head_changed_during_observation": text(git(root, "rev-parse", "HEAD")) != text(head_before),
        "untracked_scope": "excluded-private-home" if untracked == "no" else "all-not-ignored",
        "divergence": divergence,
    }


def census(home, registry, extra):
    owners = collections.defaultdict(set)
    candidates = set()
    for agent, cfg in registry.get("agents", {}).items():
        for field in ("repo", "cwd"):
            if cfg.get(field):
                path = pathlib.Path(cfg[field]).expanduser()
                if path.exists():
                    candidates.add(path)
                    owners[path.resolve()].add(agent)
    for pattern in ("Development/*", "Development/.worktrees/*", ".codex/worktrees/*/*", ".sirsi/worktrees/*"):
        candidates.update(home.glob(pattern))
    candidates.update(pathlib.Path(p).expanduser() for p in extra)
    roots, common_owners, missing = {}, collections.defaultdict(set), []
    for path in sorted(candidates):
        if not path.is_dir() or not (path / ".git").exists():
            continue
        top = git(path, "rev-parse", "--show-toplevel")
        if top.returncode:
            continue
        root = pathlib.Path(text(top)).resolve()
        common = text(git(root, "rev-parse", "--path-format=absolute", "--git-common-dir"))
        roots[root] = common
        common_owners[common].update(owners[root])
    # Worktrees outside the conventional directories are still part of the fabric.
    enumerated = set()
    for root, common in list(roots.items()):
        if common in enumerated:
            continue
        enumerated.add(common)
        for path in worktrees(root):
            if not path.is_dir():
                missing.append({"path": str(path), "common_dir": common})
            else:
                roots[path.resolve()] = common
    rows = []
    for root, common in sorted(roots.items()):
        try:
            rows.append(inspect(root, common_owners[common], home))
        except (OSError, subprocess.TimeoutExpired) as exc:
            rows.append({"path": str(root), "state": "UNREADABLE", "error": type(exc).__name__})
    return {
        "schema": "maat.fabric-reconciliation.v1",
        "observed_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "host": os.uname().nodename, "read_only": True,
        "scope": "registered roots, conventional worktrees, and every Git-linked worktree; ignored caches excluded",
        "roots": rows, "missing_linked_worktrees": missing,
        "counts": dict(collections.Counter(r["state"] for r in rows)),
        "zero_uncommitted_proven": bool(rows) and all(r["state"] == "CLEAN" and not r.get("head_changed_during_observation") for r in rows) and not missing,
        "limits": ["Point-in-time observations, not a write freeze.", "Clean is not proof of committed, pushed, tested or deployed completion.", "No private credentials or file contents are read.", "Cloud and other hosts require their own receipts; local absence is not host failure."],
    }


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--home", default=str(pathlib.Path.home()))
    parser.add_argument("--registry", default=str(pathlib.Path.home() / ".sirsi/registry/agents.json"))
    parser.add_argument("--extra", action="append", default=[])
    args = parser.parse_args()
    try:
        with open(args.registry, encoding="utf-8") as handle:
            declared = json.load(handle)
        registry_available = True
    except FileNotFoundError:
        declared, registry_available = {"agents": {}}, False
    report = census(pathlib.Path(args.home).resolve(), declared, args.extra)
    report["registry_available"] = registry_available
    if not registry_available:
        report["zero_uncommitted_proven"] = False
        report["limits"].append("No local pinned registry; ownership mapping incomplete.")
    print(json.dumps(report, indent=2))
