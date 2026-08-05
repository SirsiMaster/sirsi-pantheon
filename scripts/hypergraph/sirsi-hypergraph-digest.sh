#!/bin/bash
# sirsi-hypergraph-digest.sh — P1 machine→fleet digest producer (ADR-003 gate G2).
# Sweep feeders → replay union bus → emit a FIXED-SIZE digest (counts, repo names,
# timestamps, hashes only — Rule 26 firewall; never content). Local-only by default:
# egress requires SIRSI_HYPERGRAPH_DIGEST_EGRESS=1 (owner-enabled, H8 amendment).
# mkdir-locked (macOS has no flock): concurrent ticks no-op. Cost of each step is measured and recorded.
set -u
export PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
HG="$HOME/.local/bin/hypergraph"
OUT_DIR="$HOME/.sirsi/hypergraph"
DIGEST="$OUT_DIR/digest.json"
LOCK="$OUT_DIR/digest.lock.d"
LOG="$HOME/.sirsi/logs/hypergraph-digest.log"
mkdir -p "$OUT_DIR" "$(dirname "$LOG")"
# mkdir is atomic on APFS: if the lock dir exists a tick is already running.
if ! mkdir "$LOCK" 2>/dev/null; then exit 0; fi
trap 'rmdir "$LOCK" 2>/dev/null' EXIT

ts() { date -u +%Y-%m-%dT%H:%M:%SZ; }
ms() { python3 -c 'import time; print(int(time.time()*1000))'; }

T0=$(ms)
"$HG" sweep --root "$HOME/Development" >>"$LOG" 2>&1
T1=$(ms)
"$HG" replay --repo "$HOME" >>"$LOG" 2>&1
T2=$(ms)
STATUS_JSON=$("$HG" status --repo "$HOME" --json 2>>"$LOG")
T3=$(ms)
# Anchor leg (ADR-004, sovereign local node). Anchor the unanchored set each tick so
# the fleet level stays current without human action; idempotent by construction
# (only events lacking an anchor row are published). Failure is non-fatal: the
# digest still publishes, the next tick retries (degradation law).
"$HG" hcs anchor --repo "$HOME" >>"$LOG" 2>&1 || echo "[$(ts)] anchor pass skipped (node unreachable)" >>"$LOG"
ANCHOR_LINE=$("$HG" hcs status --repo "$HOME" 2>/dev/null | tr -d '\n')
T4=$(ms)

python3 - "$DIGEST" "$((T1-T0))" "$((T2-T1))" "$((T3-T2))" "$((T4-T3))" "$ANCHOR_LINE" <<'PY' >>"$LOG" 2>&1
import json, sys, hashlib, os, time
digest_path, sweep_ms, replay_ms, status_ms = sys.argv[1], int(sys.argv[2]), int(sys.argv[3]), int(sys.argv[4])
anchor_ms = int(sys.argv[5]) if len(sys.argv) > 5 else 0
anchor_raw = sys.argv[6] if len(sys.argv) > 6 else ""
import re as _re
_m = _re.search(r"(\d+)\s*/\s*(\d+)\s+events anchored", anchor_raw)
_anchored, _total = (int(_m.group(1)), int(_m.group(2))) if _m else (0, 0)
status = json.loads(os.popen(f"{os.path.expanduser('~')}/.local/bin/hypergraph status --repo {os.path.expanduser('~')} --json").read())
# FIXED-SIZE by construction: counts, repo names (sirsi:// URIs), timestamps, hash.
# No event content, no filesystem paths, no type payloads (Rule 26 / ADR-003).
#
# That claim used to be a comment the code did not enforce: three insight events
# carry repo="/Users/<user>" (a pre-fix `insight --repo ~`), and this producer
# passed repo keys through verbatim — putting the OS username into the one
# artifact designed to leave the machine. Those events are anchored and
# immutable, so the record stays honest and the EGRESS layer normalizes.
_repos = {}
for _k, _v in status.get("projected_repos", {}).items():
    _key = _k if _k.startswith("sirsi://") else "sirsi://fabric"
    _repos[_key] = _repos.get(_key, 0) + _v
