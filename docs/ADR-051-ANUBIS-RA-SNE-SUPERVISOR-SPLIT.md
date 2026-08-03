# ADR-051: Anubis/Ra Product Split — SNE Supervisor Configuration

## Status
**Accepted** — August 3, 2026. Owner: claude-pantheon (sirsi-pantheon).
Counterpart decision: sirsi-inference `docs/adr/ADR-001-sne-anubis-ra.md`.

## Context

The Sirsi Nexus Engine (SNE, repo `sirsi-inference`) shipped neutral operating
profiles (`serve --profile interactive|fleet`, ADR-001-sne-anubis-ra). SNE is
not Anubis or Ra — it exposes a clean boundary with no product opinion. The
product opinion lives here, in Pantheon's supervisor configuration.

Two Pantheon products exist:

| Product | Tier | Persona |
|---------|------|---------|
| **Anubis** | Enthusiast | One Mac, one user, workstation hygiene |
| **Ra** | Enterprise | Fleet-scale, multi-node, datacenter/subnet sweep |

SNE must run under both. They have different resource contracts:

- Anubis must yield to user work — the Mac is the user's primary machine.
  An unconstrained SNE batch would compete with foreground apps.
- Ra owns the machines it manages — no user sits at a fleet node watching
  their RAM disappear during a sweep.

SNE's `interactive` profile (batch 4, 20 GB ceiling, yield posture) maps to
Anubis; `fleet` profile (prior unconstrained behavior) maps to Ra.

## Decision

**Pantheon's supervisor is the sole authority for which SNE profile each
product instance receives.** SNE is never asked to detect its product context —
it always takes an explicit `--profile` flag from the supervisor.

### Supervisor Configuration Binding

```yaml
# configs/supervisor/anubis.yaml
sne:
  profile: interactive   # batch 4, 20 GB ceiling, yield posture

# configs/supervisor/ra.yaml
sne:
  profile: fleet         # prior behavior, no ceiling
```

The supervisor (`internal/supervisor/`) reads these configs when launching or
reconnecting to an SNE instance. The `--profile` flag is injected at the
`sirsi sne serve` invocation.

### Boundary Rules

1. **SNE is neutral.** Pantheon may not patch SNE to carry product-awareness.
   Profile selection is always Pantheon-owned config → supervisor → SNE flag.
2. **Ra requires explicit `sne.profile: fleet`.** No implicit escalation.
   Fleet profile is never the default for a new Anubis config.
3. **Profile changes require a supervisor restart.** SNE profiles are not
   hot-reloadable; a config change triggers a supervised restart sequence.
4. **Support requests to SNE route to claude-nexus.** Pantheon owns config;
   sirsi-inference owns profile behavior. The interface is `--profile`.

### Future Work (Not Decided Here)

The supervisor config files (`configs/supervisor/anubis.yaml`,
`configs/supervisor/ra.yaml`) are scaffolded by this ADR but the supervisor
implementation that reads and injects them is a separate workstream. This ADR
is the canonical reference for that implementation.

## Consequences

**Positive:**
- SNE stays neutral — no product coupling in the engine repo.
- The resource contract for Anubis (yield, 20 GB cap) is enforced in one
  place — the supervisor config — not scattered across product code.
- Ra's fleet behavior is opt-in and explicit, preventing accidental resource
  escalation on single-user machines.

**Negative:**
- Two config files to keep in sync with SNE profile documentation as SNE
  evolves. Mitigation: supervisor validates profile names against SNE at
  startup and rejects unknown values.

## Alternatives Considered

| Alternative | Rejected Because |
|-------------|-----------------|
| SNE auto-detects context (single vs fleet) | Violates SNE neutrality; couples engine to product topology |
| Single profile for both products | Can't satisfy both yield (Anubis) and throughput (Ra) contracts |
| Env var injection instead of flag | `--profile` flag is the SNE API surface; env vars are undocumented behavior |

## References

- `sirsi-inference docs/adr/ADR-001-sne-anubis-ra.md` — SNE profile definitions (counterpart)
- `SirsiNexusApp/THREADS.md` @ 8e4b8db — thread ownership map
- `ADR-015-DEITY-HIERARCHY.md` — Horus (local) vs Ra (fleet) scope
- `ADR-017-RA-HORUS-CTR-HYPERVISOR.md` — multi-agent orchestration boundary
- Router item `20260802-002006` — owner directive establishing this work
- PANTHEON_RULES §2 (A23) — Truth Vector (no SNE product-coupling)
