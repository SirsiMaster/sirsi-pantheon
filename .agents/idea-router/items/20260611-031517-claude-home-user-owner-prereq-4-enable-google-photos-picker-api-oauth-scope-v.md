---
from: "claude-home"
to: "user"
title: "OWNER PREREQ #4: enable Google Photos Picker API + OAuth scope + VITE_GOOGLE_OAUTH_CLIENT_ID — FW PR #5 ready"
type: "decision"
status: open
opened: 2026-06-11T03:15:17Z
---

## Instructions

**New OWNER PREREQ** for FinalWishes — Google Photos import (CR-12) ready in code (PR #5 PASS-advisory by claude-home).

To activate the feature on prod, three infrastructure changes you control:
1. **Enable Google Photos Picker API** on `finalwishes-prod` GCP project
2. **Add OAuth scope** `https://www.googleapis.com/auth/photospicker.mediaitems.readonly` to the OAuth consent screen
3. **Set `VITE_GOOGLE_OAUTH_CLIENT_ID`** (the project's Web OAuth client id) as a build-time env var in CI

Until configured, the "Import from Google Photos" button correctly throws "Google Photos import is not configured" — fail-loud, no silent degradation. This is OWNER ACTION #4 alongside the existing 3 (OPENSIGN_WEBHOOK_SECRET, CI SA datastore.indexAdmin, TCC reinstall test).

claude-finalwishes built; claude-home reviewed source-deep; both bless the code. Codex-FW post-review when they return.

— claude-home (surfacing aligned recommendation, 2026-06-11 03:14 UTC)
