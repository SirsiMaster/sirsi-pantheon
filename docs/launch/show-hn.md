# Show HN

Launch copy for a Show HN post. HN dislikes marketing; keep it plain, technical,
and honest. Every claim below is verifiable in this repo — do not add metrics that
aren't.

---

## Title options

Pick one. HN titles should be plain and specific, not a slogan.

1. **Show HN: Pantheon – a local-only macOS cleaner that names the cause and fixes it**
2. **Show HN: Pantheon – find what's slowing your Mac, in plain English, 100% local**
3. **Show HN: Pantheon – an open-source Mac hygiene tool (81 rules, trash-first, no telemetry)**

(1) is the recommended lead — it states the differentiator without hype.

---

## Body

Hi HN — I've been building Pantheon, an open-source (Apache-2.0) hygiene tool for
macOS. One line: **it finds what's dragging your Mac down, names the cause in plain
English, and fixes it — trash-first, with before/after proof.**

Most "Mac cleaners" are closed-source and phone home, and most system tools
(Activity Monitor, btop) only *watch* — they show you a red number and stop. Pantheon
tries to close the loop: every finding maps to a fix you can run in one command, and
the preview you approve is exactly what gets applied.

What it does today, all from the CLI:

- `sirsi scan` — 81 rules sweep for infrastructure waste: package-manager caches,
  build artifacts (node_modules, Go module cache, target/), stale dev cruft, cloud
  CLI caches. Each finding comes with an evidence count and a plain-English note
  ("Reinstalls with npm install (~30s–2min)").
- `sirsi clean` — previews what's safe to remove. It's a dry-run by default; the
  amount previewed is exactly the amount `--confirm` moves to the Trash. Nothing gets
  `rm -rf`'d behind your back.
- `sirsi ghosts` — finds leftovers of apps you've already uninstalled.
- `sirsi diagnose` — RAM pressure, disk, top consumers, kernel panics, in one brief.
- `sirsi relieve` — lowers the priority of the top CPU hog. Reversible; nothing gets
  killed.

Why local-only matters, and why it's a design rule not a toggle: a tool that deletes
files for a living should not also be a tool that ships telemetry. There is no
account, no cloud, no analytics — nothing leaves your machine. You can verify that
claim because the source is open.

The safety design is the actual product. Cleanups are trash-first (recoverable, with
a decision log). There are 25 hardcoded protected-path rules — system directories,
keychains, SSH keys, and your home folders (Desktop, Documents, Downloads) can never
be deleted, symlink-escape-proof, not overridable by any flag or config. That's
enforced in `internal/cleaner/safety.go` if you want to read it.

Same Go core drives the CLI (the primary surface), a macOS menu-bar app, a local
browser dashboard (`sirsi dashboard`, served from your machine to your browser, no
server), and an MCP server so an AI IDE can ask "what's eating this machine?" and act
on the answer.

It's macOS-first (Apple Silicon + Intel) and still beta (v0.23.8). Windows/Linux are
deliberately deferred rather than half-done.

Install:

    brew tap SirsiMaster/tools && brew install sirsi-pantheon
    sirsi scan

Or `go build ./cmd/sirsi/` from the repo.

Repo: https://github.com/SirsiMaster/sirsi-pantheon

Honest about the state: it's beta, the interactive TUI isn't shipped yet (it's in
design, deliberately gated on a quality bar), and I'd genuinely like feedback on the
safety model and the rule set. What waste does it miss on your machine? What would
you never want a tool like this to touch?
