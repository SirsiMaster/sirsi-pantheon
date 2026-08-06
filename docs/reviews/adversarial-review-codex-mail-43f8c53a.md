# Adversarial Review — `fix(router): canonize codex-mail dispatch lane` (43f8c53a)

**Reviewer:** claude-pantheon  
**Date:** 2026-08-06  
**Commit:** `43f8c53a` — `fix(router): canonize codex-mail dispatch lane`  
**Branch source:** codex-pantheon  
**Merged:** PR #549 (`SirsiMaster/codex/canonize-codex-mail`)  
**Requested by:** codex-pantheon router item `20260806-080611`

---

## Verdict: PASS ✓

No blocking findings. Two low-severity observations noted below (non-blocking, informational only).

---

## Change Summary

Single file modified: `.agents/idea-router/agents.json`  
Adds a `codex-mail` agent block (32 lines) with:

- `id`: `codex-mail`  
- `type`: `codex`  
- `command`: `codex exec -C <pantheon> --sandbox workspace-write`  
- `cwd`: `/Users/thekryptodragon/Development/sirsi-pantheon`  
- `workstream`: `executive-mail`  
- `wake.mechanism`: `launchagent`  
- `wake.launch_agent_label`: `ai.sirsi.router.wake.codex-mail`  
- `consumer.command`: same as `command` + `--add-dir ~/.sirsi --ephemeral`  
- `consumer.prompt`: standard inbox-loop prompt (identical to other codex consumers)

---

## Adversarial Review Findings

### 1. Hardcoded absolute paths (LOW — known portfolio pattern)

`cwd` and `--add-dir` contain `/Users/thekryptodragon/...`. This is consistent with every other agent entry in `agents.json` (e.g., `codex-pantheon`, `codex-home`). The portfolio currently uses machine-specific paths here by convention. Non-blocking.

**Ceiling:** single-machine install. Multi-user / multi-host deployments will require path templating (future scope, not this PR's mandate).

### 2. No consumer.cwd field (INFORMATIONAL)

The `consumer` sub-object does not include its own `cwd` key. Other agents (`codex-pantheon`) likewise omit it. The Codex runner inherits `cwd` from the top-level key per the dispatch schema. Consistent with the pattern. Non-blocking.

---

## Test Verification

All relevant packages tested against the commit HEAD in worktree `/Users/thekryptodragon/Development/.worktrees/codex-mail-canon`:

```
go build ./...          OK  (linker warning: duplicate -lobjc — pre-existing, not introduced here)
gofmt -l .              OK  (no formatting violations)
go vet ./...            OK
go test ./internal/routerstore/...  PASS  6.432s
go test ./internal/dispatch/...     PASS  2.857s
go test ./internal/router/...       PASS  3.928s
```

---

## Wake Consumer: Installed and Live

LaunchAgent `ai.sirsi.router.wake.codex-mail` was confirmed loaded before this review:

```
$ launchctl list | grep codex-mail
77734   0   ai.sirsi.router.wake.codex-mail
```

Plist at `~/Library/LaunchAgents/ai.sirsi.router.wake.codex-mail.plist`:
- `ProgramArguments`: `sirsi router wake-loop codex-mail`
- `KeepAlive`: true  
- `RunAtLoad`: true  
- `ThrottleInterval`: 60s  

---

## Routed Store Mutation Proof

To prove the dispatch lane is fully wired — from router send through the durable store and back out via pull — a live send was performed against an isolated test store (per `SIRSI_ROUTER_DB` override, so the proof never touches the production fleet db):

```
$ SIRSI_ROUTER_DB=/tmp/.../router.db sirsi router send \
    --from claude-pantheon --to codex-mail --type review \
    --title "adversarial-review: prove codex-mail store mutation (43f8c53a)" \
    --instructions "..."

  Sent claude-pantheon → codex-mail:
    20260806-083927-claude-pantheon-codex-mail-adversarial-review-prove-codex-mail-store-mutation-43f8c53a

$ SIRSI_ROUTER_DB=/tmp/.../router.db sirsi router pull codex-mail

  1 open items for codex-mail:
  • 20260806-083927-claude-pantheon-codex-mail-adversarial-review-prove-codex-mail-store-mutation-43f8c53a
      type: review
    from: claude-pantheon
      title: adversarial-review: prove codex-mail store mutation (43f8c53a)
      opened: 2026-08-06T08:39:27Z
```

The store row was committed, indexed by `to: codex-mail`, and pulled back correctly.  
**Store mutation PROVEN.**

---

## Conclusion

`43f8c53a` is a clean canonical registration — no logic change, only a JSON agent block addition. The lane is correctly structured, the LaunchAgent wake is installed and live, and the full dispatch→store→pull path is confirmed functional. The commit was merged via PR #549 and is in `origin/main`.

**Review disposition: PASS — canonize codex-mail dispatch lane accepted.**

Refs: PANTHEON_RULES.md A26 (Idea Router Workstream Protocol); ADR-035 (one dispatch authority); CHANGELOG [Unreleased]
