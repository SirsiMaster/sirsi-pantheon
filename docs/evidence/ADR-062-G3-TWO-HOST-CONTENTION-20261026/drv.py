#!/usr/bin/env python3
"""G3 round driver: fire one `sirsi router task claim-id` per round at a shared wall-clock schedule.
Usage: drv.py <sirsi> <url> <agent-id> <owner> <prefix> <n> <t0> <period> <out.tsv>   (token on stdin)
Writes: round  rc  ms  class   (class: win | refused | other:<first line>)"""
import os, random, subprocess, sys, time

sirsi, url, agent, owner, prefix, n, t0, period, out = sys.argv[1:10]
n, t0, period = int(n), float(t0), float(period)
tok = sys.stdin.readline().strip()
env = dict(os.environ, SIRSI_ROUTER_URL=url, SIRSI_ROUTER_TOKEN=tok, SIRSI_AGENT_ID=agent)
rng = random.Random(f"{agent}-{prefix}")
with open(out, "w") as f:
    for i in range(n):
        at = t0 + i * period + rng.uniform(0, 0.03)  # up to 30 ms jitter so arrival order varies
        d = at - time.time()
        if d > 0:
            time.sleep(d)
        s = time.time()
        p = subprocess.run([sirsi, "router", "task", "claim-id", owner, f"{prefix}{i:04d}",
                            "--worker", agent, "--thread", agent], env=env, capture_output=True, text=True)
        ms = (time.time() - s) * 1000
        if p.returncode == 0:
            cls = "win"
        elif "no claimable task" in p.stderr:
            cls = "refused"
        else:
            cls = "other:" + (p.stderr.strip().splitlines() or ["?"])[0][:160]
        f.write(f"{i}\t{p.returncode}\t{ms:.1f}\t{cls}\n")
        f.flush()
