# Ma'at native authority service

This component prepares the launch recipe for a Mac that already owns a finite
canonical Ma'at pod delegation. It does not create a delegation, create TLS
credentials, start a listener, enable a login item, or qualify an authority.

## Product and trust boundary

Classification: **platform-foundation** until the compatible native authority and
client pass their integrated qualification. The user is the operator of a pod
whose local admission must survive loss of internet without inventing a second
booking authority. The workflow is inspect → prepare → explicitly start the
qualified service → optionally opt into login start → verify → stop or opt out.
This supports reliable local operation; willingness to pay is not independently
validated. Ra owns runtime behavior, Pantheon owns package composition, and SSA
owns release and deployment custody.

ADR-120's supplied revision 2 remains **Proposed**. Its older spelling
`maat authority serve` and per-epoch pod-secret description do not describe
the current implementation verbatim: the exact source command is
`maat authority-serve`, using existing approved mutual TLS certificates and
fingerprint-to-agent bindings. This component packages that implemented command;
it does not ratify the authentication change or claim that all ADR runtime
requirements are implemented. Ra owns reconciliation of the ADR and runtime.

Only existing, explicitly approved TLS path references are accepted. The private
key is neither printed nor copied into the recipe. One canonical delegation owns
the named resources; router items, tasks and threads remain on the existing
router service. A recipe is configuration, not a grant or an admission.

## Preparation and user consent

`sirsi maat authority-service plan` inspects the inputs and renders a plan.
`sirsi maat authority-service install` explicitly writes the dormant recipe.
Use each command's `--help` for the exact required selectors. Neither command
bootstraps launchd, opens the journal, creates keys, or changes network settings.

The recipe invokes the existing package-installed `sirsi maat authority-serve`
directly, with an argv array, not a shell. A developer build or an ambient PATH
entry cannot silently become the service engine. The existing runtime currently
needs an explicit canonical registry working directory; preparation does not
bundle or fabricate a separate registry.

The installed engine must satisfy the existing Developer ID signature for Team
`9D382WV988` and identifier `sirsi`. Verification uses Apple's system tool,
without executing the engine payload or signing anything. The host must already
be enrolled on the canonical router service. An explicit local SQLite sandbox
selector is refused before store resolution, so planning cannot accidentally
create a local database.

The dormant recipe stays **outside `~/Library/LaunchAgents`**. Pantheon's legacy
refresh helper enables and bootstraps Sirsi plists in that directory, so merely
setting a disabled bit there would not preserve consent. Package installation
and upgrades do not opt the user into startup.
Case variants of `Library/LaunchAgents` are also refused, including on
case-insensitive APFS; an existing login-directory alias to the recipe directory
or one of its ancestors is refused as well.

Starting the qualified service is a separate user action using the plan's exact
launchd job and path. Login start is a separate explicit opt-in, not implied by
starting for the current session. The source implementation and fixtures in
this increment do not perform either action on the development host.

## Verification and recovery

Keep four facts separate: a configuration was inspected; a dormant recipe was
installed; a native process is running; and that process passed qualification.
Preparation does not observe global service or login state: those remain
unknown, even though preparation itself performs no startup or login action.
Do not display installed as running, or a current delegation as perpetual
authority. The serving process must revalidate at start and enforce finite
expiry, canonical epoch fencing, renewal and exactly-once reconciliation.
Those runtime guarantees belong to Ra, not this recipe renderer.

Refused input is actionable: select the actual package-installed engine, repair
the selected canonical path or private permissions, select the correctly
delegated machine, or obtain an approved current delegation/TLS route. Never
generate keys or mint a fallback grant to bypass a refusal. An occupied recipe
is preserved; inspect it rather than replacing someone else's configuration.
When persistence fails, retain the reported cleanup debt and resolve that exact
object before retrying. Never treat a partial file as installed.

An upgrade must preserve the recipe, TLS references, journal and login opt-out.
Rollback is to the previous signed package plus its compatible recipe, after
the authority's normal stop/handback procedure; never replace a running engine
or stop another job as part of preparation. Successful live startup, native
mutual TLS, offline booking, reconnection and reconciliation require separate
integrated evidence before this component may be called operational.

## Stack Lab and implementation order

Recipe: `contracts/stacklab/maat-pod-authority-service-recipe-v1.json`.
Implementation: `internal/setup/podauthority*` and
`cmd/sirsi/maatpodservice_darwin.go`. Runtime: Ra's existing authority command.

1. Validate and render without mutation.
2. Prepare a create-only dormant recipe under explicit user action.
3. Review and test positive/refusal, argv, path, permissions and preservation.
4. SSA integrates the compatible signed payload; Ra qualifies the actual native
   service and client before any activation claim.

| Decision | Chosen | Rejected |
| --- | --- | --- |
| Engine | Existing signed package Sirsi | IDE-hosted daemon or copied binary |
| Startup | Explicit user action; login separately opt-in | Install-time activation |
| Recipe placement | Dormant private directory | Auto-refreshed LaunchAgents |
| Booking authority | Existing finite canonical delegation | A second general ledger |

Data flow: canonical delegation + native identity + approved TLS selectors →
validated configuration → deterministic direct-argv recipe → explicit dormant
installation. Every refusal returns without activating a service. Runtime
events remain the authority command's responsibility.

Publication: repository source is authoritative. Reading Room and Workspace
mirrors are pending SSA's integrated runbook publication; neither is claimed
synchronized by this source increment.
