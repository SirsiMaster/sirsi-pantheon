# Desktop custody source qualification

Implementation is isolated in the existing draft PR1169. This packet is for
independent SSA source review, not a claim of installed authority or native power.

## Implemented path

- Management-only immutable finite policy and revocation on the existing
  canonical service; generic state writes cannot create authority.
- Verified token/session/runtime/thread/native-machine scope, exact owner request,
  boot, nonce, generation, candidate, inventory, wake and bounded lifetime.
- Shared transaction between consumption and real workload reservation admission;
  exact real controller budget is checked, never a borrowed PID/holder string.
- Normal supervisor private execution fact, native actual-parent/start/argv check,
  service-anchored job budget and actual confirmed lease. This fact is not a grant.
- Native identity, process/writer/assertion and full-wake adapters; fixed owned
  wake operations; bounded independent M1 observation; same-controller return and
  conclusive return-only reconciliation. No invocation replay.
- Operator bootstrap/status/preflight/coordinator/observer/dispatch/return paths.

## Observed source checks

Full affected-package race suites, CLI tests and build previously passed under
normal terminal a1aadb002ca14b43b19ca698546b14a7. Longer real-budget source tests
passed under d32ee4ba9366465197a6560af23a21be. All72 normal supervisor regressions,
including the actual execution-fact handoff, passed under
f586fe7553b04ebfbca70274fe192004. Final combined source lint/race/build gate is
pending when this packet is first written; append its actual terminal/result.

The reconciliation regression first failed because the retained unknown event
sorted after verified return. Original error rows are now retained with verified
return ordered last. Receipt c180c2aef03c4e8e888c4844b910c2fe remains a failure.

Bundled lsof(8), OUTPUT FOR OTHER PROGRAMS, identifies `a` as access mode, `f` as
file descriptor and `-F0` as NUL fields with newline set separators. A real-format
negative control failed against the old parser at
461613e0896a43b38e8e56c75706383b; the corrected parser handles access mode and
embedded filename newlines and refuses incomplete visibility/unknown evidence.
Do not relabel either failing receipt as a pass.

## Boundaries and unperformed work

Native v1 is at most180seconds to scheduled wake /210seconds lifecycle, only when
the actual confirmed controller lease and conservative job budget cover it plus
margin. A current normal supervisor uses240second leases, but the controller
checks the real row. Preconsume90second participant and15second readiness checks
are unchanged; post-wake requires genuine live refresh and unchanged generation,
not fake heartbeats or a global stale-session exception.

No live delegation, ARM, Sleep, wake schedule, guard/security/privilege change,
new key/IAM enrollment or new cloud worker was performed. Actual installed
privileged visibility/transport/participant bindings, native log-format acceptance,
independent review, normal release and physical/off-window cloud overlap remain
separate gates. Historical cold cloud acquisition errors do not block a truthful
warm-only test. No active foreign publisher/model may be interrupted for it.

An expired/lost actual controller budget or custody identity remains an incident
review, not a fabricated continuity claim. No fence-unaware rollback while a
consumed host is unresolved. Reading Room/Workspace mirrors are pending review.

Retained local artifact hashes are in manifest.json; those paths are source-host
evidence, not public hyperlinks or claims of remote availability.

## Final local gate result

Normal terminal2fa7a192d8144966af7461764a8f6b77 exited0: affected-source lint
0issues; complete desktopcustody/routerstore race suites1.247s/146.699s; CLI
failure/source tests4.852s; full CLI build;72 supervisor regressions passed.
These results supersede the earlier pending local-gate line, not the explicit
unperformed repository-wide CI, independent review or physical proof. Earlier
formatting failures remain recorded, not relabeled as passes.

## Independent SSA review correction — active peers

SSA's review of `ffd08b35` found that recently reaped or unknown-status peers
could pass the participant freshness check. The corrected check requires exact
`active` status for every delegated participant, including all three again at
consumption. `TestDesktopCustodyRejectsNonActivePeerAfterReady` changes the
coordinator or observer after readiness to each of reaped, closed, suspended,
idle, unknown, and empty; asserts that the actual stored status changed; and
proves consumption refuses without consuming authority or fencing ordinary work.

Normal admitted run `fa23dc1d95f34f3f965bc11078a47b96` passed all custody
routerstore race tests (12.430 seconds) and routerstore vet, then released
naturally. This source correction does not perform native power or create a
production delegation. The initial run `43c6173c82484e99a171446983fe3fba`
failed because its test fixture's fractional timestamp sorted before the
whole-second original in the existing upsert ordering; the corrected fixture
uses a later whole second and checks the mutation. Both receipts are retained.

The next full-share native test window was explicitly ceded to SSA for its
finite Gemma qualification; SHA starts no competing payload while it runs.

## SSA whole-controller correction — pre-native consumed fence

Independent review of `124dad06` found that successful canonical consumption
followed by an invocation-journal write failure could strand the scheduling
fence even though no native sleep ran. A separate server-owned native gate now
serializes invocation and pre-native abort with ordinary reservation admission.
Only the exact consumed custodian can terminalize a versioned record lacking
native authorization; expiry/revocation do not revive it. Legacy missing-phase
records deny compensation. Lost invocation responses are unknown, with no
native call, automatic abort or invocation retry. Once native authorization is
issued, native/journal errors retain the conservative fence and owned wake.
Actual return/observer evidence is bounded by native authorization, with both
prior consumption and authorization required.

The joined regression uses the actual canonical HTTP/session/store and private
file journal, injects the write failure after consumption, and proves zero
native calls, owned-wake cleanup and subsequent ordinary reservation. Additional
negative controls cover invoke/abort concurrency, exact role, expired/revoked
recovery, immutable abort replay, spent nonce/restart, legacy state, changed
identity/peer/budget, lost response before/after commit and a journal failure
after native authorization. Power is fake throughout these source tests.

First run6298a68c13f94737a083e2d566020691 failed because its test journal directory
was absent; the fixture now creates it privately. Joined tests, vet and CLI tests
passed under e04d6bc6bdb34fe6b3696848dfac255b (1.282/16.322/4.307 seconds).
Expanded gate e21acdcccc4c45a1acdc1aa1e3b7925b had zero lint issues and passed the
full routerstore race suite147.636 seconds, but failed the existing native
no-consumption negative after the new native interval bound accidentally omitted
the prior consumption guard. Both guards and explicit new native-phase negative
cases are restored. This failed receipt remains a failure. Final combined-source
qualification is pending the next gate when this section is written.

Final combined-source correction gate `0417e04b2a1848bc88a020522aad83f8`
passed: affected-source lint0 issues; full desktopcustody/routerstore race suites
1.204/151.103 seconds; vet; CLI tests4.234 seconds; full non-installed CLI build;
and diff check. Natural exit0/release. Native linker duplicate-lobjc warnings
were non-fatal. Whole successor CI, independent SSA delta acceptance, normal
release and installed native/physical qualification remain pending.
