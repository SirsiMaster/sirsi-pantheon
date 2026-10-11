# Router prior art (M0 chapter, Ra charter 2026-10-09)

Purpose: for every wall the router has, say what the best existing systems do, what we copy, what we reject.
Status labels are honest: **READ** = I fetched and read the source on 2026-10-11; **RECALL** = from background
knowledge, not re-read, must be verified before anything is built on it.

## What nothing else models (our real novelty)
1. Spend, not CPU, is the scarce resource: token and subscription budget admitted before dispatch
   (Cloud Worker Funding Law). LLM gateways do budgets; none we know reserve worst-case cost across lanes.
2. The owner is a distinct principal that agents must not impersonate (the F1 finding on the owner console).
3. A worker is an LLM session with a spawn cost and an attended/unattended distinction.

## Reading 1: Graphile Worker (Postgres job queue) — READ
- Workers learn of new jobs via Postgres `LISTEN/NOTIFY`; the docs claim "typically under 3ms from task schedule to
  execution" (vendor claim, not measured by us). Claims use `SKIP LOCKED`. At-least-once execution, exponential
  backoff, default 25 attempts over ~3 days.
- FAQ: polling is kept ON ALONGSIDE LISTEN/NOTIFY (default poll 2 s, tunable to 30-60 s) because scheduled and
  retried jobs have no event at the moment their time arrives, and a notification can be missed.
- Lesson for us: the stored job is the truth; the notification is only a hint. A missed hint must cost latency,
  never correctness. Copy: event wake + a slow safety poll. Reject: nothing; our spool files already are the truth.

## Reading 2: qmail's queue trigger — READ (via the qmail INTERNALS notes and the O'Reilly qmail book excerpt)
- qmail-queue notifies qmail-send by writing one byte to a named pipe (`lock/trigger`); qmail-send waits for it to
  become readable before rescanning `todo/`. A byte persists in the pipe until read, which beats a signal that can be lost.
- Operational note in the book: a damaged trigger shows up as a ~30 s delay before mail moves.
- Lesson for us: our relay spool is Maildir-shaped; the same trick (a persistent wake token, files as truth) removes
  the 150 ms double poll. A kqueue directory watch gives the same hint without a FIFO per lane; choose by what the codex
  no-network sandbox permits. The 60 s figure I had in mind was NOT confirmed by any source and is not claimed.

## Reading 3: Dominant Resource Fairness (Ghodsi et al., NSDI 2011; implemented in Mesos) — READ (abstract level)
- Generalizes max-min fairness to multiple resource types: equalize each user's share of their dominant resource.
  Claimed properties: sharing incentive, strategy-proofness, envy-freeness, Pareto efficiency (under Leontief demands:
  fixed proportions).
- Critique to keep in view: Bonald and Roberts argue proportional fairness gives a better efficiency/fairness
  trade-off and that strategy-proofness matters less under dynamic demand.
- Lesson for us: our consumer slots are a single-resource cap plus a hand-made reserved slot. The honest multi-resource
  version has at least three dimensions per consumer (host CPU slot, token budget, attended-session slot).
  Copy: dominant-share accounting to decide who gets the next slot. Reject for now: strategy-proofness (lanes are not
  adversarial); fixed proportions (LLM sessions are not Leontief).

## Survey table (RECALL: verify before building)
| Our problem | Prior art | Take |
|---|---|---|
| Claim/lease/retry on Postgres | River, Oban, pg-boss, SQS visibility timeout | retry policy, dead-letter handling, fenced leases (Kleppmann) |
| Durable multi-step work | Temporal | heartbeats, activity timeouts, replayable history |
| Delivery guarantees, priority | NATS JetStream, RabbitMQ (prefetch QoS, DLX), Google Pub/Sub (ack deadline) | pull consumers, redelivery policy, priority |
| Heterogeneous machines | HTCondor ClassAds | capability advertisement + requirement matching |
| Fair slots | Slurm fairshare, Kubernetes priority classes, Mesos DRF | share accounting, preemption policy |
| Supervision | Erlang/OTP supervisors, Kubernetes controllers | restart intensity limits (our 10-fruitless counter is a crude one), leader-election leases |
| Registry from Git | GitOps (Argo CD, Flux) | our `registry sync` pin is the same idea |
| Agent interop | Google A2A protocol, MCP (tools only) | task lifecycle vocabulary for outside agents |

## What this changes in the plan
M0-01 measured the CLI read through the spool at p50 420 ms against an 88 ms network floor, with two 150 ms polls
suspected (H3 still untested). Reading 1 and 2 give the M1-01 design: event hint + slow safety poll.
**New, bigger suspect (not yet measured):** the wake loop checks for work on a 1-minute interval (logs show
`interval=1m0s`), so an item can wait up to 60 s before a lane even starts. That dwarfs a 150 ms poll. M0-02 below measures it.

## M0-02 pre-registration (written before measuring)
Metric: time from `router send` returning to the consumer's first `router acknowledge`, per lane type, n>=10 each.
H1: median is dominated by the wake interval (median >= 25 s for a wake-loop lane with the default 1 m interval).
H2: an attended or event-woken lane acknowledges within 2 s of the send.
Fail: either fails. Negative control: a lane with no consumer must show no acknowledgement within the window.
Constraint: sends go to real lanes only with the owner's standing leave to message them; use Ra-to-Ra self-sends
where possible so no other lane is woken or billed.
