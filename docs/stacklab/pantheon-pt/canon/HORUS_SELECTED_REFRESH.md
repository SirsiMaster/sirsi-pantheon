# Exact Horus selected configuration refresh

Feature-freeze corrective source after root-relay controller `2049e8f6`, for the
existing Horus supervisor only. SSA integrates the successor; this recipe is not
installed-client or successful-registration evidence.

The existing `sirsi launchd restart ai.sirsi.horus.agent-router` operation plans
read-only by default. Plan and apply use the existing canonical remote actor,
own-thread/runtime and exact registry-grant authorizer. Only SSA and SHA receive
the additional label grant. No wildcard/router-wide control, new authorization
framework, owner approval gate, elevation or daemon is introduced.

## Exact selected transition

- Native real/effective nonroot UID must agree. Resolve the account home via
  the OS account record, not caller HOME. Target is that owner's
  `gui/<uid>/ai.sirsi.horus.agent-router`; the sole plist is its existing
  `Library/LaunchAgents/ai.sirsi.horus.agent-router.plist`.
- Accept the shipped four-argument `/bin/zsh -l -c` wrapper only when its command
  is exactly literal `exec <absolute-payload> horus supervise`. Recognize the
  renderer's quoted form without evaluating shell syntax or running the payload.
  Pin shell digest, full legacy payload path/digest, plist digest and loaded
  PID/start. Require loaded `ps comm` to match that selected payload.
- Qualify root-owned, executable, non-group/other-writable protected replacement
  `/usr/local/libexec/sirsi-pantheon/sirsi`. Verify both payloads through exact
  `/usr/bin/codesign --verify --strict -R` with Apple generic anchor, Team
  `9D382WV988`, identifier `sirsi`. No payload execution occurs during planning.
- New arguments are exactly the protected path, `horus`, `supervise`; remove
  legacy Program when replacing the shell. Preserve unrelated plist semantics,
  WorkingDirectory, environment, KeepAlive, logs and scheduling fields, checked
  through a data-only plutil round trip. No shell/login or credential insertion.
  Inline credential-valued environment fields refuse with a specific residual:
  establish existing credential-free enrollment/selector first. Ordinary
  environment and credential-file/path selectors are not blanket-prohibited.
- The read-only plan includes replacement/configuration digests and a complete
  `plan_sha256`. Apply requires existing `--apply --drained` and all four original
  identity pins plus `--expected-plan-sha256`. This is a pin, not another
  permission gate. Already-direct configurations use this same selected operation.

## Preservation, drain and finite recovery

Apply retains the selected-service lock and repeatedly re-inspects instance,
artifacts, native disabled state and child enumeration. Unknown intent, fabric
quarantine, disabled Horus, child appearance, malformed probes or drift refuse
before retirement. A final check follows staging. No enable, bulk install-daemons,
unrelated launchd action, model or network configuration change.

Horus performs synchronous reconciliation and Go duties, including work without
an OS child. `--drained` means the owner has quiesced producers and in-progress
passes and retained a recovery plan. An empty child list does not prove this.
Legacy Horus has no atomic drain handshake: these are cooperative samples, not
exclusion against same-UID writers or a new pass after the last sample.

Preserve exact original bytes in a create-only mode-0600 sibling:
`.ai.sirsi.horus.agent-router.<original-sha256>.backup.plist`. Occupation refuses;
never overwrite even a seemingly equivalent backup. Retain original, backup,
stage and parent descriptors. Stage is random-named, private, fsynced and
full-byte/EOF read back. Resolve parent components without following links.
Revalidate named identities and parent continuity, publish with one same-directory
atomic swap, verify displaced/installed leaves, privatize the displaced original
and fsync the directory. This is not a kernel inode-CAS against an uncooperating
account owner. Unknown substitution is explicit debt, never an unlink/blind overwrite.

After confirmed bootout, recovery uses a fresh independent 15-second context.
Publish only the planned configuration. If publication fails before the new job
is bootstrapped, restore only verified retained transaction identities and try
the original selected configuration once. Preserve the publication failure even
when original-instance recovery succeeds. Changed identities refuse rollback.

A failed bootout gets an independent bounded read-only classification: original
instance remains, concurrent changed instance, or retirement outcome unknown.
None authorizes another bootout/bootstrap. A failed bootstrap is not proof of
absence: poll bounded readback without repeating bootstrap or guessing a legacy
rollback. Preserve its error even if the exact new instance is observed. Require
two stable fresh-instance readbacks with planned After artifacts. Outer caller
cancellation remains failure even if backend recovery succeeds. No backup/stage
is automatically deleted; record retained leaves/cleanup debt. Restored/displaced
originals may be mode 0600; original bytes remain exact.

### Selected recovery when no running job can be inspected

`restart` planning intentionally requires a running instance; it is not the retry
route for an absent job. The existing native GUI-owner mechanism is the exact
argv `[/bin/launchctl, bootstrap, gui/<native uid>, <selected Horus plist>]`,
reported in a failed-bootstrap receipt together with the original backup path,
intended selected plist hash, protected program path/hash. This is an operator
recovery instruction, never an automatic command runner or a new permission gate.

SSA/SHA recheck the retained recipe/After hashes, actual native GUI owner, current
disabled/quarantine intent and selected job state, then use that one bootstrap
operation only with the intended direct configuration still present. A failed
print alone is not evidence of absence. Bootstrap does not authorize an enable,
bootout, another label, configuration overwrite or an original-payload replay;
an already-loaded label must be observed, not retired as a retry. If the selected
bytes or identities differ, retain the debt and inspect before acting. Original
backup restoration after a new bootstrap attempt is not an inferred safe route.
Recovering original bytes requires separately verified exact selected identities
and retirement/state evidence; there is no automatic legacy rollback on this path.

Fixtures retain the explicit backup/After/program/argv evidence even when all
forty readbacks find no running job. A native data-only plutil fixture separately
checks nonalphabetical nested dictionaries; equality compares decoded JSON
semantics with numeric tokens preserved, not nested dictionary byte order.

## Qualification boundary

Fixtures use temp files and per-instance native doubles, never real launchctl,
codesign, model calls or host configuration. Main owns admitted normal/race/lint
and independent review; preserve the existing five-profile and root-relay suites.
SSA/SHA rollout must retain old/new artifact and loaded-instance evidence,
unrelated jobs, recovery/cleanup state, fresh canonical Horus registration and a
successful supervisor pass. A new PID or green launchd state alone does not cure
or qualify lifecycle-fence failures.
