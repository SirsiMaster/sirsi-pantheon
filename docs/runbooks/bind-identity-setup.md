# Runbook — install the `sirsi-bind` identity (OWNER, one-time, ~5 min)

Enforces the bind decided in [ADR-041](../ADR-041-IDENTITY-ENFORCED-BIND.md):
option **A scoped as C** — a second identity makes bind real, and only on
authority-model paths.

**Why you and not an agent:** creating a GitHub App and granting it review rights
are access-control changes. Agents don't perform those, and an agent that could
mint this credential unilaterally would defeat the separation the App exists to
create. Everything downstream of this runbook is already built and automated.

---

## 1. Create the App

<https://github.com/settings/apps/new>

| Field | Value |
|---|---|
| **Name** | `sirsi-bind` |
| **Homepage URL** | `https://github.com/SirsiMaster/sirsi-pantheon` |
| **Webhook** | **Uncheck "Active"** — this App never receives events |

**Repository permissions** — grant exactly these, nothing more:

| Permission | Level | Why |
|---|---|---|
| **Pull requests** | **Read & write** | submit the approving review |
| **Contents** | **Read-only** | read the head SHA being bound |
| **Metadata** | **Read-only** | mandatory, auto-selected |

Do **not** grant Administration, Actions, or Workflows. This identity approves;
it never merges, never pushes, never edits protection.

Click **Create GitHub App**.

## 2. Capture the App ID and private key

On the App's page:

- Copy the **App ID** (top of the page).
- **Generate a private key** → downloads a `.pem`.

Install both on the conduit host:

```bash
mkdir -p ~/.sirsi
echo "<APP_ID>" > ~/.sirsi/bind-app.id
mv ~/Downloads/sirsi-bind.*.private-key.pem ~/.sirsi/bind-app.pem
chmod 600 ~/.sirsi/bind-app.pem
```

> **Do not put this key in GitHub Secrets, CI, or the repo.** A key reachable from
> a workflow lets any PR mint the token and approve itself — the exact circularity
> ADR-041 removes. Local-only placement *is* the enforcement boundary.
> `.gitignore` covers `*.pem`; the gitleaks check is a backstop, not permission.

## 3. Install it on the repo

App page → **Install App** → your account → choose the scope → **Install**.

Scope is your call. `sirsi-pantheon` alone is the minimum (it is the only repo with
`binding-hold.yml` today). **All repositories** is also fine and avoids re-doing this:
the App can only *submit an approving review* — no merge, no push, no settings — so on
any repo without the gate wired, its approval is inert. Wider install ≠ wider power;
the enforcement boundary is the local key, not the repo list.

## 4. Verify — end to end, no guessing

```bash
# identity resolves and the App can see the repo
scripts/bind/sirsi-bind.sh --help

# real check: bind the next open authority-model PR (one touching a gated path).
# (The original test PR #218 has since merged — use whatever authority PR is open.)
scripts/bind/sirsi-bind.sh <pr-number>
```

Expect `✔ PR #<n> bound on <sha>` and the `binding-hold` check flipping to green
within ~30s. If it stays red, read the check log — it prints the author login and
head SHA it required. With no authority PR open, verify the identity alone by minting
an installation token (see ADR-041 §Verify).

> **✅ Completed 2026-07-22.** App `sirsi-bind` (App ID **4361030**) created, private
> key placed at `~/.sirsi/bind-app.pem` (perms `600`), installed **org-wide**.
> Token-mint verified against `sirsi-pantheon`, `FinalWishes`, `SirsiNexusApp`, and
> `Assiduous`. To extend the gate to another repo, wire `binding-hold.yml` into it and
> note it in that repo's canon.
>
> **Live in two repos as of 2026-07-29:** `sirsi-pantheon`, and **`SirsiNexusApp`**
> (ported in SirsiNexusApp PR #206, merged `dd0ed63d`; authority paths there are
> `.github/`, `scripts/bind/`, `proto/`, `packages/sirsi-auth/`, `SIRSI_RULES.md` plus its
> synced `CLAUDE.md`/`GEMINI.md`, and `docs/ADR-*`). Branch protection requiring the check
> is **not yet enabled in SirsiNexusApp** — see the sequencing warning below before turning
> it on.

## When the gate WEDGES instead of failing — toggle a label, never push

**The failure mode:** the `binding-hold` run sits **queued indefinitely** and never reports, so the PR
stays BLOCKED with every other check green. `gh run cancel` can return HTTP 500 and `gh run rerun`
refuses with *"already running"*, so neither clears it. Observed on PR #391, run 30497693695.

**Do not push a commit to shake it loose.** A new head SHA is exactly what drops every bind already
recorded — the binds are pinned to the head SHA on purpose — so the "fix" costs you the reviews you
just collected and puts you back at the start. That is the circular trap this gate is designed to
create, and it catches you hardest when you are in a hurry.

**Do this instead.** `binding-hold.yml` triggers on
`[opened, synchronize, reopened, labeled, unlabeled]`, so **toggling any label re-fires the gate at the
same head SHA** and the binds survive:

```bash
gh pr edit <n> --repo <owner>/<repo> --add-label documentation
gh pr edit <n> --repo <owner>/<repo> --remove-label documentation
```

