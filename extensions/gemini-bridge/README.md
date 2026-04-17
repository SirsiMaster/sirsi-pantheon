# 𓁆 Seshat — Gemini Bridge Extension

**Bidirectional knowledge sync between Gemini AI Mode, NotebookLM, and Antigravity IDE.**

Part of the [Sirsi Pantheon](https://github.com/SirsiMaster/sirsi-pantheon) ecosystem.

## Features

- **Knowledge Items Browser** — Browse all Antigravity Knowledge Items with artifact drill-down
- **Chrome Profile Discovery** — Detects all Chrome profiles for Gemini extraction
- **Six-Direction Sync Status** — Visual status for all bridge directions
- **Dashboard Panel** — Full webview with KI inventory, conversation count, and sync actions
- **GEMINI.md Sync** — Push Knowledge Items into workspace GEMINI.md context
- **Export to Markdown** — Export KIs as NotebookLM-ready Markdown sources

## Commands

| Command | Description |
|---------|-------------|
| `𓁆 Seshat: List Knowledge Items` | Browse and open Knowledge Items |
| `𓁆 Seshat: Export Knowledge Item` | Export a KI as NotebookLM Markdown |
| `𓁆 Seshat: Sync KI → GEMINI.md` | Inject KI context into workspace GEMINI.md |
| `𓁆 Seshat: List Chrome Profiles` | Discover and set default Chrome profile |
| `𓁆 Seshat: List Brain Conversations` | View recent Antigravity conversations |
| `𓁆 Seshat: Open Bridge Dashboard` | Open the full bridge dashboard |

## Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| `seshat.defaultProfile` | `SirsiMaster` | Default Chrome profile for extraction |
| `seshat.autoSync` | `false` | Auto-sync KIs to GEMINI.md on startup |
| `seshat.pantheonBinaryPath` | `sirsi` | Path to Pantheon CLI binary |
| `seshat.antigravityDir` | `~/.gemini/antigravity` | Override Antigravity data path |

## Installation

### From OpenVSX
```bash
# Via VS Code / Antigravity IDE
ext install SirsiMaster.seshat-gemini-bridge
```

### From VSIX
```bash
code --install-extension seshat-gemini-bridge-0.1.0.vsix
```

## Development

```bash
cd extensions/gemini-bridge
npm install
npm run compile
npm run package
```

## License

MPL-2.0 — See [LICENSE](../../LICENSE)
