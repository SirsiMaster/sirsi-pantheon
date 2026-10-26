import sys, collections
R = sys.argv[1]

def load(p):
    return {int(l.split("\t")[0]): l.rstrip("\n").split("\t") for l in open(p)}

def tally(a, b, label, want):
    A, B = load(a), load(b)
    rounds = sorted(set(A) | set(B))
    dist, other, missing, bad = collections.Counter(), [], 0, []
    winner = collections.Counter()
    for r in rounds:
        if r not in A or r not in B:
            missing += 1
            continue
        w = (A[r][3] == "win") + (B[r][3] == "win")
        dist[w] += 1
        if A[r][3] == "win": winner["m1"] += 1
        if B[r][3] == "win": winner["m5"] += 1
        for side, row in (("m1", A[r]), ("m5", B[r])):
            if row[3].startswith("other"):
                other.append((r, side, row[3]))
        if w != want:
            bad.append(r)
    lat = sorted(float(x[2]) for x in list(A.values()) + list(B.values()))
    print(f"{label}: rounds={len(rounds)} missing={missing} winners-per-round={dict(sorted(dist.items()))} "
          f"wins m1={winner['m1']} m5={winner['m5']} non-claim-errors={len(other)} "
          f"latency p50={lat[len(lat)//2]:.0f}ms p99={lat[int(.99*len(lat))-1]:.0f}ms")
    for o in other[:5]:
        print("   other:", o)
    print(f"   rounds != {want} winner(s): {len(bad)} {bad[:10]}")
    return len(bad) == 0 and missing == 0 and not other

ok_b = tally(f"{R}/m1.tsv", f"{R}/m5.tsv", "G3 shared ledger (want exactly 1)", 1)
ok_c_detect = sum(1 for r in load(f"{R}/ctl-m1.tsv") if True)
A, B = load(f"{R}/ctl-m1.tsv"), load(f"{R}/ctl-m5.tsv")
double = sum(1 for r in A if r in B and A[r][3] == "win" and B[r][3] == "win")
print(f"negative control (separate ledgers): rounds={len(A)} rounds with 2 winners={double} (want > 0, ideally all)")
print("VERDICT:", "PASS" if ok_b and double > 0 else "FAIL")
