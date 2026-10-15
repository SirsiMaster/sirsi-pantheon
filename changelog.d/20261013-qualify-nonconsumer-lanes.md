- **fix(router):** `sirsi router ping` qualifies deliberate nonconsumer lanes instead of reporting
  a bare `UNSTAFFED`. Actor-only `type: service` lanes (`horus`, `mercury-runner` — never routed
  inbox work by design, no wake mechanism) and lanes with a declared `resident` consumer but no
  wake mechanism (`gemma-pantheon` — paused by the owner-authorized cloud cutover) now report
  "not a staffing gap" detail instead of the generic "no worker path" message, so alerts stop
  reading as a dispatch defect for lanes that were never meant to self-staff. The verdict stays
  `UNSTAFFED` (no worker path genuinely exists) — only the detail is clarified.
