# Native remediation execution — corrective source contract

Scope: `menubar-remediation-20261005`, native operator review → confirmation →
one command → fresh diagnostic observation → retained bounded outcome. This is
platform-foundation correction, not a new remediation catalog, privilege,
installer, background service, or model workflow. Existing typed mappings and
Go operational authority remain unchanged. Installed UX is not yet qualified.

## Approval and invocation

When confirmation opens, freeze semantic diagnostic evidence (not its ephemeral
UUID), exact argv including `--json`, and working directory. Immediately before
launch, require the same current finding, freshness/error state, command and
repository. Cleared, changed, stale or failed observations invalidate consent.
The closed legacy repair registry follows the same rule; prose never becomes
executable authority. Each explicit next-action confirmation binds its source
receipt, exact command, argv and repository. No new command authority is added.
The producing directory stays with each report: changing selection from A to B
refuses offered actions before forming confirmation, not merely after consent.
Unknown-scope preloaded reports remain display-only; current selection never
supplies missing report provenance.

Set the invocation latch before any suspension. A ResultView identity captures
once, including non-JSON and error output. `.task`, project callbacks, Refresh,
Retry and post-action refresh must not replay the original invocation. Recheck
is diagnostic-only. Another mutation requires another explicit intent and its
own frozen confirmation. Retain the actual follow-up receipt, not an automatic
rerun of the original action.

## Capture and honest limits

Use nonblocking stdout, stderr and stdin; stdin is at most 64 KiB. Retained
stdout plus stderr share one aggregate 4 MiB limit. Read/write work is chunked
and checks a monotonic deadline. Success requires confirmed child exit AND EOF
on both streams. Overflow, cancellation, timeout and I/O failure withhold typed
decode, even if the captured prefix is valid JSON. Complete nonzero typed
reports remain available for their existing consumers to interpret.

On incomplete capture, send TERM only to the directly owned Foundation Process
and allow at most 250 ms exit observation. There is no process-group or
descendant cleanup claim, no foreign PID signal, and no unbounded wait for EOF
or exit. If exit is still unknown, retain that cleanup debt explicitly; stopping
capture does not mean the operation stopped. Kernel process launch and executor
scheduling are not hard real-time bounded. Output is a bounded in-memory receipt
and Activity projection, not a newly introduced durable forensic store.

## Verification and recovery

Consume the specific successful post-action diagnosis returned to the verifier,
not shared `engine.health` after awaiting. Failed/incomplete/malformed reads
remain unverified, preserve existing timestamp/error freshness, and never imply
resolution. A fresh clear observation reports current health only, not proof
that the preceding action caused recovery. For incomplete execution, inspect
current evidence and existing Ma'at/Stack Lab recovery routes before renewing
intent; never auto-retry a possibly committed mutation.
Each action start and newer verification read clears the prior verdict and
invalidates older read generations. A delayed old clear cannot overwrite a
new action's failed/unknown verification.

## Qualification still required

Source fixtures: `RemediationExecutionTests` (semantic/argv/repo drift, once-only
non-JSON/error, failed diagnosis, nonzero report compatibility, aggregate overflow,
blocked stdin, TERM ignoring, retained pipe writers, final launch denial).
Fixtures use only owned inert temporary scripts with finite self-expiring
children, not a model/service or live remediation payload.

Main's normal qualification `a3b97d60e46849e0b4c929291431c14c` built the revised
source and ran 80 Swift tests: two skipped and zero failures, including 15
remediation fixtures. Independent source recheck resolved report-scope and
verification-generation findings. Sanitizer/negative controls, immutable review
and installed review/confirm/execute/reverify/recovery UX remain separately
required. SSA owns publishing and runtime rollout; this is not task completion
or installed proof.
