# Hosted router board

The hosted board is the owner's router console on Cloud Run (`sirsi-router-board`), reachable from any device
by a signed-in **sirsi.ai** Google account through IAP. It shows every lane, mailbox, thread and ledger task
the way `sirsi dashboard` does on a Mac, and it can **edit the ledger**.

## What you can change

Open an item or a task; the inspector shows an **Edit as you@sirsi.ai** bar. **Add task** sits beside
Refresh.

| On | Action | What it does |
|---|---|---|
| Router item | **Close** | Closes the item. The result reads `board:you@sirsi.ai: <your note>`. |
| Router item | **Make task** | Adds a ledger task for the recipient (subject = the item title, commissioned by `board:you@sirsi.ai`), then closes the item, naming the task. |
| Router item | **Reassign** | Moves the item to another agent's inbox with your note. |
| Ledger task | **Set status** | `pending` or `blocked`. |
| Ledger task | **Blocked by** | A task id or a reason; empty clears it. |
| Ledger task | **Rename** | Replaces the subject. |
| Ledger task | **Reassign** | Changes the responsible party. |
| Anywhere | **Add task** | A new task on any agent's ledger. |

Every task edit appends a timeline entry owned by `board:you@sirsi.ai`, so the ledger shows who changed what.

## What you cannot change, and why

- **Send messages, delete anything, or control lanes, threads, sessions or tokens.** The board's token
  carries only `@observer` (read the ledger) and `@ledger-editor` (the edits above). The router service
  refuses everything else no matter what the page asks.
- **Claim, complete or release a task.** A task lease belongs to the lane doing the work, bound to that
  lane's own authenticated thread. The board relabels and reassigns; the lane executes. The router also
  refuses `in-progress` or `done` without a lease.

## How sign-in is checked

IAP admits only `domain:sirsi.ai` (`board.sh verify` proves the policy). On top of that, every edit checks
IAP's signed assertion (`x-goog-iap-jwt-assertion`, ES256 against Google's published keys) for this
service's audience and a sirsi.ai account. An edit must also come from the board's own page: same origin and
the `X-Sirsi-Board: 1` header, so another site cannot ride your IAP session. If the audience is not
configured, every edit is refused.

## Operating it

```bash
JOB_SA=<mint-sa>@sirsi-nexus-live.iam.gserviceaccount.com scripts/router-service/board.sh provision
JOB_SA=<mint-sa>@sirsi-nexus-live.iam.gserviceaccount.com scripts/router-service/board.sh mint
scripts/router-service/board.sh deploy
```

`mint` issues the token with `--agents router-board,@observer,@ledger-editor` straight into Secret Manager.
`deploy` sets `SIRSI_BOARD_IAP_AUDIENCE=/projects/<number>/locations/<region>/services/sirsi-router-board`
and ends with `verify`. `DRY_RUN=1` prints the gcloud commands without changing anything.