# The anchor-freshness invariant (hypergraph hcs check). A five-hour silent
# notarization outage on 2026-07-26 was invisible because every command
# happily printed a number and returned 0. This runs it every tick and puts
# the verdict IN the digest, so a stall becomes a fleet-visible signal rather
# than something a human stumbles over. Degradation law: an older binary
# without `hcs check` reports unknown, it does not fail the digest.
# Structure freshness as a FLEET signal (ADR-003: compact signals upward). The
# projection can now report how old each repo's structure snapshot is; the digest
# is where that becomes visible beyond this machine.
_hg = f"{os.path.expanduser('~')}/.local/bin/hypergraph"
_health = {"healthy": None, "reason": "hcs check unavailable — installed binary predates it; reinstall with `go build -o ~/.local/bin/hypergraph ./cmd/hypergraph`"}
_raw = os.popen(f"{_hg} hcs check --repo {os.path.expanduser('~')} --json 2>/dev/null").read().strip()
if _raw.startswith("{"):
    # Only parse what LOOKS like JSON. An older binary prints a cobra usage
    # error here, and feeding that to json.loads produced the useless reason
    # "Expecting value: line 1 column 1" — an honest null with an unhelpful
    # explanation is still a surface that fails to say what is wrong (IO7).
    try:
        _health = json.loads(_raw)
    except Exception as _e:
        _health = {"healthy": None, "reason": f"hcs check returned unparseable JSON: {_e}"}

# Structure freshness as a FLEET signal (ADR-003: compact signals upward).
#
# NOTE the placement: this must come AFTER _hg is defined. It was first written
# above that assignment, raised NameError, and a bare `except Exception` reported
# it as "no data" — a programming error wearing the costume of an empty result,
# which is precisely the silent-degradation class this producer exists to avoid.
# The handler is now narrow, and an unexpected failure is recorded, not hidden.
_structure = []
_structure_error = ""
try:
    _sraw = os.popen(f"{_hg} status --repo {os.path.expanduser('~')} --json 2>/dev/null").read().strip()
    if _sraw.startswith("{"):
        _structure = json.loads(_sraw).get("structure_freshness", [])
    else:
        _structure_error = "status --json produced no JSON (binary predates structure_freshness?)"
except (ValueError, OSError) as _e:
    _structure_error = f"structure freshness unavailable: {_e}"

# The classifier REFUSED this field when first added, and was right to:
# structure_freshness names repos — the same portfolio inventory that put
# per_repo_event_counts in LOCAL_ONLY. So the detail stays local and the fleet
# gets a NAME-FREE aggregate. Across a machine boundary the useful question is
# "is any structure badly stale?", never "which repo is". The guard did not just
# block a mistake; it forced the better shape.
_structure_summary = {
    "repos_with_structure": len(_structure),
    "oldest_age_days": max([f.get("age_days", 0) for f in _structure], default=0),
    "unknown_age": sum(1 for f in _structure if f.get("unknown")),
}
if _structure_error:
    _structure_summary["unavailable_reason"] = _structure_error


