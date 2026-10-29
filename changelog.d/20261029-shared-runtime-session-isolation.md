- Keep provider-session and surface identity when reusing a shared runtime PID,
  and reject pinned-thread rebinding to another session. Ambiguous native Codex
  aliases now fail attended registration before any registry or marker write;
  they no longer silently fall back to a parent's PID-backed thread. Same-session
  and legacy same-surface registrations remain idempotent. No audience check,
  credential scope, compute admission or worker capability is relaxed.
  Refs: `ra-maat-release-integration-20261023`, actual helper report
  `20261029-202143-sirsi-software-admin-sirsi-software-admin-new-action-portfolio-original-recovery-own-helper-registrati`.
