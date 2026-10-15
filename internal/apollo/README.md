# Apollo client boundary

Pantheon consumes Apollo's authenticated service contract; it does not implement
inference, tokenization, model loading or native rank execution. Offline planning
and single-host v3 remain separate. `cluster.go` projects exact service950c778d.

Remote CLI commands are disabled until enrolled endpoint/session authentication
is integrated. `NewClusterClient` rejects nil clients, plaintext and redirects.
A non-nil dependency-injected client is not proof of authentication. Never use
router credentials, secret flags or locally minted enrollment authority.

`Prepare` sends conversation text only to Apollo's preparation endpoint and
validates frozen response identities. `CreateJob` derives its request from that
preparation and checks prepared rank_count, contiguous unique-host fences and
live resources. The service does not expose host inventory; exact host/budget
matching is service-side `validAdmission`, conditional on trusted integration.

`Status`, `Cancel` and `Events` take the original admitted Job. Responses/events
must match its complete execution tuple. Original expired fences may be used for
recovery; fresh admissions still require live expiry. Never replace original
identity with a status snapshot on reconnect. Caller-owned receipt/cursor storage
must remain private and scoped to the authenticated conversation.

Streams have per-event limits, no cumulative byte cap, and require final
released/orphan evidence. Early EOF or interrupted/oversized streams return
IncompleteStreamError with LastSequence. Completed/failed alone is not teardown.
The authorized event text is not telemetry. Cancellation remains disabled until
the service guarantees immutable nonreuse or atomic stale-target preconditions.

Run the whole relevant suite:

```sh
go test -race ./internal/apollo -count=1
go test ./cmd/sirsi -run 'Test(Apollo|SplitApollo)' -count=1
go vet ./internal/apollo
golangci-lint run ./internal/apollo/...
```

See `docs/architecture/APOLLO_CLUSTER_CLIENT_V1.md` and
`docs/user-guides/apollo-cluster.md` for current limits and enabling gates.