body = {
  "schema": "sirsi.hypergraph.digest/v1",
  "produced_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
  "machine": "m5-sirsi",
  "totals": {"events": status.get("events", 0), "repos": len(status.get("projected_repos", {}))},
  "per_repo_event_counts": _repos,
  "per_type_event_counts": status.get("projected_types", {}),
  "anchors": {"anchored": _anchored, "total": _total, "pending": max(_total - _anchored, 0)},
  "anchor_health": _health,
  "structure_freshness": _structure,
  "structure_summary": _structure_summary,
  "step_cost_ms": {"sweep": sweep_ms, "replay": replay_ms, "status": status_ms, "anchor_status": anchor_ms},
}
# --- FIELD-BY-FIELD EGRESS CLASSIFICATION (claude-nexus ask, 2026-07-27) ---
#
# Three identity leaks were found in this one artifact in a single week: host
# identity, a portfolio inventory in the per-repo keys, and the owner's OS
# username via a raw filesystem path. All three are fixed. The point nexus made
# is that they were DISCOVERED INCREMENTALLY rather than enumerated, and three
# findings in one artifact measures how well it is understood, not luck.
#
# So every top-level field is classified here, explicitly, and the producer
# FAILS CLOSED on any field that is not: adding a field without classifying it
# refuses to write a digest at all. That is what closes the leak class by
# construction rather than one finding at a time.
#
# LOCAL_ONLY does NOT mean secret — the full digest is written locally and any
# component on owned silicon may read it (ADR-005 seam #2, corrected: the seam
# is local-vs-cloud, not pillar-vs-pillar). It means the field may not cross the
# MACHINE boundary.
EGRESS_SAFE = {
  "schema",                  # version string
  "produced_at",             # timestamp
  "totals",                  # counts only
  "per_type_event_counts",   # event-type mix; no repo or host identity
  "anchors",                 # counts only
  "anchor_health",           # verdict + reason; reason text is path-scrubbed below
  "step_cost_ms",            # timings
  "digest_sha256",           # hash of the whole body
  "structure_summary",       # counts + max age; names no repo
}
LOCAL_ONLY = {
  "machine",                 # host identity
  "per_repo_event_counts",   # portfolio inventory: names unreleased products
  "structure_freshness",     # per-repo detail — names repos, same inventory
}

_unclassified = set(body) - EGRESS_SAFE - LOCAL_ONLY
if _unclassified:
    raise SystemExit(
        f"REFUSING to write digest: unclassified field(s) {sorted(_unclassified)}. "
        "Every field must be declared EGRESS_SAFE or LOCAL_ONLY before it can ship.")

payload = json.dumps(body, sort_keys=True, separators=(",",":"))
_home = os.path.expanduser("~")
if _home in payload or os.path.basename(_home) in payload:
    raise SystemExit(f"REFUSING to write digest: it contains the home path or username ({_home}). H8 egress guard.")
body["digest_sha256"] = hashlib.sha256(payload.encode()).hexdigest()
tmp = digest_path + ".tmp"
open(tmp,"w").write(json.dumps(body, indent=1, sort_keys=True))
os.replace(tmp, digest_path)
if _health.get("healthy") is False:
    print(f"[{body['produced_at']}] ANCHOR STALL: {_health.get('reason','')}")
print(f"[{body[chr(39)+'produced_at'+chr(39)] if False else body['produced_at']}] digest ok events={body['totals']['events']} repos={body['totals']['repos']} sweep={sweep_ms}ms replay={replay_ms}ms status={status_ms}ms sha={body['digest_sha256'][:12]}")
PY
GEN_RC=$?

# The "fail-closed hard stop" above was not one. This script runs `set -u`, NOT
# `set -e`, so a SystemExit from the classifier (unclassified field, identity
# leak) left the shell running: it continued into egress and cartography and
# exited ZERO, while the PREVIOUS digest.json stayed on disk. Monitoring saw a
# successful tick and consumers saw a stale artifact — exactly the silent-failure
# class this producer claims to close. (codex-home review 2026-07-27, P1.)
#
# The Python side already writes atomically (tmp + os.replace), so a refusal
# leaves no partial artifact; what was missing was the process telling anyone.
if [ "$GEN_RC" -ne 0 ]; then
  echo "[$(ts)] DIGEST GENERATION REFUSED (rc=$GEN_RC) - no artifact written; previous digest.json is now STALE" >>"$LOG"
  exit "$GEN_RC"
fi

# Egress: OFF unless the owner enables it (H8 amendment). P2 wires the cloud target.
if [ "${SIRSI_HYPERGRAPH_DIGEST_EGRESS:-0}" = "1" ]; then
  # The egress projection is built by SUBTRACTION from the classification, so a
  # newly added field is excluded until someone classifies it. Fail-closed, not
  # fail-open: the default for anything unknown is "does not leave".
  python3 - "$DIGEST" >>"$LOG" 2>&1 <<'PYEG'
