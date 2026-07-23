# ADR-044 — `sirsi runner`: Self-Hosted CI as a Product Verb

**Status:** Accepted · 2026-07-22
**Extends:** ADR-042 (Self-Hosted CI on Sirsi Silicon)
**Owner directive:** self-hosted M5 runners "should be the default install for Nexus and Pantheon" — and, by the local-frames ruling, for every repo the toolkit touches.

## Context

ADR-042 established the ruling: local Sirsi silicon is the default for all CI
compute; cloud rental is the justified exception. The 2026-07-22 incident (the
account-wide Actions allowance drained ten days early during investor week,
every job refusing in two seconds) proved the ruling operationally — the one
repo already on a self-hosted runner sailed through the freeze untouched.

That night's remediation was executed by hand from a proven shell seed
(`~/.sirsi/actions-runner/install-runner.sh`): six repositories, six launchd
services, one Mac, zero marginal dollars. A hand-run script is an incident
response; a default is a product verb. This ADR productizes the seed.

## Decision

A `sirsi runner` command group in the Pantheon CLI:

- **`sirsi runner install <owner>/<repo>`** (bare `<repo>` defaults to
  SirsiMaster) — registers this Mac as the repo's build worker: clones the
  donor runner software, fetches a registration token via `gh` (gh owns
  GitHub auth; we never reimplement it), configures with the fleet identity
  `m5-sirsi` and labels `self-hosted,macOS,ARM64,m5`, installs the
  reboot-durable launchd service, then polls the runners API until the runner
  reports **online** — the verb succeeds only on proof, not on "started".
  Idempotent: an already-configured repo no-ops.
- **`sirsi runner status`** — every locally-installed runner graded live
  against GitHub (online/offline/unregistered/unreachable, busy). `--json`
  emits `[{"repo","name","status","busy"}]` — a published contract the board
  producer consumes so the menubar can show fleet runner health.
- **`sirsi setup` integration** — when setup runs inside a git repo with a
  GitHub origin and gh is authenticated, it offers (default-yes) to put that
  repo's builds on this Mac. The default install experience now lands on
  owned silicon; the rented cloud becomes the deliberate exception.

### Preserved gotchas (the seed's hard-won knowledge, now in Go)

1. The donor copy strips **all** instance state — `.runner_migrated`
   especially, which makes `config.sh` believe the copy is already configured
   and silently skip registration.
2. `svc.sh` requires `./runsvc.sh` beside it — staged from `bin/runsvc.sh`
   before `svc.sh install`, or the service install fails.

## Security

Fork-PR safety is verified strict (`all_external_contributors` approval
required) on all public Sirsi repos: a stranger's fork cannot ask this
hardware to run its code without explicit approval. Product code repos have
moved private. The runner runs as the login user with no elevated
privileges; registration tokens are single-use, short-lived, and fetched
per-install via the operator's own `gh` auth — never stored.

## Consequences

- One command turns any Mac into a repo's build worker with proof of
  liveness; the fleet's runner health is one `--json` away from any surface.
- The runner software donor dir (`~/.sirsi/actions-runner/sirsi-pantheon`)
  must exist from one initial by-hand install per machine. Acceptable: it is
  a one-time bootstrap, and `install` names the gap explicitly when missing.
- One runner instance takes one job at a time; parallel storms queue.
  A second instance is minutes to add, deliberately manual (ponytail: the
  ceiling is named, the upgrade path is known, the demand is not yet real).
