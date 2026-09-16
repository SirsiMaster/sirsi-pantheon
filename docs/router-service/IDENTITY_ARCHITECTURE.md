# Router Identity Architecture — current state, what's fixed, what isn't, and why

**Status: reference, not an ADR.** This is the synthesis of one night's incident (2026-09-15/16:
an M1 and an M5 fought over hostnames, orphaned ten thread records, dropped a router item into a
private file, and stalled four PRs behind a CI environment bug) into one document, so the next
person — human or agent — inherits the lessons instead of re-deriving them at 3am. Where a fix
landed, it's named with its PR. Where a fix was designed and *rejected*, that's recorded too —
the rejection is as load-bearing as the code.

## 1. The identity model as it exists today

Four layers, each with its own notion of "who is this":

1. **Host token** (`HostToken`, `internal/routerstore/hosttokens.go`) — a bearer secret minted for
   one `Host` string (`sirsi router token mint <host>`, via the `sirsi-router-token` Cloud Run job).
   One token per physical machine; the relay is the only process on a host that holds it.
2. **Session** (`Session`, `sessions.go`) — minted from a token (`MintSession(host, agent, runtime)`),
   cached at `~/.sirsi/sessions/<agent>.json`. `sess.Host` is stamped from the CLIENT's claimed
   identity at mint time and never re-checked against reality afterward.
3. **Thread** (`internal/router/threads.go`) — a registered watcher (agent + surface + PID), scoped
   to a machine via `MachineID()` for LOCAL reaping decisions (`SameMachine`, PR #223, 2026-07) —
   but scoped to `os.Hostname()` (via `sess.Host`) for the SERVICE's `threadAuthority` check
   (`serve.go`). **These are two different machine-identity primitives protecting two different
   layers of the same word "host."**
4. **MachineID** (`internal/machineid`, extracted from `internal/router` in PR #765) — the hardware
   `IOPlatformUUID` (darwin) / `/etc/machine-id` (linux). Stable across renames, reinstalls,
   networks. This is the ONLY primitive in this list that is actually a fixed point.

**The bug, stated once:** layers 2 and 3's server-side check (`serve.go`'s token-host and
`threadAuthority` comparisons) key on `os.Hostname()`, which macOS lets DHCP and mDNS rewrite at
any time, on any interface event, with no notification. Layer 4 already solved this for layer 3's
LOCAL half. It was never wired into the SERVICE-facing half. That gap is the root cause of every
symptom below.

## 2. What actually happened (2026-09-15/16), root-caused

| Symptom | Mechanism |
|---|---|
| M1 got a 403 minting a session | `os.Hostname()` drifted from `M1.local` (the token's bound host) to `M5.local` (IPMonitor, unprompted) |
| A router item "vanished" | M5's shell had `SIRSI_ROUTER_DB` set, which makes `internal/routerstore/resolve.go`'s split-brain refusal a no-op **by design** (`cutOverMarker()` treats any `SIRSI_ROUTER_DB` as "a deliberate test/sandbox process") — the item was written to a private local file, never reaching the service |
| 10 M5 wake-loop threads went "unregistered" | Their thread records were bound to host `Mac`; correcting the session's claimed host to `M5.local` (the "obviously correct" fix) made every one of them a host mismatch — `threadAuthority` has no notion of "this is the same machine, just renamed" |
| Migrating M5's identity broke it, reverting fixed it | The M5's *token* was minted for `Mac` (from when `HostName` was unset); pinning `HostName` to `M5.local` without re-minting the token first made every session mint 403 |
| Four unrelated PRs' CI kept failing on hundreds of unrelated tests | The self-hosted runner's service process, launched outside the interactive login session's process tree, gets no `TMPDIR` — Go's `t.TempDir()` falls back to `/tmp` (`root:wheel`) instead of the per-user sandbox tmp, breaking every `routerstore` test that asserts a trust-group directory mode |
| The same CI fix appeared to keep failing after landing | The person diagnosing it (this session) restarted the runner without re-applying its own `env -i` isolation, reintroducing `SIRSI_*` env leakage from an unrelated interactive shell — a second, independent contamination class layered on top of the first, both presenting as "hundreds of unrelated test failures" |

## 3. What's fixed, and where

Evidence states below are as of a live re-check run at the time of writing
(`gh pr view <n> --json number,state,mergedAt,mergeCommit`), not from memory —
re-run that command before relying on this section, since PR state moves.

- **PR #766 — MERGED, verified** (`e7448c2fecb327603bdc0189d552089aec4e9237`). CI runner gets
  `TMPDIR` (`${{ runner.temp }}` in the workflow; also hardcoded as a defense-in-depth in
  `runsvc.sh` itself, since the runner's Node service wrapper does not pass an externally-set
  `TMPDIR` through to its child at all — confirmed by direct process-environment inspection, not
  assumed). This is the dependency every other PR below needed to get a clean CI run at all.
- **PR #761 — OPEN, built and locally verified** (`go vet` clean, full local test suite green as
  of the last local run; not yet merged). A wake-loop consumer with no durable action for 30 min
  is terminated once and replaced (the "stuck `claude --print`" class); spool clients fail fast on
  a trust-group mismatch instead of a silent 30s wait; `node-status` reads the indexed open-items
  view instead of the whole corpus.
- **PR #763 — OPEN, built and locally verified**, not yet merged. The `claude-io`/`codex-io`
  registry cwd correction (a stale, uncommitted SSA fix finally committed).
- **PR #764 — OPEN, built and locally verified** (new test `TestRouterDBOnCutoverHostWarning`
  passing locally), not yet merged. `sirsi router doctor` now warns, once, whenever
  `SIRSI_ROUTER_DB` is set on a cut-over host — the fix for the "vanished item" row above:
  read-only, on by default, catches the exact misconfiguration before it drops another item.
