# Sirsi macOS Permission Implementation

Pantheon now implements the portfolio-wide permission-silence contract at the
shared operational boundary. Resident Go and Swift surfaces no longer touch
protected stores to infer Full Disk Access, no longer register themselves with
TCC at launch, and no longer request notification authorization automatically.

The rule applies to every Sirsi product, not only Desktop access. The canonical
contract is `/Users/thekryptodragon/Development/SIRSI_MACOS_PERMISSION_CONTRACT.md`.
The executable portfolio gate is
`/Users/thekryptodragon/Development/tools/check_sirsi_macos_permission_contract.sh`.

Current product disposition:

- Pantheon is the signed human authorization broker.
- Pantheon CLI, TUI, MCP, launch agents, and health loops never prompt.
- SNE and Sirsi Inference operate only on app-owned model/runtime locations.
- Nexus web permissions remain browser-owned and user-gesture initiated; a future
  native Nexus client must consume the same broker contract.
- Hypergraph records authorization evidence but never obtains permission itself.
- Sirsi IO Connect requests device/network access only inside its explicit setup.

Release proof requires a stable Developer ID identity and a clean-account launch
with no unsolicited TCC request. Ad-hoc builds remain development-only and may
not replace an installed production application.
