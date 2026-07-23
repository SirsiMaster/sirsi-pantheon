#!/usr/bin/env python3
"""Reap completed CCD scheduled-task session leaks.

Completion-based, NOT age-based. A resident claude-desktop runner is reaped
IFF its CCD session:
  1. has a scheduledTaskId  (interactive/named sessions have none -> never touched)
  2. is NOT the newest instance for that task  (the running one has outstanding work)
  3. last did work > GRACE_MIN ago  (a just-finished turn gets a grace period)
and it is not the current process.

Why SIGKILL: the `disclaimer` wrapper ignores SIGTERM. These are completed
sessions with no outstanding work, so hard-kill is safe (they are not
load-bearing model servers — those live under ~/.sirsi/*.pid and are excluded
here because they carry no scheduledTaskId).

Usage: ccd-reap-completed.py [--apply]   (default: dry-run)
"""
import json, os, glob, subprocess, datetime, time, sys

GRACE_MIN = 10          # ponytail: fixed 10-min grace; widen if a run legitimately idles longer mid-task
MATCH_WINDOW_S = 180    # pid start must be within this of session createdAt to attribute
APPLY = "--apply" in sys.argv
now = time.time()
me = os.getpid()
base = os.path.expanduser("~/Library/Application Support/Claude/claude-code-sessions")

def epoch(v):
    if v is None: return None
    if isinstance(v, (int, float)): return v/1000.0 if v > 1e12 else float(v)
    try: return datetime.datetime.fromisoformat(str(v).replace("Z", "+00:00")).timestamp()
    except Exception: return None

# 1. load non-archived sessions
sessions = []
for f in glob.glob(base + "/**/local_*.json", recursive=True):
    try: d = json.load(open(f))
    except Exception: continue
    if d.get("isArchived"): continue
    sessions.append(dict(sid=d.get("sessionId"), created=epoch(d.get("createdAt")),
                         last=epoch(d.get("lastActivityAt")), sched=d.get("scheduledTaskId"),
                         path=f, title=d.get("title")))

# newest instance per scheduled task = the running one, protected
newest = {}
for s in sessions:
    if s["sched"] and s["last"]:
        newest[s["sched"]] = max(newest.get(s["sched"], 0), s["last"])

# 2. resident MAIN runners -> start epoch
def pid_start(pid):
    try:
        ls = subprocess.check_output(["ps", "-o", "lstart=", "-p", str(pid)], text=True).strip()
        return int(subprocess.check_output(["date", "-j", "-f", "%a %b %d %T %Y", ls, "+%s"], text=True).strip())
    except Exception: return None

try:
    pids = subprocess.check_output(
        ["pgrep", "-f", "claude-code/2.1.209/claude.app/Contents/MacOS/claude "], text=True).split()
except subprocess.CalledProcessError:
    pids = []

reap = []
for pid in map(int, pids):
    if pid == me: continue
    st = pid_start(pid)
    if st is None: continue
    best, bd = None, 1e9
    for s in sessions:
        if s["created"] is None: continue
        dd = abs(s["created"] - st)
        if dd < bd: bd, best = dd, s
    if best is None or bd > MATCH_WINDOW_S:      # unattributable -> never kill
        continue
    if not best["sched"]:                        # interactive/named -> protect
        continue
    if best["last"] and best["last"] == newest.get(best["sched"]):  # running instance -> protect
        continue
    idle_min = (now - best["last"]) / 60 if best["last"] else 1e9
    if idle_min < GRACE_MIN:                      # just finished -> grace
        continue
    reap.append((pid, best["sched"], round(idle_min)))

# 3. act: SIGKILL parent + children (TERM is ignored by disclaimer)
killed = 0
for pid, sched, idle in reap:
    targets = [pid]
    try: targets += [int(x) for x in subprocess.check_output(["pgrep", "-P", str(pid)], text=True).split()]
    except subprocess.CalledProcessError: pass
    for t in targets:
        if APPLY:
            try: os.kill(t, 9); killed += 1
            except ProcessLookupError: pass
            except PermissionError: print(f"  EPERM {t}", file=sys.stderr)
    print(f"{'KILL' if APPLY else 'WOULD-REAP'} pid={pid} task={sched} idle={idle}min")

print(f"\n{'reaped' if APPLY else 'dry-run: would reap'} {len(reap)} completed-leak session(s)"
      f"{f' ({killed} procs killed)' if APPLY else ''}; grace={GRACE_MIN}min")

# 4. archive pass (owner directive 2026-07-23: single continuous history, no manual archiving).
# Store-level isArchived flip — reversible from the app's Archived list, no prompts.
#   a) scheduled-task runs that are NOT the newest instance for their task -> completed routine run
#   b) untagged sessions idle > STALE_DAYS with no live process just reaped -> drift trash
STALE_DAYS = 7   # ponytail: fixed window; tune if owner's interactive sessions idle longer
reaped_pids = {p for p, _, _ in reap}
archived = 0
for s in sessions:
    idle_min = (now - s["last"]) / 60 if s["last"] else 1e9
    if s["sched"]:
        if s["last"] and s["last"] == newest.get(s["sched"]): continue   # running instance
        if idle_min < GRACE_MIN: continue
    else:
        if idle_min < STALE_DAYS * 1440: continue
    if APPLY:
        try:
            d = json.load(open(s["path"])); d["isArchived"] = True
            json.dump(d, open(s["path"], "w"))
        except Exception as e:
            print(f"  archive-fail {s['sid']}: {e}", file=sys.stderr); continue
    archived += 1
    print(f"{'ARCHIVE' if APPLY else 'WOULD-ARCHIVE'} {s['sid']} task={s['sched'] or '-'} title={s['title']!r} idle={round(idle_min)}min")
print(f"{'archived' if APPLY else 'would archive'} {archived} session record(s); stale-window={STALE_DAYS}d")
