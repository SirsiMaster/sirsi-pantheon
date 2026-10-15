# Apollo client boundary

Pantheon consumes the Apollo service contract; it does not implement inference,
tokenization, model loading or rank execution. Offline planning remains separate.

`cluster.go` is provisional platform groundwork. The CLI is disabled until an
enrolled authentication transport and authenticated plan integration exist.
`NewClusterClient` refuses nil HTTP clients; a non-nil injected client alone is
not proof of authentication. Integration must enforce the enrolled endpoint and
caller/session identity without borrowing router credentials or secret flags.

`CreateJob(ctx, request, expectedRanks)` requires a complete rank/host inventory
from the authenticated frozen plan matching `request.PlanSHA256`. The caller
must authenticate that plan; the method compares the returned admission with
that separate inventory. Never derive expected ranks from the response being
validated. No expected inventory is a rejection before network access.

`Status` and `Cancel` also require that same frozen admission and reject
changed execution, request, plan or epoch. `Events(ctx, admittedJob, after, emit)` uses the original admission tuple for
all events and reconnects. Never replace it with fresh status. The callback
receives typed token text, ID, position and committed count only after tuple and
sequence validation; zero IDs and positions survive JSON output. The caller owns
cursor persistence in its authorized conversation scope. No hardware telemetry
stores reply content.

The service schemas and endpoint enrollment still need actual implementation
and installed qualification. See `docs/architecture/APOLLO_CLUSTER_CLIENT_V1.md`
and `docs/user-guides/apollo-cluster.md` for current limitations.

Focused socket-free regression checks:

```sh
go test -race ./internal/apollo -run 'Test(EventsBindFrozenTupleAndPreserveReply|RankAdmissionMatchesCompleteFrozenPlan|MissingAuthenticationAndPlanRefuseBeforeNetwork)$'
```
