# Plan — Ra / Horus Fabric

### Shipped foundation

1. Keep registry, runnable predicate, leases, wake events, state derivation and
   completion contracts aligned.
2. Resolve every client through the canonical service or an explicit disposable
   test store; implicit local production fallback is forbidden.
3. Test claim/expiry/idempotency, event synthesis, body integrity, read ack,
   authentication, migration and restart recovery.
4. Operate the host relay as the only token holder for network-isolated lanes.

### Release-candidate work

5. Publish this architecture and recipe as one Ra Stack Lab wing with exact
   source/test/build/rollback receipts.
6. Reconcile the live service revision, schema and per-host client identities
   into a durable receipt; do not reconstruct missing cloud facts.
7. Run the isolated third-machine rehearsal and record it as rehearsal, not as
   physical-fleet proof.

### Next bounded increment

8. Implement the router-owned informer as an additive supervisor. It must select
   delivery from declared registry type/session mode, keep all existing wake
   safety gates, and refuse duplicate host ownership. It must land with a
   positive delivery/read-ack proof and a negative duplicate/interactive-spawn
   control.

### Completion boundary

9. Resolve external peer mappings, installed-runtime recovery, fresh-host proof,
   and commercial narrative/design evidence before calling Ra GA. Until then the
   release classification is platform-foundation/pilot.
