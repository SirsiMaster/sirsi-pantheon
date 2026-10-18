# Ma’at atomic reservation repair

Classification: platform-foundation. Operational owner: SSA/Ma’at with Ra
owning canonical router deployment. This serves fabric operators whose admitted
work was stopped by a slow full-registry sweep or a lost reservation.

## Mechanism and trust boundary

Production `schedule.Ledger` delegates reservation operations to the canonical
Store's `MaatReserve`, `MaatMutate`, and `MaatRead` RPCs. The SQL implementation
locks the existing `maat:reservations` row inside a transaction, reads the
current policy/state, applies the operation, and commits once. PostgreSQL row
locks serialize replicas; SQLite serializes write transactions. Expiry sweeps
cannot write a snapshot obtained before another reservation. Failed operations
roll back. Legacy direct SetState writes to that key refuse rather than silently
overwriting admitted work. No schema change, database fork, or record deletion.

The normal bearer/session/runtime/registration chain remains in force. New RPCs
are denied to lane-scoped cloud credentials until separately scoped; this fix
does not grant cloud lanes generic scheduler writes. The native supervisor still
checks exact host/holder/work/lease, actual process ancestry/start identity, and
admission before spawn and during execution. It must not substitute grace or a
cached ticket for canonical state. Executor registration binds the actual job
process on the destination machine, never an origin host's PID.

## Rollout and rollback

1. Qualify the exact source on SQLite and PostgreSQL; include concurrent grants,
   floor-share policy, heartbeat/release with expiry sweep, transaction rollback,
   remote roundtrip and scoped-token refusal, and registration negative controls.
2. Retain exact prior signed CLI and cloud revision identities. Coordinate Ra and
   active workload owners. Deploy the compatible canonical API and update M5,
   M1 and Studio clients together; legacy writers refuse until updated. Do not
   stop an owner's newer generator or create a competing generation process.
3. Verify native harmless admitted commands through the canonical service on
   each destination, including a heartbeat interval and release readback. Then
   observe the existing owner's generation actually adding data. Record failures
   and retained cleanup debt; do not close on a test-only pass.
4. Roll back clients and cloud as a coordinated set to the recorded prior
   identities if required. Do not restore an old ledger snapshot or delete newer
   reservations. Source and signed artifacts are preserved for investigation.

## Evidence status

Oct18 local SQLite full schedule/store suites, narrow registration tests,
atomic reservation race tests and CLI build passed under real native admission
`20261018T141618Z-m5-sirsi-software-admin`. Supervisor terminal
`~/.sirsi/evidence/maat-runs/66f6e35408824628a55ed172647da56e/terminal.json`
exited0. PostgreSQL, exact final source build and deployed generation progression
still require their own readbacks; this document does not claim those gates pass.

Three-home publication: canonical source is this runbook; Reading Room and
Workspace mirrors remain pending, not assumed synchronized.
