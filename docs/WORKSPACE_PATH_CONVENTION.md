# Workspace-Root-Relative Path Convention

**Status:** Accepted — 2026-08-05  
**Scope:** All Sirsi portfolio repositories  
**Custodian:** claude-pantheon (portfolio infrastructure)

---

## The Convention

The Sirsi portfolio manifesto (`SIRSI_MANIFESTO.md`) lives **one level above every repo** at
`~/Development/SIRSI_MANIFESTO.md`. It is deliberately not vendored per-repo to avoid
N-way fork drift.

### In reading instructions (AGENTS.md, CLAUDE.md, GEMINI.md, etc.)

Use the workspace-root-relative form: `$HOME/Development/SIRSI_MANIFESTO.md`

This is correct for owner-local sessions (the only context where AI agents read this file).
Absolute paths like `/Users/thekryptodragon/Development/SIRSI_MANIFESTO.md` are equivalent
but machine-specific; prefer the `$HOME`-relative form in all new instructions.

### In gate scripts (pre-push hooks, CI checks, canon enforcement)

If a gate script ever reads a `canon_documents` list or checks manifesto existence, it MUST
resolve workspace paths via:

```sh
DEVELOPMENT_ROOT="${DEVELOPMENT_ROOT:-$HOME/Development}"
MANIFESTO="$DEVELOPMENT_ROOT/SIRSI_MANIFESTO.md"
```

Gate scripts MUST fail **open** (skip the check, not fail the build) when the manifesto is
absent — it will not be present on CI runners, fresh clones, or contributor machines. The
manifesto is an owner-context reading guide, not a CI artifact.

### In completion.contract.json `canon_documents`

Entries should use the portable form: `${DEVELOPMENT_ROOT:-$HOME/Development}/SIRSI_MANIFESTO.md`

Gate scripts that enforce this field MUST apply the resolution rule above and fail open when
the file is not found.

---

## Decision Log

**Option considered and rejected — vendor a per-repo symlink or pointer doc:**  
Would create 9+ files to maintain, all pointing at the same upstream. Every repo change
requires a coordinated sweep. Rejected: maintenance cost > benefit.

**Option considered and rejected — CI artifact / git submodule:**  
The manifesto is owner-context strategy, not a build dependency. Adding it to CI increases
complexity with no runtime benefit. Rejected: wrong layer.

**Decision — document the env-var resolution convention, fail open:**  
Zero files to maintain. Gate scripts that don't exist today gain a clear authoring rule. The
latent risk (gate fails on second machine) is defused by the fail-open requirement. Cost: one
doc per portfolio (this file), mirrored in AGENTS.md reading instructions.

---

## Portfolio Coverage

All repos using this convention should carry a pointer to this document in their AGENTS.md
manifesto section. The canonical pattern is in `sirsi-pantheon/docs/WORKSPACE_PATH_CONVENTION.md`.

Repos confirmed using the `$HOME/Development/SIRSI_MANIFESTO.md` pattern (as of 2026-08-05):
sirsi-pantheon, SirsiNexusApp, FinalWishes, assiduous, porch-and-alley, sirsi-hypergraph,
sirsi-inference, sirsi-io, homebrew-tools, assiduous-scaffold.