Two fresh runs complete and the PR goes CLEAN. Note the label must be applied by a **real user token** —
a label applied via `secrets.GITHUB_TOKEN` does not re-trigger the workflow (GitHub suppresses
GITHUB_TOKEN-triggered runs to prevent recursion), which is the same mechanic documented in
`binding-hold.yml` and the reason the gate computes sensitivity inline rather than in a labeling job.

Credit: claude-io found and used this on #391 after the gate wedged.

## Enabling branch protection in a repo that already has open PRs

Ordering matters and getting it backwards wedges every open PR permanently.

`binding-hold` does **not** report on a PR whose runs predate the workflow — the file is in neither that
PR's head nor its base. If you require the check before those PRs acquire it, they sit forever at
`expected — waiting for status` with no way to clear them.

Merging the workflow to `main` fixes the *base* but does **not** retroactively create check runs on
already-open PRs. So:

1. Bind and merge the PR that adds `binding-hold.yml`.
2. **Re-run or synchronize every already-open PR**, then confirm with `gh pr checks <n>` that
   `binding-hold` actually **appears** for each one.
3. Only then enable branch protection requiring the check.

Do step 3 before step 2 and you get the same permanent wedge, one move later. Same caution applies to
**renaming** a required check: protection keeps expecting the old name forever, so rename before
protection references it.

## Router-only rejection clearance on the selected head

The normal binder reads the canonical router channel before posting APPROVE.
It passes the selected repository, PR number and current full head SHA into
`scripts/bind/router-rejection.jq`. Each unresolved review/decision rejection
remains a separate hold. A later ACCEPT/PASS title, a timestamp, a SHA mentioned
in prose, or another reviewer's positive verdict cannot clear it.

To explicitly resolve a finding, its original canonical `from` sender posts a
new review/decision with exactly one binder-specific declaration in its existing
body. The following fixed-order, seven-line fenced block is a declaration of an
accepted review, not an example to quote into a live verdict inadvertently:

````text
```sirsi-bind-verdict-v1
repo: SirsiMaster/sirsi-pantheon
pr: 927
head: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
verdict: ACCEPT
resolves: exact-router-rejection-item-id
```
````

Replace the example with the actual accepted full 40-hex head and exact rejection
item ID. Multiple own findings may be listed comma-separated with no spaces.
Every rejection must have a matching resolution by its own sender. Sender
equality uses the existing canonical router `from` field; this block adds no
router protocol, schema, key, identity credential or approval authority.

The marker, field names/order and verdict are exact. No extra/duplicate fields,
extra declaration blocks, missing fences, abbreviated hashes, empty resolution
lists or unrecognized values count as positive proof. Repository and SHA matching
are case-insensitive. Prose outside the block is explanatory, not the positive
clearance declaration; do not use this reserved marker for quoted counterexamples
inside an actual review. A rejection title contradicting an ACCEPT block never
counts as clearance or scopes a hold away.

A declared REJECT uses the same shape with `verdict: REJECT` and `resolves: -`.
This allows a known unrelated repository's rejection to remain out of scope even
when its PR number coincides. Historical rejection titles naming this PR but
without reliable declared repository metadata remain conservative holds. Unknown
historical accepts never clear them. They are not permanent gates: a new exact
head acceptance from the original sender can name those historical rejection IDs;
the existing explicit `--override-pr <n> --override-finding <text|@file>` route is
unchanged for an actual owner override. Missing sender custody is not guessed.

The ordinary request-changes path is unchanged. The posted review remains pinned
to the queried `HEAD_SHA`; router resolution does not substitute an old commit.
Reading/parsing failure still refuses approval, rather than claiming no rejection.

Qualification uses `scripts/bind/router-rejection.test.sh` and the actual normal
binder fixture `scripts/bind/router-rejection-bind.test.sh`. The latter supplies
only temporary fake App ID/key plaintext and closed gh/sirsi/openssl doubles in
an isolated environment; it performs no real signing, network or review mutation.
Its optional absolute script-path argument is test-only, for an independent
guard-omission negative control. There is no production bypass switch. Main owns
execution, negative-control and publication qualification; source is not a live
bind receipt and does not gate the independent urgent 125 rollout.

## What is now true, and what still isn't

**Enforced by GitHub, not convention:**
- A PR touching `.github/`, `scripts/bind/`, `cmd/sirsi/`, `internal/router/`, `PANTHEON_RULES.md`,
  or `docs/ADR-*` cannot merge without an approving review from a non-author identity.
- That review is pinned to the head SHA — push after a bind and the bind drops.
- `SirsiMaster` (every agent) cannot satisfy it on its own PRs. GitHub forbids
  self-approval; that primitive is not forgeable.

**Still honor-system, stated plainly:**
- Any process on this host that can read `~/.sirsi/bind-app.pem` can bind. The
  separation is credential placement, not cryptographic proof of a human. It closes
  the #213–217 class (an agent clearing its own hold through the same automation it
  wrote); it does not make binds unforgeable by a compromised host.
- Ordinary product PRs remain fully autonomous by design — that is option C's scope,
  not an oversight.
