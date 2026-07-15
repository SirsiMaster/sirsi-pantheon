# Product Hunt launch copy

Tagline, description, and first comment. Voice matches the README and site: plain
English, "Your Mac, self-healing", "Weigh. Judge. Purge." Only claims verifiable
in-repo. No fabricated metrics.

---

## Name

Sirsi Pantheon

## Tagline (60 chars max)

Options — pick one:

1. **Your Mac, self-healing** (37) — recommended; matches the README hero.
2. **Find what slows your Mac, fix it — 100% local** (46)
3. **The open-source Mac cleaner that names the cause** (49)

## Topics

Mac · Developer Tools · Open Source · Productivity

---

## Description (short — the gallery blurb)

Pantheon finds what's dragging your Mac down, names the cause in plain English, and
fixes it — trash-first, with before/after proof. 100% local, zero telemetry, open
source. 81 rules for waste, ghost-app detection, and one-command fixes.

---

## Description (long — the product body)

**Most Mac tools either watch or delete. Pantheon does the part in between: it names
the cause, then fixes it — with a paper trail.**

Activity Monitor and btop show you a red number and stop. Closed-source cleaners fix
things, but you can't see what they touch and they phone home. Pantheon closes the
loop, in the open.

**What it does**

- **Scan** — 81 rules find infrastructure waste: package-manager caches, build
  artifacts (node_modules, Go module cache), stale dev cruft, cloud CLI caches. Every
  finding comes with evidence and a plain-English note.
- **Clean** — previews what's safe to remove. Dry-run by default; the amount previewed
  is exactly what gets moved to the Trash.
- **Ghosts** — finds leftovers of apps you already uninstalled.
- **Diagnose** — RAM pressure, disk, top consumers, kernel panics, in one brief.
- **Relieve** — calms the top CPU hog. Reversible; nothing gets killed.

**Safety is the product**

It deletes files for a living, so the safety design comes first: trash-first and
recoverable, dry-run by default (preview = apply), and 25 hardcoded protected-path
rules — keychains, SSH keys, and your home folders can never be deleted, not
overridable by any flag.

**One engine, every surface**

The same Go core drives the CLI, a macOS menu-bar app, a local browser dashboard, and
an MCP server so your AI IDE (Claude Code, Cursor, Windsurf) can ask "what's eating
this machine?" and act on the answer.

100% local. Zero telemetry. Apache-2.0. macOS-first (Apple Silicon + Intel). Beta.

    brew tap SirsiMaster/tools && brew install sirsi-pantheon

---

## First comment (from the maker)

Hi Product Hunt 👋

I built Pantheon because the two tools I reached for every day didn't do the job I
actually wanted. Activity Monitor and btop *watch* — they tell you the number is red
but not why, and definitely not what to do about it. The commercial cleaners *fix*
things, but they're closed-source and they phone home, which is a strange thing to
trust with a tool whose whole job is deleting your files.

So Pantheon is the opposite of both: it names the cause in plain English, maps every
finding to a one-command fix, and does all of it 100% locally — no account, no cloud,
no analytics. Because it's open source, that "no telemetry" claim is something you can
verify instead of take on faith.

The part I'm proudest of is the safety model. Cleanups are trash-first and
recoverable, previews match exactly what gets applied, and there's a hardcoded list of
paths — keychains, SSH keys, your home folders — that can never be touched no matter
what flag you pass.

It's still beta and macOS-first. I'd love feedback on the rule set and the safety
design specifically: what waste does it miss on your machine, and what would you never
want a tool like this to go near?

Weigh. Judge. Purge.

→ github.com/SirsiMaster/sirsi-pantheon