import json, os, sys
digest_path = sys.argv[1]
body = json.load(open(digest_path))
LOCAL_ONLY = {"machine", "per_repo_event_counts", "structure_freshness"}
EGRESS_SAFE = {"schema","produced_at","totals","per_type_event_counts",
               "anchors","anchor_health","step_cost_ms","digest_sha256",
               "structure_summary"}
unclassified = set(body) - EGRESS_SAFE - LOCAL_ONLY
if unclassified:
    raise SystemExit(f"REFUSING egress projection: unclassified field(s) {sorted(unclassified)}")
out = {k: v for k, v in body.items() if k in EGRESS_SAFE}
raw = json.dumps(out, sort_keys=True)
home = os.path.expanduser("~")
if home in raw or os.path.basename(home) in raw:
    raise SystemExit("REFUSING egress projection: contains home path or username")
p = digest_path.replace(".json", "-egress.json")
open(p + ".tmp", "w").write(json.dumps(out, indent=1, sort_keys=True))
os.replace(p + ".tmp", p)
print(f"egress projection written: {len(out)} of {len(body)} fields ({sorted(set(body)-EGRESS_SAFE)} withheld)")
PYEG
  echo "[$(ts)] egress enabled; projection written, no remote target wired yet (P2)" >>"$LOG"
fi

# Architecture cartography (owner mandate 2026-07-24, permanent chore): regenerate the
# generated diagram set from the live system each tick so it can never drift. Non-fatal.
CARTO="$HOME/Development/SirsiNexusApp/scripts/cartography.py"
if [ -f "$CARTO" ]; then
  python3 "$CARTO" --out "$HOME/Development/SirsiNexusApp/docs/architecture" >>"$LOG" 2>&1 \
    || echo "[$(ts)] cartography skipped" >>"$LOG"
  python3 "${CARTO%cartography.py}cartography_svg.py" \
      --out "$HOME/Development/SirsiNexusApp/docs/architecture/svg" >>"$LOG" 2>&1 \
    || echo "[$(ts)] cartography(svg) skipped" >>"$LOG"
fi

# ─────────────────────────────────────────────────────────────────────────────
# Hedera config-reversion guard (added 2026-07-29 by claude-nexus)
#
# The sovereign node's JVM heap ceiling lives in an .env inside the NPX CACHE
# (~/.npm/_npx/<hash>/node_modules/@hashgraph/hedera-local/.env), which is
# regenerated whenever npx re-resolves the package. It has silently reverted
# 4g -> 2g at least once, and 2 GiB is the exact ceiling that caused the repeat
# consensus-node OOM class. The reversion is INVISIBLE while the node keeps
# running on its start-time value, and only bites on the NEXT restart — hours or
# weeks later, detached from the cause.
#
# So this reports it loudly on every tick rather than trusting anyone to
# remember. It does NOT auto-repair: silently rewriting a file in a cache the
# package manager owns would hide the churn we need to see.
HEDERA_ENV=$(ls -d "$HOME"/.npm/_npx/*/node_modules/@hashgraph/hedera-local/.env 2>/dev/null | head -1)
if [ -n "$HEDERA_ENV" ]; then
  HEAP=$(grep -m1 '^PLATFORM_JAVA_HEAP_MAX=' "$HEDERA_ENV" 2>/dev/null | cut -d= -f2)
  case "$HEAP" in
    4g|8g|1[0-9]g) : ;;  # 4g or larger — fine
    *) echo "[$(ts)] WARN hedera heap reverted to '${HEAP:-unset}' in $HEDERA_ENV — 2g is the known OOM ceiling; next node restart will crash-loop. Set PLATFORM_JAVA_HEAP_MAX=4g." >>"$LOG" ;;
  esac
fi
