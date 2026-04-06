# Seshat — Knowledge Bridge

Seshat ingests knowledge from multiple AI platforms and tools, reconciles it, and exports to multiple targets. Universal knowledge grafting.

## Commands

### Ingest knowledge
```bash
pantheon seshat ingest                           # All sources
pantheon seshat ingest --source chrome-history   # Chrome browsing history
pantheon seshat ingest --profile SirsiMaster     # Specific Chrome profile
pantheon seshat ingest --all-profiles            # All Chrome profiles
pantheon seshat ingest --since "7 days ago"      # Recent items only
```

Sources: Chrome history, Gemini conversations, Claude sessions, Apple Notes, Google Workspace.

### Export knowledge
```bash
pantheon seshat export notebooklm               # Push to Google NotebookLM
pantheon seshat export thoth                     # Push to Thoth memory
pantheon seshat notebooklm                       # Export + open browser
pantheon seshat notebooklm --profile SirsiMaster # Specific profile
```

### List knowledge items
```bash
pantheon seshat list             # Show all ingested items with sources
```

### Manage adapters and profiles
```bash
pantheon seshat adapters         # List available source/target adapters
pantheon seshat profiles chrome  # List Chrome profiles
pantheon seshat open chrome --profile SirsiMaster  # Launch Chrome profile
```

### Authentication
```bash
pantheon seshat auth google      # Authenticate with Google Workspace
```

### MCP Context Server
```bash
pantheon seshat mcp              # Start Seshat's MCP context server
```

Exposes knowledge items as context for AI coding tools.

## How It Works

1. **Ingest**: Seshat reads from configured sources (browser history, AI chats, notes)
2. **Reconcile**: Deduplicates and normalizes knowledge items
3. **Store**: Saves to local knowledge base (no external transmission)
4. **Export**: Pushes to targets on demand (NotebookLM, Thoth, etc.)

All data stays local. Zero telemetry. Secrets are filtered before storage.
