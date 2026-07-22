# ADR-042: Self-Hosted CI on Sirsi Silicon

**Status:** Accepted (owner-directed, 2026-07-22)
**Deciders:** Cylton Collymore (owner), claude-nexus (executor)
**Related:** ADR-040 (Do No Harm to the Running Host)

## Context

On 2026-07-22 the entire SirsiMaster fleet's GitHub Actions went down mid-investor-week:
every job failed at ~2s with *"The job was not started because recent account payments
have failed or your spending limit needs to be increased."* Diagnosis: the free plan's
2,000 included minutes/month were exhausted almost immediately each month because
GitHub-hosted **macOS runners bill at a 10x multiplier**. sirsi-pantheon's Mac-first CI
alone consumed 1,237 macOS minutes in July (= 12,370 multiplier-minutes, ~6x the entire
monthly allowance, ~$77 of a ~$108 would-be bill).

The owner's ruling, verbatim: *"isn't that the whole point of nexus and pantheon...
the transition from cloud to local hosted frames? why was this ever a question?"*

Sirsi's thesis is the self-healing compute substrate on owned silicon. Renting Apple
hardware from a cloud vendor at 10x to test software whose purpose is sovereign local
compute contradicted the thesis, cost real money, and put a third party's billing state
in the critical path of every merge.

## Decision

1. **CI macOS jobs run on Sirsi-owned Apple Silicon.** A self-hosted runner
   (`m5-sirsi`, labels `self-hosted, macOS, ARM64, m5`) is registered to
   SirsiMaster/sirsi-pantheon and managed by launchd
   (`~/Library/LaunchAgents/actions.runner.SirsiMaster-sirsi-pantheon.m5-sirsi.plist`,
   install root `~/.sirsi/actions-runner/sirsi-pantheon/`), so it is reboot-durable
   with no human in the loop.
2. All pantheon workflows that previously targeted `macos-latest` / `macos-15`
   (`ci.yml`, `release.yml`, `ios.yml`) now target
   `runs-on: [self-hosted, macOS, ARM64]`.
3. **Doctrine: local frames are the default for Sirsi compute.** Renting a
   cloud-hosted runner is the exception and requires a written justification in the
   workflow file. This applies fleet-wide (Nexus Linux jobs are candidates for a
   follow-up runner or containerized lane).

## Security constraints (binding)

- **sirsi-pantheon is a public repo**, so fork PRs are the threat: their code must
  never execute on Sirsi hardware. Compensating controls, both mandatory:
  1. Every self-hosted job reachable from a `pull_request` trigger carries the fork
     guard `if: github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository`
     (fork PRs skip the job entirely).
  2. Repo Actions policy requires manual approval for workflow runs from **all**
     outside collaborators (`fork-pr-contributor-approval: all_external_contributors`,
     set 2026-07-22), not just first-time contributors.
  Any OTHER Sirsi repo attaching a self-hosted runner must be private, or adopt both
  controls above before registration.
- The runner executes as the standard user, not root, honoring ADR-040
  (do no harm to the running host). No sudo in workflow steps.
- Registration/removal tokens are short-lived and never committed.

## Consequences

- **+** ~$77/month of rented macOS minutes goes to $0; the 10x multiplier is gone.
- **+** GitHub billing outages can no longer block macOS CI (self-hosted jobs do not
  consume hosted minutes) — the exact failure mode that took the fleet down.
- **+** CI runs on the same silicon class the product targets — test fidelity.
- **−** The workspace persists between jobs (not a pristine VM); workflows must not
  assume a clean host. One job runs at a time; register additional runner instances
  if queueing becomes a bottleneck.
- **−** Host availability is now a CI dependency; the launchd service plus
  `sirsi diagnose` cover liveness.
