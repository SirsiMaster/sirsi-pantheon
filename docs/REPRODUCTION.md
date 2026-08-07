# Test our findings yourself

Everything we report is a measurement someone else should be able to make.
This page is the complete environment and parameter record for every number
we publish. If your results differ, we want to know — file an issue with
your environment block and we will look together.

Canonical source: sirsi-inference @ 823d1dc.

> **What you can actually run today, and what you cannot.**
>
> The `scripts/…` **and** `docs/evidence/…` paths named on this page are
> **internal to the private `sirsi-inference` engine repo**. They are recorded
> here so the exact procedure and provenance behind each number is nameable —
> **none of them exist in this repository.** Do not read the `scripts/…` paths
> as commands you can execute, and do not read the `docs/evidence/…` paths as
> files you can open here.
>
> What a third party CAN run today is the **client-side harness**, which
> ships with Anubis, measures any OpenAI-compatible server, and contains no
> engine code. Point it at the sealed signed binary or hosted endpoint we
> provide (see *What ships publicly vs what stays sealed*, below).
>
> A scrubbed public harness covering the remaining measurements is not
> published yet. Until it is, the numbers on this page are **independently
> checkable against our endpoint, but not independently re-derivable from
> our source** — stated plainly rather than implied, per Rule A33.

## Environment (as measured)

| Item | Value |
|---|---|
| Hardware | Apple M5 Max, 48 GB unified memory |
| OS | macOS 26.5.2 |
| Model | `mlx-community/gemma-4-12B-it-8bit`, snapshot `200bb6db075e137a4deb08838865ac4ddb86292e` |
| MLX | 0.32.0, vendored build (`scripts/build-mlx.sh`; stamp in `mlxbuild_stamp.go`) |
| Engine source | pinned per-file SHA-256 in `docs/evidence/contribution-adjusted/perf-manifest-pinned.txt` (private repo — not in this repository) |
| Binary | `docs/evidence/contribution-adjusted/pinned-bin-sha.txt` (private repo — not in this repository; see *the one artifact this page still owes you*, below) |
| Comparator inventory | `docs/evidence/contribution-adjusted/mlxlm-localmods.txt` (exact local state of the comparator install; private repo — not in this repository) |
| Machine state | quiet: no other GPU workloads; warm-up before every measurement |

## The measurements and their procedures

Every `Script` cell below names a file in the **private** `sirsi-inference`
repo. It documents how the number was produced; it is **not** a command a
reader can run. See the note at the top of this page.

| Reported finding | Regime | Script (private repo — not publicly runnable) |
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

## The one artifact this page still owes you

"Verification runs against a SEALED signed binary we provide" is only a
checkable claim if you can confirm the binary you were given is the binary we
measured. That confirmation is a single line — the SHA-256 in
`pinned-bin-sha.txt` — and it is currently **inside the private repo**, so the
seal cannot be checked against anything. Publishing that one hash reveals no
kernel, no architecture, and no patch; withholding it makes the sealed-binary
route unfalsifiable in exactly the way Rule A33 exists to prevent.

Same reasoning, lower stakes, for `mlxlm-localmods.txt`: it describes the state
of the PUBLIC comparator (`mlx_lm`), not our engine, and a reader cannot
reproduce a comparison without knowing what the comparator was.

`perf-manifest-pinned.txt` is a closer call — the hashes reveal nothing but the
filenames sketch engine module structure. `patch-qmv.py` stays sealed
unconditionally, per the deferred upstream-filing decision above.

This is recorded as an open gap rather than quietly left as a broken path. The
publication decision is the owner's, not this page's.

Refs: ADR-051, docs/CLAIMS-TABLE-A33.md, ANUBIS_RULES.md § Rule A33
