# GEMINI.md
**Operational Directive for All Development Agents (sirsi-thoth)**
**Version:** 2.0.0 (Pantheon Unified)
**Date:** March 23, 2026

---

## 0. Identity
This is the **sirsi-thoth** repository — the Thoth persistent knowledge system for AI-assisted development.
A standalone CLI tool (`npx thoth-init`) that gives AI coding assistants structured memory across sessions.

- **GitHub**: `https://github.com/SirsiMaster/sirsi-thoth`
- **Local Path**: `/Users/thekryptodragon/Development/sirsi-thoth`

**This repo is NOT sirsi-anubis. This repo is NOT SirsiNexusApp.**
Thoth is a standalone tool — it works in any project without Anubis or MCP.

---

## 1. Universal Rules (Apply to ALL Sirsi Portfolio Repos)

> These rules are inherited from the Sirsi Portfolio Standard v2.0.0.
> See `docs/SIRSI_PORTFOLIO_STANDARD.md` for the full standard.

0.  **Minimal Code** (Rule 0): Write the smallest amount of clean, correct code. DELETE AND REWRITE over band-aids.
1.  **Challenge, Don't Just Please**: Push back on suboptimal requests.
2.  **Critical Analysis First**: Analyze Architecture, Security, and Business impact before writing code.
3.  **Solve the "How"**: You own implementation details.
4.  **Agentic Ownership**: Plan -> Build -> Verify -> Document.
5.  **Sirsi First** (Rule 1): Check if it exists in the ecosystem first.
6.  **Implement, Don't Instruct** (Rule 2): Build working code, not instructions.
7.  **Test Before Declaring Done** (Rule 3): Verify `node index.js --help` works.
8.  **Follow the Pipeline** (Rule 4): Local -> GitHub -> Production.
9.  **Always Push & Verify** (Rule 5): Push and verify immediately.
10. **ADRs are Mandatory** (Rule 8): Document significant decisions.
11. **Do No Harm** (Rule 14): Never break working functionality.
12. **Additive-Only Changes** (Rule 15): Don't recode working modules.
13. **Mandatory Canon Review** (Rule 16): Read this file and SPECIFICATION.md first.
14. **Sprint Planning is Mandatory** (Rule 17): Plan before coding.
15. **Living Canon** (Rule 18): Codify new rules immediately.
16. **Identity Integrity** (Rule 19): `SirsiMaster` account only.
17. **Commit Traceability** (Rule A7): Every commit references canon.
18. **Feature Documentation** (Rule A8): User docs + developer README.
19. **Release Versioning** (Rule A13): SemVer. VERSION file + CHANGELOG.
20. **Statistics Integrity** (Rule A14): Verifiable numbers only.
21. **Session Definition** (Rule A15): session = one AI conversation.
22. **Voice Rule**: Direct verbs. Never "the user wanted."
23. **𓁟 Thoth is Mandatory**: Read `.thoth/memory.yaml` FIRST.
24. **🪶 Ma'at Governs Quality**: Every feature links to canon.
25. **Context Monitoring** (Rule A9): Report context health after every sprint.

## 2. Thoth-Specific Rules

*   **Simplicity is the Product**: Thoth is a single JS file + templates. No framework, no build step, no runtime dependencies. If it gets complex, it has failed.
*   **Works Everywhere**: Thoth must work in any project — Go, TypeScript, Python, Rust, or polyglot. No language assumptions.
*   **Non-Interactive by Default**: `npx thoth-init --yes` must work without prompts.
*   **IDE Rules Injection**: When Thoth modifies GEMINI.md/CLAUDE.md, it adds to existing content — never overwrites.

## 3. Canonical Documents

1. `GEMINI.md` (this file)
2. `SPECIFICATION.md` — Full Thoth specification
3. `docs/SIRSI_PORTFOLIO_STANDARD.md`
4. `.thoth/memory.yaml`
5. `.thoth/journal.md`
