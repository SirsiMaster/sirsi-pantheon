# Enrolled signing client

`sirsi-sign ARTIFACT [codesign|bin|app|pkg|dmg]` sends an artifact to an
already-enrolled signing Mac and verifies the returned Sirsi Developer ID
signature. It never enrolls a signer, imports a certificate or copies a private
key. Commercial Pantheon builds use this client through
`PANTHEON_SIGNING_EXECUTION=remote-service` and `PANTHEON_SIGN_CLIENT`.

## Transport contract

Every SSH operation, both SCP directions, and rsync's SSH transport use the same
settings: `StrictHostKeyChecking=yes`, `BatchMode=yes`, password and keyboard
interactive authentication disabled, and a ten-second connection timeout.
Only existing host trust and existing noninteractive credentials are used.
Unknown/changed host keys or unavailable authentication fail rather than opening
a dialog, learning a key, or waiting for a person. `SIRSI_SIGN_HOSTS` retains its
existing ordered primary/standby selection; it does not authorize enrollment or
host-key installation.

The default ordered targets are `sirsi-m5`, then `sirsi-m1`: existing enrolled
SSH aliases supply the destination user, host address and pinned host identity.
The client does not create those aliases or add trust. DHCP address changes no
longer require editing the client. Missing aliases, DNS, credentials or trusted
host identity still fail closed; an explicit non-empty `SIRSI_SIGN_HOSTS`
continues to replace the ordered list. An unset or empty override uses defaults.

An enrolled WatchPaths daemon can be idle with exit status zero: `not running`
alone does not mean the signer is broken. The client preflight checks enrollment;
actual artifact signing/notarization still requires the enrolled service to
complete its work. No screen unlock or account-security change is performed by
this client. A failed reachability/authentication check is not evidence that a
certificate is missing.

## Verification and limits

Run `go test -race ./tools/signing` on a host with zsh. Fixtures execute the actual
client with isolated fake transport and signature-verification programs. They
exercise file/app publication, waiting, readback, cleanup, refusal and a foreign
signature, and reject an `accept-new` negative control. They contact no real
host, read no credentials, and establish transport behavior only—not actual
signing, notarization, installation or multi-machine inference success.

Additional denial-only fixtures check default/empty/explicit ordered target
selection and causally reject the former DHCP address defaults. These inspect
argv to fake transport only; they do not prove live alias reachability or submit
a signing job.

The existing result-signature checks are unchanged.

## Bounded transfers and failover (2026-11-05)

- Every SSH, SCP and rsync call also carries `ServerAliveInterval=15` and
  `ServerAliveCountMax=4`, so a link that dies mid-transfer ends in about a
  minute. Each upload and download is also bounded overall by
  `SIRSI_SIGN_TRANSFER_TIMEOUT` (default 900 s).
- The job ID is validated before any host is contacted.
- **Failover keeps one job ID and never double-signs.** The client moves to
  the next host in `SIRSI_SIGN_HOSTS` only when the job provably never entered
  the first signer's queue: staging, upload, or a rename the host reports as
  not done. Its hidden staging is then removed. Two cases stop instead:
  - an enqueue whose outcome is unknown (exit 6);
  - a job ID already present on a signer (exit 7).

  Either is reconciled by that ID, never resubmitted. With
  `SIRSI_SIGN_CUSTODY_DIR`, each host attempt is appended to `<id>.attempts`;
  the custody record itself is never rewritten.
- **Reconcile from `<id>.attempts`, not `<id>.json`.** The custody record names
  the first enrolled host and is written before staging; after a failover the
  host that holds the job is the `result=enqueued` line in `.attempts`. The
  client also prints that host when the job is queued.
- If a rename over SSH reports failure while the job still looks staged, the
  client removes the staging and re-tests in one call. Rename is atomic, so the
  job is then either queued (continue there) or provably absent (fail over);
  anything unreadable stops with exit 6.
- A caller-set `SIRSI_SIGN_JOB_ID` requires `SIRSI_SIGN_CUSTODY_DIR` (exit 2
  otherwise): its exclusive record is what stops a rerun from enqueueing the
  same ID on a second host while the first is unreachable.
- Exit 8: the job finished on the signer but its artifact could not be fetched
  (download failed or timed out), or it has no artifact (rejected, or pruned).
  The result stays on the signer under its ID. Exit 5 remains a returned
  artifact that failed Team verification.

`sirsi-signd.sh` (the signing daemon, previously only on the signing Mac) is
versioned here:

- **Timestamp retry:** "The timestamp service is not available" is retried up
  to 3 times (10 s, then 30 s) for the same job and input. Any other failure,
  or a third outage, fails the job as before. Nothing unsigned is ever
  published.
- **Outbox pruning is OFF by default.** Signed artifacts are never deleted
  unless the signer's custodian sets `SIRSI_SIGND_KEEP_DAYS` (whole days) in
  the daemon's environment. When set, a finished job older than that loses its
  artifact; its `receipt.txt`, its `done`/`failed` marker and `audit.log` stay
  forever, so every job ID remains reconcilable. Unfinished jobs are never
  pruned.

The fixtures cover failover on a failed and a hung upload, refusal to fail
over after an ambiguous enqueue, the late-landing rename race (rename wins,
rm wins, unreadable), refusal of a caller ID without custody, existing-ID
refusal, a ServerAlive negative control, timestamp retry/success, fail-closed
after three outages, no retry of other failures, no pruning by default, and
opt-in pruning. They do not qualify real signing, notarization,
Apple's timestamp service or live multi-host failover.

## Commercialization profile

- Classification: platform-foundation; not an independently launchable product.
- User: Sirsi release operators and the enrolled native build service.
- Pain: an unattended release must not create authentication/trust prompts or
  silently accept a new host identity in a later transfer phase.
- Workflow: submit to an existing pinned signer, retrieve and verify the result.
- Commercial contribution: dependable distribution and support; standalone
  willingness to pay is UNKNOWN and not claimed.
- Trust boundary: artifact bytes cross the existing SSH boundary; signing keys
  remain on enrolled signing hosts; host trust is never expanded automatically.
- Operational owner: SSA installed-package custody, coordinating the canonical
  signing service and Pantheon release source owner.
- Done evidence: source/race/negative fixtures, exact review and release
  integration, followed separately by a real signed-package/readback receipt.
