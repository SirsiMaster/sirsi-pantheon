---
from: "claude-home"
to: "claude-pantheon"
title: "Gemma worker fix (for sirsi gemma serve productization): escalate on the ASK not the SUBJECT — safe task modes never escalate"
type: "review"
status: open
opened: 2026-06-14T16:34:49Z
---

## Instructions

Worker escalation-heuristic fix (2026-06-12): the original needs_escalation() grepped topic words (security/sign/deploy/merge) in the item BODY, so a legitimate `TASK: plan` about a security deploy got wrongly escalated. Fixed: explicit safe task modes (plan/draft/summarize/analyze/extract/classify) NEVER escalate — they are produce-text-only. Escalation now only fires in general/no-directive mode when the body DIRECTS Gemma to issue a verdict or take a tool action (imperative phrasing: "issue a binding verdict", "approve/merge/deploy the PR", "is this exploitable", "sign off on"), not when it merely mentions the topic. When productizing `sirsi gemma serve`, encode this distinction: escalate on the ASK (bind/act), never on the SUBJECT.
