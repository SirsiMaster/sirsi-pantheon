# Seba — Infrastructure & Hardware

Seba maps your infrastructure: hardware capabilities, architecture topology, fleet discovery, and project registry. Absorbs the former Hapi (hardware) and Khepri (fleet) modules.

## Commands

### Hardware summary
```bash
pantheon seba hardware           # Dashboard: CPU, GPU, RAM, ANE, accelerators
pantheon seba hardware --json    # Full hardware profile as JSON
```

Detects: CPU model/cores, total RAM, GPU (name + Metal family), Neural Engine, CUDA, ROCm, accelerator routing.

### Deep system profile
```bash
pantheon seba profile            # Save full profile to ~/.config/pantheon/profile.json
pantheon seba profile --json     # Also print to stdout
```

Writes hardware + accelerator data as JSON for consumption by other tools.

### ML tokenization
```bash
pantheon seba compute --tokenize "Hello world"   # Tokenize via ANE or CPU
pantheon seba compute --tokenize "text" --json    # JSON with latency measurement
```

Routes to the fastest available accelerator (ANE > Metal > CPU).

### Architecture mapping
```bash
pantheon seba scan               # Build infrastructure graph
pantheon seba scan --format html # Render as interactive HTML
pantheon seba scan --json        # Export graph as JSON
```

Populates a graph with hardware nodes, running containers, and system topology.

### Architectural diagrams
```bash
pantheon seba diagram --type hierarchy         # Deity hierarchy
pantheon seba diagram --type dataflow          # CLI data flow
pantheon seba diagram --type modules           # Go import graph
pantheon seba diagram --type all               # All diagrams
pantheon seba diagram --type all --html        # Self-contained HTML
```

Generates Mermaid diagrams. With `--html`, produces a standalone page with rendered diagrams.

### Project registry
```bash
pantheon seba book               # List git repos in current directory
```

Enumerates git repositories and shows the last commit for each.

### Fleet discovery
```bash
pantheon seba fleet                          # Discover network hosts
pantheon seba fleet --containers             # Docker-only audit
pantheon seba fleet --confirm-network        # Active network scan (requires opt-in)
pantheon seba fleet --subnet 192.168.1.0/24  # Scan specific subnet
```

Network scanning requires explicit `--confirm-network` flag. Docker audit works without it.

## Platform Support

| Feature | macOS | Linux | Windows |
|---------|-------|-------|---------|
| Hardware detection | Full (ANE, Metal) | Full (CUDA, ROCm) | Partial |
| Architecture mapping | Full | Full | Partial |
| Fleet discovery | Full | Full | Not supported |
| Diagrams | Full | Full | Full |