- **PR #765 — OPEN, built and locally verified** (new tests for the opt-in path and the shape
  helpers passing locally; `TestThreadAuthorityIsHostScoped` re-run and still passing, confirming
  the rejected bridge in §4 was correctly left out), not yet merged.
  `internal/machineid` extracted as a shared leaf package; `internal/routerstore` can claim
  `MachineID()` instead of `os.Hostname()` via an explicit opt-in
  (`SIRSI_ROUTER_USE_MACHINE_ID`), **off by default**.

**Why four of five are still OPEN**: their CI depends on #766's fix, and the self-hosted runner
that executes CI needed two rounds of environment-contamination repair (§6) before it could run
clean. "Built and locally verified" is a real, checkable state — it is not "merged" and this
document does not claim it is. Re-run the `gh pr view` check above for current status before
treating any of #761/#763/#764/#765 as shipped.

## 4. What was designed, then rejected — and must not be quietly re-attempted

**Automatic hostname↔machine-id bridging, at either the token-host check or `threadAuthority`.**
The design: when comparing two identity strings, if one looks like a hostname and the other looks
like a machine-id UUID, treat the mismatch as "the same host mid-migration" and allow it.

**Why it's unsafe, proven by this repo's own pre-existing test:**
`TestThreadAuthorityIsHostScoped` constructs exactly the attack this bridge cannot distinguish from
a legitimate migration — an authenticated session on host A touching a thread record that names
host B. The bridge's only signal is "the two strings have different shapes," which is *also* true
of two unrelated hosts where one has migrated and the other hasn't. There is no string-content
signal that discriminates "an attacker" from "me, renamed." Running the drafted bridge against the
existing suite: the attack test fails (a cross-host rewrite is allowed through). Running without
the bridge: it passes. That result is the whole argument — it doesn't need a second one.

The token-host version of the same bridge has an independent problem even if the thread-authority
one didn't: it would let ANY hostname-bound token mint a session for ANY machine-id the caller
chooses to claim, not just its own host's — a real widening of what that token authorizes, not a
neutral compatibility shim.

**The actual requirement for a safe migration:** proof of the OLD identity authorizing the NEW one,
recorded once, by an explicit action — not inferred from what the two strings happen to look like.
Concretely: a session already holding a valid token/thread binding for `Mac` performs one
authenticated call that says "I am now also known as `<machine-id>`"; the server records that
mapping; from then on either identity resolves to the same thread/token records. This is unbuilt.
It is `rs-42`/`rs-43` on Ra's ledger, and it is the ONLY correct shape for the fix — not a smaller
version of the rejected bridge.

## 5. The scoped-token gap (rs-44)

Surfaced 2026-09-16 by a real, legitimate ask: claude-home wants a cloud Routine (an
Anthropic-hosted sandbox, outside the physical fleet entirely) to read router status with no
mutation capability. **No token scope exists today.** `HostToken` has no scope field; a session
minted from any host token may call any RPC `ruleOfRa` allows, mutating or not. "Read-only" is
enforceable only by the CALLER's own discipline (never invoking a mutating method), never by the
credential itself. This was always true — it just never mattered until a credential was asked to
leave the physical fleet, where the caller's discipline can no longer be assumed.

Also true, orthogonal, and worth stating precisely: **there is no simple REST status endpoint on
the Cloud Run service.** `GET /api/node-status` exists (ADR-026) but is wired to the LOCAL Horus
dashboard binary, not the service. The service's actual protocol is signed RPC
(`POST /v1/call/{Method}`, bearer token, minted session, per-call `HMAC-SHA256` signature over
`method\nnonce\nbody`) — reachable from a `curl`+`openssl` sandbox, but a real implementation, not
a one-line fetch.

## 6. The operational lesson, stated once so it stops repeating

**A background/service process's environment is a property of the exact command that launched it
— not of the session, not of "the fix I already applied," not of a script that "did this
correctly last time."** Tonight this bit twice, independently, in the same investigation:
`SIRSI_ROUTER_DB` bypassing the split-brain refusal (§2, row 2) and `SIRSI_*` vars leaking into the
CI runner on a restart that dropped its own isolation flag (§2, last row; §A35 new entry, same
document). The fix in both cases is the same discipline: read the LAUNCHED PROCESS's own
environment back (`ps eww -p <pid>`) after every restart, not the command you typed. A command that
looks like the one that worked is a new claim, not a repetition of the old one.

## 7. What "the perfect router architecture" actually requires — in order

This is the sequence, not a wishlist. Each step is a precondition for the next; skipping ahead
reproduces this exact night on a different host.

1. **Ship what's already built** — PR #761/#763/#764/#765/#766, in that dependency order (#766
   first; the other four each merge `main` after it to inherit the CI fix).
2. **Build the authenticated migration verb** (rs-42/rs-43) — the ONE safe way to move a host's
   identity from hostname to machine-id, gated on proof of the old identity, not string shape.
3. **Migrate one host** (the M1 — lowest risk, easiest to verify from this session) and let it run
   through real DHCP/network churn before touching the second.
4. **Build a real scoped/read-only token type** (rs-44) before any credential is handed to a
   consumer outside the physical fleet — the claude-home ask is the first instance of a pattern
   that will recur.
5. **Only then** flip `SIRSI_ROUTER_USE_MACHINE_ID` fleet-wide and retire the hostname-keyed path
   as legacy-only.

Refs: PR #223, #761, #763, #764, #765, #766; ledger rs-42, rs-43, rs-44; `PANTHEON_RULES.md` A35
(new 2026-09-16 entry); router items `20260915-204553`, `20260916-045903`/`20260916-050902`.
