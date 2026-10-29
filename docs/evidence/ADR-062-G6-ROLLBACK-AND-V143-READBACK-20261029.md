# ADR-062 G6 rollback rehearsal and v0.24.143 install readback — 2026-10-29

Operator: Ra (`thr-3c75e21290ef931f`, M1). Owner decision card, 2026-10-29: "Ra ships it now".

## v0.24.143 publication

- Prep PR #1254 (head 576f26fc). CI green; merged as `e34f1fb2`. Tag `v0.24.143` points at `e34f1fb2`.
- Release run 40874191224: Verify Source, Build DMG+PKG, npm, and Homebrew cask all passed. Published 12:38:21Z.
- Assets: `SirsiPantheon-0.24.143-arm64.dmg` and `SirsiPantheon-0.24.143-arm64.pkg`. PKG `sha256:1ce99e932d0bef42b7780f24867f2eb29c47edd9fed26264aeb0f54abe7fbd37`. Signed `Developer ID Installer: Sirsi Technologies Inc. (9D382WV988)`; Gatekeeper reports `Notarized Developer ID`.
- No router service deploy: every `ReapDeadThreads` call site is client-side (thread CLI, doctor, menubar, supervisor duties).

## Install readback

| Host | Route | `sirsi version` |
|---|---|---|
| M1 | `brew upgrade --cask` 0.24.135 → 0.24.143 | v0.24.143 |
| Studio | `brew upgrade --cask` | v0.24.143 |
| M5 | The cask failed: `/Applications/Pantheon.app` is root-owned from the prior PKG install, so brew could not write to it and purged its 0.24.143 files. The bundle stayed intact (`codesign --verify --deep --strict` OK). Re-installed via `installer -pkg` from the release PKG after a hash and signature check. | v0.24.143, codesign OK |

Process adoption, judged by start time against install time (12:40–12:43Z):
- **Adopted 143:** all M1 wake loops except `claude-nexus`, and 22 of the 28 M5 wake loops. They restarted at their consumer boundary.
- **Still the earlier image (not restarted, per standing rules):** the relays (M1, M5, Studio), `horus supervise` (all three), M1 `claude-nexus`, and six M5 loops started 2026-10-28 00:33 local (`sirsi-hardware-admin`, `codex-apollo`, `codex-pantheon`, `codex-porch-and-alley`, `codex-finalwishes`, `codex-assiduous`).
- **Residual:** `horus supervise` runs `stale-thread-reconcile` every 15 min, which calls `ReapDeadThreads`. Until supervise restarts on 143, the old rule can still reap a resumed attended session there.

## Resumed-session readback (143 client, live registry)

The test registered `ra`/`codex` (a group with no live watcher on M1) with session `g6-readback-143b-1793278083`, anchored to an exited PID 49526.
- After two reap passes (`sirsi thread list`), the thread was still `active`.
- Re-registering the same session from live PID 49524 returned the **same** `thr-dc8abee2e612b093`, which was then closed.
- The first attempt, on `ra`/`claude`, was retired by `ReapStrayThreads` (ADR-024 one-watcher-per-surface) because this attended session is the live watcher for that group. That is a test-design collision, not the #1253 path. A single-session lane has no live sibling.

## G6 Cloud Run rollback rehearsal

Service `sirsi-router` (sirsi-nexus-live, us-central1):
- **Pre:** `sirsi-router-v142-full-58e4806e` at 100%. `/v1/healthz` returned 200 on the main URL and on the v141 tag URL.
- **12:35:29Z, rollback:** traffic moved to `sirsi-router-v141-full-c1651420=100` in **12 s**. Main healthz 200, plus three more probes at 200.
- **12:35:58Z, roll-forward:** traffic moved back to `v142=100` in **4 s**. Healthz 200.
- Total window on v141: **27 s**. A trap guaranteed the restore.

G6 is now rehearsed for traffic rollback. The SPKI client pin remains the owner's choice of key (ADR-062 audit 2026-10-05).
