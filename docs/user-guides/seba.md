# Seba — Infrastructure & Hardware

Seba maps your infrastructure: hardware capabilities, architecture topology, fleet discovery, and project registry. Absorbs the former Hapi (hardware) and Khepri (fleet) modules.

## Commands

### Hardware summary
```bash
sirsi seba hardware           # Dashboard: CPU, GPU, RAM, ANE, accelerators
sirsi seba hardware --json    # Full hardware profile as JSON
```

Detects: CPU model/cores, total RAM, GPU (name + Metal family), Neural Engine, CUDA, ROCm, accelerator routing.

### Deep system profile
```bash
sirsi seba profile            # Save full profile to ~/.config/pantheon/profile.json
sirsi seba profile --json     # Also print to stdout
```

Writes hardware + accelerator data as JSON for consumption by other tools.

### ML tokenization
```bash
sirsi seba compute --tokenize "Hello world"   # Tokenize via ANE or CPU
sirsi seba compute --tokenize "text" --json    # JSON with latency measurement
```

Routes to the fastest available accelerator (ANE > Metal > CPU).

### Architecture mapping
```bash
sirsi seba scan               # Build infrastructure graph
sirsi seba scan --format html # Render as interactive HTML
sirsi seba scan --json        # Export graph as JSON
```

Populates a graph with hardware nodes, running containers, and system topology.

### Architectural diagrams
```bash
sirsi seba diagram --type hierarchy         # Deity hierarchy
sirsi seba diagram --type dataflow          # CLI data flow
sirsi seba diagram --type modules           # Go import graph
sirsi seba diagram --type all               # All diagrams
sirsi seba diagram --type all --html        # Self-contained HTML
```

Generates Mermaid diagrams. With `--html`, produces a standalone page with rendered diagrams.

### Project registry
```bash
sirsi seba book               # List git repos in current directory
```

Enumerates git repositories and shows the last commit for each.

### Fleet discovery
```bash
sirsi seba fleet                          # Discover network hosts
sirsi seba fleet --containers             # Docker-only audit
sirsi seba fleet --confirm-network        # Active network scan (requires opt-in)
sirsi seba fleet --subnet 192.168.1.0/24  # Scan specific subnet
```

Network scanning requires explicit `--confirm-network` flag. Docker audit works without it.

## Platform Support

| Feature | macOS | Linux | Windows |
|---------|-------|-------|---------|
| Hardware detection | Full (ANE, Metal) | Full (CUDA, ROCm) | Partial |
| Architecture mapping | Full | Full | Partial |
| Fleet discovery | Full | Full | Not supported |
| Diagrams | Full | Full | Full |
