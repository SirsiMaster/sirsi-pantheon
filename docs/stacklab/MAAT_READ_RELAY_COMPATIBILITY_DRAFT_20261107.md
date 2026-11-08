# Draft: Ma'at unsent-read candidate compatibility

Status: source candidate with bounded fixture qualification on November 8,
2026. Nine Python mock controls, nine named Go relay fixtures, and the same Go
selection under race detection passed on M5 under separate fresh CPU1 grants.
An isolated unchanged production baseline reproduced HTTP202/queued behavior
for the unsent-read regression; the candidate passed that exact assertion.
The initial complete-fixture baseline could not compile because it referenced a
new candidate helper; that attempt is inconclusive, not a red control.
These are source/mock results, not deployment, installation or live fault
verification. This draft does not close queued-mutation custody or the separate
legacy-read and native-transition obligations.

## Wire and version matrix

The candidate preserves the spool request and response schema and introduces
no persistent frontier format. A newly submitted `MaatRead` after every
attempted connection was classified never reached receives a 503, rather than
a queued response. Legacy queued reads retain the old classifier: ambiguous
nested or joined errors can be mistaken for pre-send, causing an immediate
redial and later retries even if the request might have been sent. This is a
known replay risk and an outstanding release hold, not a safety endorsement.
Reclassifying those queued errors as unknown without a persistent frontier
would instead let subsequent wakes pass parked custody; this candidate does not
introduce that change or establish any durable per-lane frontier. Mutation retry
semantics remain unchanged.

| Relay | Supervisor | Bounded source expectation |
| --- | --- | --- |
| old | old | Existing behavior; an unavailable read can be queued and the supervisor can stop. |
| new | old | An unsent read returns 503; the old supervisor need not recognize the rendered sentinel and can stop. |
| old | new | The old relay can still queue the read; the new supervisor deliberately does not treat queued mutations or a 202 as the new sentinel. |
| new | new | The exact rendered error for an owned, deadline-bounded reservation read is eligible for the existing bounded running-supervisor retry. |

This is a static compatibility model, not evidence that binaries, installations,
loaded processes, or any host match these versions. A 503 is not an admission,
a lease renewal, a revocation check, or proof a previously queued request is gone.
Read failures before launch or before the signed read is sent remain fail-closed.
During an outage revocation is not observable until a successful check, and the
previously proven deadline remains the hard bound.

## Existing lifecycle boundary and unresolved exclusions

The physical spool-directory shared/exclusive lock is cooperative: updated
clients hold shared admission through settlement and a maintenance controller
holds exclusive admission, but legacy producers do not honor it. On the exact
Darwin root-relay path, existing startup maintenance uses root-bound intent and
a PID/start/generation-bound endpoint before recovery/discovery when its marker
is present; ordinary startup has an active endpoint but no durable marker. The
service refresh path combines its own service lock, exclusive cooperative
producer admission, exact selected-instance drain acknowledgment, repeated
instance/child/queue checks and drain revalidation. Those are useful controls,
not proof that an independently started old relay or non-cooperating writer is
excluded across crash, restart, upgrade or downgrade.

Before any separately authorized transition, the existing custodian must
establish actual root identity, selected executable/loaded version, sole relay
ownership, all relevant producers and admitted duties, and exact retained
queue/custody inventory; retain a reviewed crash-recoverable fence throughout
the handoff and verify the successor and rollback boundaries. If any legacy
writer or concurrent relay cannot be excluded by an enforceable pre-existing
mechanism, no transition is qualified. This candidate supplies no such new
exclusion mechanism and grants no authority to restart or stop any process.

The separate mutation defect remains: a queued `MaatMutate` receiving a 403
can be treated as delivered and removed without preserving the rejection. The
new characterization fixture records that existing behavior; it is not a safety
endorsement. A source-only read fix is independently reviewable, but deploying
it as a complete Ma'at relay repair while that custody risk is unresolved would
misstate scope. Credential-exposure assessment remains with its existing
custodian and is not authority to change or reveal any credential here.

## Qualification still required

Further runs require a compatible measurement-owner window and fresh own
admission. The isolated Go relay/private-spool and Python deadline/revocation/
denial controls above have bounded M5 results; broader non-regression,
supported-platform builds and exact binary/client mixed-version tests remain.
Independently
review exact bytes, verify the deployment and rollback contract, and retain
evidence before any release claim. These controls do not replace a real lifecycle
and legacy-exclusion qualification or solve outstanding mutation custody.
