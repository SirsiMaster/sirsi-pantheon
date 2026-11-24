#!/usr/bin/env python3
"""Pantheon production completion protocol v1 (stdlib only).

Source authority: this versioned product file, embedded by completionproof.
Standalone exports are byte-identical; no caller-selected helper is consulted.
This validates declarations, not the truth of test receipts or actual delivery.
No command in a contract is ever executed by this program.
"""

import argparse
import datetime
import json
import math
import os
from pathlib import Path
import stat
import sys

LIMIT = 4 * 1024 * 1024
CATEGORIES = ("product", "design", "technical", "operational", "narrative")
CLASSIFICATIONS = {"commercial-product", "pilot", "prototype", "internal-tool", "platform-foundation", "research"}
STATUSES = {"draft", "blocked", "ready_for_review", "completed"}
FINAL = {"ready_for_review", "completed"}
READ_REQUIRED = {"prd", "requirements", "scope", "test-plan", "release-plan"}


class Invalid(ValueError):
    pass


def require(condition, message):
    if not condition:
        raise Invalid(message)


def text(obj, key):
    require(isinstance(obj, dict), "expected object")
    value = obj.get(key)
    require(isinstance(value, str) and bool(value.strip()), "missing string: " + key)
    return value


def rows(obj, key):
    require(isinstance(obj, dict), "expected object")
    value = obj.get(key)
    require(isinstance(value, list) and bool(value), "missing list: " + key)
    return value


def strings(obj, key):
    value = rows(obj, key)
    require(all(isinstance(v, str) and bool(v.strip()) for v in value), "invalid string list: " + key)
    return value


def unique_pairs(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, "duplicate JSON key")
        result[key] = value
    return result


def invalid_constant(_):
    raise Invalid("nonfinite JSON constant")


def finite_float(value):
    number = float(value)
    require(math.isfinite(number), "nonfinite JSON number")
    return number


def document(raw):
    require(len(raw) <= LIMIT, "JSON exceeds byte limit")
    obj = json.loads(raw.decode("utf-8"), object_pairs_hook=unique_pairs,
                     parse_constant=invalid_constant, parse_float=finite_float)
    require(isinstance(obj, dict), "expected JSON object")
    return obj


def load(path):
    # Contract and diagnostic proof reads also refuse FIFO/device blocking and
    # symlink leaves. Canon paths may be absolute per the portfolio contract.
    fd = os.open(path, os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW)
    with os.fdopen(fd, "rb") as stream:
        info = os.fstat(stream.fileno())
        require(stat.S_ISREG(info.st_mode), "JSON must be a regular file")
        require(info.st_size <= LIMIT, "JSON exceeds byte limit")
        return document(stream.read(LIMIT + 1))


def path_in(repo, value):
    path = Path(value)
    return path if path.is_absolute() else repo / path


def nonempty_file(path):
    info = path.stat()
    require(stat.S_ISREG(info.st_mode) and info.st_size > 0, "canon must be a nonempty file")


def contract_policy(repo, contract):
    text(contract, "repo_name")
    text(contract, "release_target")
    require(text(contract, "classification") in CLASSIFICATIONS, "invalid contract classification")
    required_docs = set()
    for row in rows(contract, "canon_documents"):
        path = text(row, "path")
        role = text(row, "role")
        nonempty_file(path_in(repo, path))
        if role in READ_REQUIRED:
            required_docs.add(path)
    categories = set()
    for row in rows(contract, "done_definition"):
        category = text(row, "category")
        require(category not in categories, "duplicate done category")
        categories.add(category)
        strings(row, "criteria")
    require(set(CATEGORIES) <= categories, "missing done category")
    commands = {}
    for row in rows(contract, "required_verification_commands"):
        name, command = text(row, "name"), text(row, "command")
        require(name not in commands, "duplicate required command identity")
        commands[name] = command
    return required_docs, commands


def validate_proof(repo, contract, proof, policy, require_completed=False, expected_id=None):
    status = text(proof, "status")
    require(status in STATUSES, "invalid proof status")
    blockers = proof.get("blockers", [])
    require(isinstance(blockers, list) and all(isinstance(v, str) and bool(v.strip()) for v in blockers), "invalid blockers")
    if require_completed:
        require(status == "completed", "completion requires completed status")
        require(not blockers, "completion requires empty blockers")
    work_item = text(proof, "work_item_id")
    if expected_id is not None:
        require(isinstance(expected_id, str) and bool(expected_id.strip()), "missing exact expected item")
        require(work_item == expected_id, "work item does not match exact expected item")
    text(proof, "agent_id")
    require(Path(text(proof, "repo")).resolve() == repo, "proof repo mismatch")
    require(text(proof, "classification") == text(contract, "classification"), "proof classification does not match contract")

    required_docs, commands = policy
    read = set()
    for row in rows(proof, "canon_read"):
        path = text(row, "path")
        text(row, "read_at")
        nonempty_file(path_in(repo, path))
        read.add(path)
    if status in FINAL:
        require(required_docs <= read, "required canon not read")
    for row in rows(proof, "requirements_trace"):
        require(text(row, "doc") in read, "trace doc not read")
        text(row, "section")
        text(row, "claim")
        strings(row, "files_changed")
        strings(row, "evidence")
    closure = proof.get("closure_evidence")
    for category in CATEGORIES:
        text(closure, category)
    seen = {}
    for row in rows(proof, "verification"):
        name, command = text(row, "name"), text(row, "command")
        require(name not in seen, "duplicate verification identity")
        seen[name] = command
        outcome = text(row, "status").lower()
        text(row, "evidence")
        if status in FINAL:
            require(outcome in {"pass", "passed"}, "final verification not passing")
    if status in FINAL:
        # Every required name must occur once, with its exact command, not just
        # one successful arbitrary check. Additional passing checks are allowed.
        require(all(seen.get(name) == command for name, command in commands.items()), "missing or mismatched required command")
    for key in ("environment", "how_verified", "observed_result", "verified_at"):
        text(proof.get("working_status"), key)
    handoffs = proof.get("handoffs", [])
    require(isinstance(handoffs, list), "handoffs must be a list")
    for row in handoffs:
        for key in ("from", "to", "router_item", "status"):
            text(row, key)
    if status == "blocked":
        require(bool(blockers), "blocked proof needs a blocker")


