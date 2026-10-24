#!/usr/bin/env python3
"""
Agent Completion Gate

Validates that an agent's claimed completion traces back to repo canon
documents and includes concrete working-status evidence.

Usage:
  python3 /Users/thekryptodragon/Development/tools/agent_completion_gate.py validate --repo .
  python3 /Users/thekryptodragon/Development/tools/agent_completion_gate.py check-contract --repo .
  python3 /Users/thekryptodragon/Development/tools/agent_completion_gate.py init-proof --repo . --work-item 20260630-example
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import sys
from pathlib import Path
from typing import Any


VALID_CLASSIFICATIONS = {
    "commercial-product",
    "pilot",
    "prototype",
    "internal-tool",
    "platform-foundation",
    "research",
}

FINAL_STATUSES = {"ready_for_review", "completed"}
PASS_STATUSES = {"pass", "passed"}


def fail(message: str) -> None:
    print(f"FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def load_json(path: Path) -> dict[str, Any]:
    try:
        with path.open("r", encoding="utf-8") as fh:
            data = json.load(fh)
    except FileNotFoundError:
        fail(f"missing file: {path}")
    except json.JSONDecodeError as exc:
        fail(f"invalid JSON in {path}: {exc}")
    if not isinstance(data, dict):
        fail(f"expected JSON object in {path}")
    return data


def write_json(path: Path, data: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as fh:
        json.dump(data, fh, indent=2)
        fh.write("\n")


def require_string(data: dict[str, Any], key: str, where: str) -> str:
    value = data.get(key)
    if not isinstance(value, str) or not value.strip():
        fail(f"{where}: missing non-empty string `{key}`")
    return value


def require_list(data: dict[str, Any], key: str, where: str) -> list[Any]:
    value = data.get(key)
    if not isinstance(value, list) or not value:
        fail(f"{where}: missing non-empty list `{key}`")
    return value


def repo_path(repo: Path, maybe_relative: str) -> Path:
    path = Path(maybe_relative)
    return path if path.is_absolute() else repo / path


def validate_contract(repo: Path, contract: dict[str, Any]) -> list[str]:
    require_string(contract, "repo_name", "contract")
    require_string(contract, "release_target", "contract")
    classification = require_string(contract, "classification", "contract")
    if classification not in VALID_CLASSIFICATIONS:
        fail(f"contract: invalid classification `{classification}`")

    canon_docs = require_list(contract, "canon_documents", "contract")
    required_doc_paths: list[str] = []
    for idx, item in enumerate(canon_docs):
        where = f"contract.canon_documents[{idx}]"
        if not isinstance(item, dict):
            fail(f"{where}: expected object")
        path_text = require_string(item, "path", where)
        role = require_string(item, "role", where)
        path = repo_path(repo, path_text)
        if not path.exists():
            fail(f"{where}: missing canon doc `{path_text}`")
        if path.is_file() and path.stat().st_size == 0:
            fail(f"{where}: empty canon doc `{path_text}`")
        if role in {"prd", "requirements", "scope", "test-plan", "release-plan"}:
            required_doc_paths.append(path_text)

    done_definition = require_list(contract, "done_definition", "contract")
    seen_categories = set()
    for idx, item in enumerate(done_definition):
        where = f"contract.done_definition[{idx}]"
        if not isinstance(item, dict):
            fail(f"{where}: expected object")
        category = require_string(item, "category", where)
        seen_categories.add(category)
        criteria = require_list(item, "criteria", where)
        if not all(isinstance(c, str) and c.strip() for c in criteria):
            fail(f"{where}.criteria: all criteria must be non-empty strings")

    for category in ["product", "design", "technical", "operational", "narrative"]:
        if category not in seen_categories:
            fail(f"contract.done_definition: missing `{category}` category")

    required_commands = require_list(contract, "required_verification_commands", "contract")
    for idx, item in enumerate(required_commands):
        where = f"contract.required_verification_commands[{idx}]"
        if not isinstance(item, dict):
            fail(f"{where}: expected object")
        require_string(item, "name", where)
        require_string(item, "command", where)

    return required_doc_paths


def validate_proof(repo: Path, proof_path: Path, proof: dict[str, Any], required_docs: list[str], *, require_completed: bool = False) -> None:
    # Completion authority and all semantic checks consume this one loaded object.
    # Standalone validation intentionally still supports draft/blocked evidence.
    if require_completed:
        if proof.get("status") != "completed":
            fail("proof: completion requires status `completed`")
        blockers = proof.get("blockers", [])
        if not isinstance(blockers, list) or blockers:
            fail("proof: completion requires an empty blockers list")
    require_string(proof, "work_item_id", "proof")
    require_string(proof, "agent_id", "proof")
    proof_repo = require_string(proof, "repo", "proof")
    if Path(proof_repo).resolve() != repo.resolve():
        fail(f"proof.repo `{proof_repo}` does not match repo `{repo}`")

    status = require_string(proof, "status", "proof")
    if status not in {"draft", "blocked", "ready_for_review", "completed"}:
        fail(f"proof: invalid status `{status}`")

    classification = require_string(proof, "classification", "proof")
    if classification not in VALID_CLASSIFICATIONS:
        fail(f"proof: invalid classification `{classification}`")

    canon_read = require_list(proof, "canon_read", "proof")
    canon_read_paths = set()
    for idx, item in enumerate(canon_read):
        where = f"proof.canon_read[{idx}]"
        if not isinstance(item, dict):
            fail(f"{where}: expected object")
        path_text = require_string(item, "path", where)
        require_string(item, "read_at", where)
        canon_read_paths.add(path_text)
        if not repo_path(repo, path_text).exists():
            fail(f"{where}: referenced missing canon doc `{path_text}`")

    if status in FINAL_STATUSES:
        missing = [path for path in required_docs if path not in canon_read_paths]
        if missing:
            fail(f"proof.canon_read missing required canon docs: {', '.join(missing)}")

    trace = require_list(proof, "requirements_trace", "proof")
    for idx, item in enumerate(trace):
        where = f"proof.requirements_trace[{idx}]"
        if not isinstance(item, dict):
            fail(f"{where}: expected object")
        doc = require_string(item, "doc", where)
        require_string(item, "section", where)
        require_string(item, "claim", where)
        require_list(item, "files_changed", where)
        require_list(item, "evidence", where)
        if doc not in canon_read_paths:
            fail(f"{where}: doc `{doc}` was not listed in canon_read")

    closure = proof.get("closure_evidence")
    if not isinstance(closure, dict):
        fail("proof: missing object `closure_evidence`")
    for key in ["product", "design", "technical", "operational", "narrative"]:
        value = closure.get(key)
        if not isinstance(value, str) or not value.strip():
            fail(f"proof.closure_evidence: missing non-empty `{key}`")

    verifications = require_list(proof, "verification", "proof")
    for idx, item in enumerate(verifications):
        where = f"proof.verification[{idx}]"
        if not isinstance(item, dict):
            fail(f"{where}: expected object")
        require_string(item, "name", where)
        require_string(item, "command", where)
        item_status = require_string(item, "status", where).lower()
        require_string(item, "evidence", where)
        if status in FINAL_STATUSES and item_status not in PASS_STATUSES:
            fail(f"{where}: final proof requires passing status, got `{item_status}`")

    working_status = proof.get("working_status")
    if not isinstance(working_status, dict):
        fail("proof: missing object `working_status`")
    for key in ["environment", "how_verified", "observed_result", "verified_at"]:
        require_string(working_status, key, "proof.working_status")

    handoffs = proof.get("handoffs", [])
    if not isinstance(handoffs, list):
        fail("proof.handoffs: expected list")
    for idx, item in enumerate(handoffs):
        where = f"proof.handoffs[{idx}]"
        if not isinstance(item, dict):
            fail(f"{where}: expected object")
        require_string(item, "from", where)
        require_string(item, "to", where)
        require_string(item, "router_item", where)
        require_string(item, "status", where)

    blockers = proof.get("blockers", [])
    if status == "blocked" and not blockers:
        fail("proof.blockers: blocked status requires at least one blocker")

    print(f"PASS: {proof_path} satisfies completion gate for {repo}")


def init_proof(repo: Path, work_item: str, agent_id: str) -> None:
    contract = load_json(repo / ".agents" / "completion.contract.json")
    now = dt.datetime.now(dt.timezone.utc).isoformat()
    canon_docs = contract.get("canon_documents", [])
    proof = {
        "work_item_id": work_item,
        "agent_id": agent_id,
        "repo": str(repo.resolve()),
        "status": "draft",
        "classification": contract.get("classification", "prototype"),
        "canon_read": [
            {
                "path": doc.get("path", ""),
                "role": doc.get("role", ""),
                "read_at": now,
                "notes": ""
            }
            for doc in canon_docs
            if isinstance(doc, dict)
        ],
        "requirements_trace": [
            {
                "doc": "",
                "section": "",
                "claim": "",
                "files_changed": [],
                "evidence": []
            }
        ],
        "closure_evidence": {
            "product": "",
            "design": "",
            "technical": "",
            "operational": "",
            "narrative": ""
        },
        "verification": [
            {
                "name": cmd.get("name", ""),
                "command": cmd.get("command", ""),
                "status": "not-run",
                "evidence": ""
            }
            for cmd in contract.get("required_verification_commands", [])
            if isinstance(cmd, dict)
        ],
        "working_status": {
            "environment": "",
            "how_verified": "",
            "observed_result": "",
            "verified_at": ""
        },
        "handoffs": [],
        "blockers": []
    }
    proof_path = repo / ".agents" / "proofs" / f"{work_item}.json"
    write_json(proof_path, proof)
    print(proof_path)


def main() -> None:
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest="cmd", required=True)

    validate = sub.add_parser("validate")
    validate.add_argument("--repo", default=".")
    validate.add_argument("--proof", default=None)
    validate.add_argument("--require-completed", action="store_true",
                          help="require completed, unblocked evidence in the same loaded proof")

    completion = sub.add_parser("validate-completion")
    completion.add_argument("--repo", default=".")
    completion.add_argument("--proof", required=True,
                            help="original proof path for diagnostics; document comes only from stdin")

    check_contract = sub.add_parser("check-contract")
    check_contract.add_argument("--repo", default=".")

    init = sub.add_parser("init-proof")
    init.add_argument("--repo", default=".")
    init.add_argument("--work-item", required=True)
    init.add_argument("--agent-id", default=os.environ.get("SIRSI_AGENT_ID", "unknown-agent"))

    args = parser.parse_args()
    repo = Path(args.repo).resolve()

    if args.cmd == "init-proof":
        init_proof(repo, args.work_item, args.agent_id)
        return

    contract_path = repo / ".agents" / "completion.contract.json"
    contract = load_json(contract_path)
    required_docs = validate_contract(repo, contract)

    if args.cmd == "check-contract":
        print(f"PASS: {contract_path} canon contract is valid for {repo}")
        return

    proof_arg = args.proof
    if proof_arg is None:
        latest_path = repo / ".agents" / "completion.proof.json"
        if latest_path.exists():
            proof_path = latest_path
        else:
            proof_dir = repo / ".agents" / "proofs"
            proofs = sorted(proof_dir.glob("*.json"), key=lambda p: p.stat().st_mtime, reverse=True) if proof_dir.exists() else []
            if not proofs:
                fail("no proof supplied and no .agents/completion.proof.json or .agents/proofs/*.json exists")
            proof_path = proofs[0]
    else:
        proof_path = repo_path(repo, proof_arg)

    if args.cmd == "validate-completion":
        raw = sys.stdin.buffer.read(4 * 1024 * 1024 + 1)
        if len(raw) > 4 * 1024 * 1024:
            fail("completion proof exceeds 4 MiB")
        try:
            proof = json.loads(raw.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            fail(f"invalid completion proof on stdin: {exc}")
        if not isinstance(proof, dict):
            fail("expected completion JSON object on stdin")
    else:
        proof = load_json(proof_path)
    validate_proof(repo, proof_path, proof, required_docs,
                   require_completed=(args.cmd == "validate-completion" or args.require_completed))


if __name__ == "__main__":
    main()
