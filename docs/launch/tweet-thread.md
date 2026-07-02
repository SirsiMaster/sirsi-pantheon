# Launch tweet thread

A 6-tweet launch thread. Voice: plain, confident, no hype. Every claim is
verifiable in-repo (81 rules, local-only, zero telemetry, trash-first, Apache-2.0,
macOS-first). Attach `assets/demo.gif` to tweet 1.

---

**1/ (hook — attach demo.gif)**

Your Mac, self-healing.

Pantheon finds what's dragging your machine down, names the cause in plain English,
and fixes it — trash-first, with before/after proof.

100% local. Zero telemetry. Open source.

🧵

---

**2/ (the problem)**

Activity Monitor and btop are great at one thing: watching. They show you a red
number and stop.

Closed-source "cleaners" fix things — but you can't see what they delete, and they
phone home.

Pantheon's job starts where watching ends: name the cause, then fix it.

---

**3/ (the loop)**

`sirsi scan` — 81 rules find caches, build artifacts, and stale dev cruft.
`sirsi clean` — previews what's safe to remove.
`sirsi ghosts` — leftovers of apps you already uninstalled.
`sirsi diagnose` — RAM, disk, kernel panics, in one brief.

Every finding maps to a one-command fix.

---

**4/ (safety is the product)**

It deletes files for a living, so safety is the design:

• Trash-first — recoverable, full decision log
• Dry-run by default — preview = apply, exactly
• 25 hardcoded protected paths — keychains, SSH keys, your home folders can never be
  touched

Weigh. Judge. Purge.

---

**5/ (why local)**

A tool that deletes files should not also ship your data.

No account. No cloud. No analytics. Nothing leaves your machine — ever.

It's a design rule, not a settings toggle. And because it's open source
(Apache-2.0), you can verify it.

---

**6/ (install + CTA)**

One Go core, every surface: CLI, macOS menu bar, a local browser dashboard, and an
MCP server for AI IDEs.

macOS-first (Apple Silicon + Intel). Beta.

    brew tap SirsiMaster/tools && brew install sirsi-pantheon
    sirsi scan

→ github.com/SirsiMaster/sirsi-pantheon
