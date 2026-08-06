# Test our findings yourself

Everything we report is a measurement someone else should be able to make.
This page is the complete environment and parameter record for every number
we publish, with one-command scripts. If your results differ, we want to
know — file an issue with your environment block and we will look together.

Canonical source: sirsi-inference @ 823d1dc. Scripts and evidence live in
the sirsi-inference repo (private engine repo); the client-side harness
that measures any OpenAI-compatible server ships with Anubis.

## Environment (as measured)

| Item | Value |
|---|---|
| Hardware | Apple M5 Max, 48 GB unified memory |
| OS | macOS 26.5.2 |
| Model | `mlx-community/gemma-4-12B-it-8bit`, snapshot `200bb6db075e137a4deb08838865ac4ddb86292e` |
| MLX | 0.32.0, vendored build (`scripts/build-mlx.sh`; stamp in `mlxbuild_stamp.go`) |
| Engine source | pinned per-file SHA-256 in `docs/evidence/contribution-adjusted/perf-manifest-pinned.txt` |
| Binary | `docs/evidence/contribution-adjusted/pinned-bin-sha.txt` |
| Comparator inventory | `docs/evidence/contribution-adjusted/mlxlm-localmods.txt` (exact local state of the comparator install) |
| Machine state | quiet: no other GPU workloads; warm-up before every measurement |

## The measurements and their scripts

| Reported finding | Regime | Script |
|---|---|---|
| Single-stream vs stock toolchain (+4.02% mean, 32/33 rounds) | one request at a time, deployed-stock comparator | `scripts/prove-exceed.sh` |
| Contribution-adjusted comparison (parity with the repair shared) | same, comparators patched with our proposed fix (`docs/evidence/contribution-adjusted/patch-qmv.py`) | `scripts/prove-contribution.sh` |
| Concurrent throughput (~1.43× at equal caps) | max-concurrency 32 both sides, exact 4096-token workload | `scripts/prove-contribution-concurrent.sh` |
| Deterministic serving (16/16 identical under load) | solo vs 8-way concurrent + window-crossing neighbor, `SIRSI_BATCH_INVARIANT=1` | `scripts/prove-guarantee.sh` |
| Invariant-mode cost (recorded, no claims) | 8-way 64-token burst, three arms | `scripts/time-invariant-arms.sh` |

## Framing we hold ourselves to

The behaviors we reported upstream are **potential defects** and the changes
we proposed are **potential improvements** until the upstream maintainers
judge them — they know their framework better than we do. Our numbers
describe **our machine, our workload, our pinned versions**, nothing more
general. Different hardware, model, or workload can legitimately produce
different results; that is why this page exists.

## What ships publicly vs what stays sealed (owner decision 2026-08-03)

Public, so anyone can verify: this parameter/environment record, the
CLIENT-side benchmark harness (measures any OpenAI-compatible server;
contains no engine code), comparator setup, and the potential-defect
reproductions already public in the upstream issue threads. Verification
runs against a SEALED signed binary (or a hosted endpoint) we provide.

Never published as source: the engine kernels, the serving architecture
internals, and the proposed upstream patch (held pending the deferred
upstream-filing decision). The engine repo is private; a provisional
patent disclosure on the deterministic-serving method precedes any
public artifact that reveals its mechanics.

Refs: ADR-051, docs/CLAIMS-TABLE-DRAFT-A33.md, ANUBIS_RULES.md § Rule A33
