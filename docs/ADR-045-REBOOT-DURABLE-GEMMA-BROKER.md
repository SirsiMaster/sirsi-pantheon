# ADR-045 — Reboot-Durable Gemma Broker (launchd owns the bounded server)

**Status:** Accepted — 2026-07-23
**Owner directive:** "local-llm-sovereignty" (2026-07-23): *"the local LLM should survive and reboot everyone else."*
**Refs:** ADR-031/-A/-B/-C (bounded broker), ADR-040 (load-bearing), PANTHEON_RULES A32; router items 20260723-130309.

> **2026-08-05 amendment — durable quarantine:** Reboot durability must yield
> to an explicit safety quarantine. The native SNE broker now serves on 8477
> under ADR-046. `sirsi gemma serve --quarantine` renames only its canonical
> plist to `.plist.quarantined`, disables and boots out only
> `ai.sirsi.gemma-broker`, and verifies launchd no longer holds the target.
> Both Pantheon self-healers already skip that filename: the dead-label duty
> manages only `.plist` files, while gemma-liveness cannot start an uninstalled
> broker. `sirsi setup` also preserves the quarantine. Restoration is explicit
> with `--restore`; bootstrap or readiness failure re-quarantines the service.
> No watcher, router, Horus, or unrelated label is disabled.

## Context

The warm local-model broker (bounded MLX server on 127.0.0.1:8765) was launched
via `nohup` — it died at reboot AND once more mid-day, and a **cloud** agent had
to revive it each time. That inverts sovereignty: the Tier-0 substrate every
local loop depends on was being life-supported by the very cloud sessions it is
supposed to outlive. The legacy `ai.sirsi.gemma` LaunchAgent could not help: it
was a one-shot `RunAtLoad, KeepAlive=false` launcher (ADR-031-C provenance)
whose forked, detached child was invisible to launchd — nothing revived a dead
broker between boots. The gemma-liveness supervisor duty (2-min cadence) is a
backstop, but it rides another LaunchAgent; the broker needed its own spine.

## Decision

**launchd owns the broker process directly.**

1. `sirsi gemma serve --foreground` runs the full ADR-031 bounded path (RAM
   refuse-gate, node-derived concurrency, memory-cap wrapper,
   `--prompt-cache-size/-bytes`), writes the pid/port files, registers with
   Hapi, then **`exec`s the capped server in place** — the pid launchd
   supervises IS the serving process, and the pid file stays truthful.
2. `setup.InstallGemmaBroker()` (run by `sirsi setup`) writes the
   **`ai.sirsi.gemma-broker`** LaunchAgent — `RunAtLoad + KeepAlive=true`,
   `ThrottleInterval 30`, `ProcessType Interactive`, `HF_HUB_OFFLINE=1` — and
   **retires the mis-wired `ai.sirsi.gemma`** plist. Skips cleanly when the MLX
   runtime is absent.
3. With the agent installed, `sirsi gemma serve` (background form) starts the
   broker **through launchd** (`launchctl kickstart`) so there is exactly one
   supervisor; `--stop` still works and says plainly that launchd will bring
   the broker back.
4. **Intentional quarantine outranks self-healing.** `--quarantine` preserves
   the launch definition as a reversible `.plist.quarantined` artifact while
   removing it from every automatic bootstrap path. `--status` reports this
   state distinctly from down/uninstalled. Normal `serve` refuses to bypass
   it. `--restore` is the only re-arm path and must enable, restore, bootstrap,
   and pass readiness; otherwise it returns the plist to quarantine.

## Evidence

Deployed live on the owner's machine 2026-07-23 (wrapper-script form of the same
shape) and kill-tested: broker pid 36715 SIGKILLed → launchd revived it as pid
36848 within seconds. This ADR canonizes that shape into the installer so every
machine gets it from `sirsi setup`, not from hand surgery.

The 2026-08-05 quarantine amendment is regression-bound by focused command and
installer tests, the full Go suite, `go build ./cmd/sirsi`, and `go vet ./...`.
Live activation remains a separate service-owner operation.

## Follow-ons

P1 (watchdog hardening) and P2 (offline takeover) remain open under router
items 20260723-130309.