def init_proof(repo, contract, work_item, agent):
    # Do not turn item identity into a directory traversal or overwrite a proof.
    require(bool(work_item.strip()) and work_item not in {".", ".."} and not any(c in work_item for c in "/\\\x00"), "unsafe work item filename")
    require(bool(agent.strip()), "missing agent")
    now = datetime.datetime.now(datetime.timezone.utc).isoformat()
    proof = {
        "work_item_id": work_item, "agent_id": agent, "repo": str(repo),
        "classification": contract["classification"], "status": "draft",
        "canon_read": [{"path": row["path"], "role": row["role"], "read_at": now, "notes": ""} for row in contract["canon_documents"]],
        "requirements_trace": [{"doc": "", "section": "", "claim": "", "files_changed": [], "evidence": []}],
        "closure_evidence": {key: "" for key in CATEGORIES},
        "verification": [{"name": row["name"], "command": row["command"], "status": "not-run", "evidence": ""} for row in contract["required_verification_commands"]],
        "working_status": {key: "" for key in ("environment", "how_verified", "observed_result", "verified_at")},
        "handoffs": [], "blockers": [],
    }
    # Anchored parents; refuse symlinks and leave existing proof bytes intact.
    fd = os.open(repo, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        for part in (".agents", "proofs"):
            try:
                os.mkdir(part, mode=0o700, dir_fd=fd)
            except FileExistsError:
                pass
            child = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
            os.close(fd)
            fd = child
        out = os.open(work_item + ".json", os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600, dir_fd=fd)
        with os.fdopen(out, "w", encoding="utf-8") as stream:
            json.dump(proof, stream, indent=2)
            stream.write("\n")
            stream.flush()
            os.fsync(stream.fileno())
    finally:
        os.close(fd)
    print(repo / ".agents" / "proofs" / (work_item + ".json"))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="mode", required=True)
    for name in ("check-contract", "init-proof", "validate", "validate-completion"):
        child = sub.add_parser(name)
        child.add_argument("--repo", default=".")
        if name == "init-proof":
            child.add_argument("--work-item", required=True)
            child.add_argument("--agent-id", default="unknown-agent")
        if name in {"validate", "validate-completion"}:
            child.add_argument("--proof", required=(name == "validate-completion"))
            child.add_argument("--expected-item-id", required=(name == "validate-completion"))
        if name == "validate":
            child.add_argument("--require-completed", action="store_true")
            child.add_argument("--proof-stdin", action="store_true")
    args = parser.parse_args()
    repo = Path(args.repo).resolve(strict=True)
    contract = load(repo / ".agents" / "completion.contract.json")
    policy = contract_policy(repo, contract)
    if args.mode == "check-contract":
        print("PASS: completion contract")
        return
    if args.mode == "init-proof":
        init_proof(repo, contract, args.work_item, args.agent_id)
        return
    if args.mode == "validate-completion" or args.proof_stdin:
        # --proof is display metadata ONLY on these paths. Never open it.
        proof = document(sys.stdin.buffer.read(LIMIT + 1))
    else:
        if args.proof is None:
            path = repo / ".agents" / "completion.proof.json"
            if not path.exists():
                choices = list((repo / ".agents" / "proofs").glob("*.json"))
                require(bool(choices), "no proof supplied")
                path = max(choices, key=lambda p: p.stat().st_mtime_ns)
        else:
            path = path_in(repo, args.proof)
        resolved = path.resolve(strict=True)
        require(repo in resolved.parents, "proof outside repo")
        proof = load(path)
    validate_proof(repo, contract, proof, policy,
                   require_completed=(args.mode == "validate-completion" or args.require_completed),
                   expected_id=args.expected_item_id)
    print("PASS: completion proof")


if __name__ == "__main__":
    try:
        main()
    except (Invalid, OSError, ValueError, TypeError, RecursionError):
        # Fixed diagnostic: never echo proof text, private paths or parser input.
        print("FAIL: completion input refused", file=sys.stderr)
        sys.exit(1)
