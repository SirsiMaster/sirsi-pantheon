# 𓂀 Mirror — Semantic File Deduplication & Importance Ranking

> **Status**: Design Document — v0.1  
> **Filed**: March 21, 2026  
> **Module**: `internal/mirror/`  
> **Command**: `anubis mirror`  
> **Theme**: Egyptian copper mirrors — tools of truth that reveal what is real and what is reflection

---

## The Problem

People accumulate thousands of duplicate files — photos, music, documents — across
Downloads, Desktop, iCloud, external drives, and app exports. Existing dedup tools
are dumb: they find exact matches and ask you to pick one. But **which one matters?**

The photo in your Camera Roll that's tagged with faces, GPS, and referenced by 3 albums
is not the same as the WhatsApp-compressed copy sitting in Downloads. They're byte-different
but semantically identical — and the user needs to keep the *right* one.

Nobody solves this well because it requires:
1. Understanding file content (not just hashes)  
2. Understanding file context (where it lives, what references it)  
3. Presenting relationships visually so users can make informed decisions

## The Insight

Apple's Neural Engine (ANE) on M-series Macs and A-series iPhones does
15+ trillion operations per second. CoreML models run on-device, privately,
with zero cloud dependency.

Anubis already has:
- **Brain module** — downloads/manages CoreML + ONNX models from GitHub Releases
- **Classifier interface** — `Classify()` and `ClassifyBatch()` with worker pool
- **Seba graph** — kinetic infrastructure visualization with force-directed layout
- **Ka scanner** — detects ghost app remnants (similar pattern: scan → classify → report)
- **Rule A11** — no telemetry, everything stays on-device

We just need to connect the dots.

---

## Product Tiers

### 🆓 Ankh (Free) — Hash-Based Deduplication

**Target**: Anyone with duplicate files. Zero ML required.

| Feature | Description |
|:--------|:------------|
| **Exact match** | SHA-256 hash comparison across directories |
| **Perceptual hash** | pHash for images — catches resized/recompressed copies |
| **Audio fingerprint** | Chromaprint-style fingerprinting for music files |
| **Size analysis** | Group files by size first (fast pre-filter) |
| **Dry run** | Show what would be cleaned, never auto-delete |
| **Safe list** | Protect directories from dedup (e.g., originals) |

**CLI**:
```
anubis mirror ~/Photos ~/Downloads        # Find duplicates across dirs
anubis mirror --photos ~/Pictures          # Photo-specific scan
anubis mirror --music ~/Music              # Music-specific scan  
anubis mirror --dry-run                    # Preview only
anubis mirror --min-size 1MB               # Skip small files
```

**Output**: Duplicate groups with file paths, sizes, dates, and a recommendation
of which to keep (newest, largest, in protected directory).

### 👁️ Eye of Horus (Pro) — Semantic Importance Ranking

**Target**: Power users, photographers, musicians, content creators.

| Feature | Description |
|:--------|:------------|
| **Face detection** | CoreML Vision — photos with faces rank higher |
| **Scene recognition** | Classify photo content (landscape, portrait, document, screenshot) |
| **Metadata scoring** | GPS, EXIF, album membership, Finder tags, Spotlight comments |
| **Reference tracking** | Which apps/libraries point to this file? |
| **Importance score** | 0.0–1.0 composite score per file |
| **Knowledge graph** | Seba-powered visualization of file relationships |
| **Smart selection** | Auto-select the lowest-importance duplicate for removal |

**CLI**:
```
anubis mirror --rank ~/Photos              # Scan + importance ranking
anubis mirror --graph ~/Photos             # Generate knowledge graph
anubis mirror --clean --confirm            # Remove lowest-ranked duplicates
anubis mirror --protect-faces              # Never suggest deleting photos with faces
```

**ANE/CoreML Models** (downloaded via `anubis install-brain`):
- `mirror-vision-v1.mlmodelc` — face detection + scene classification
- `mirror-audio-v1.mlmodelc` — audio fingerprinting + genre classification
- `mirror-embeddings-v1.onnx` — file embedding model for semantic similarity

---

## Architecture

```
┌─────────────────────────────────────────────────┐
│                anubis mirror                     │
│                                                  │
│  ┌──────────┐  ┌──────────┐  ┌──────────────┐  │
│  │ Scanner  │  │ Hasher   │  │ Classifier   │  │
│  │          │  │          │  │ (Brain)      │  │
│  │ Walk dirs│→ │ SHA-256  │→ │ CoreML/ONNX  │  │
│  │ Filter   │  │ pHash    │  │ Face detect  │  │
│  │ Group    │  │ AudioFP  │  │ Scene class  │  │
│  └──────────┘  └──────────┘  └──────────────┘  │
│       │              │              │            │
│       └──────────────┼──────────────┘            │
│                      ▼                           │
│              ┌──────────────┐                    │
│              │  Ranker      │                    │
│              │  Importance  │                    │
│              │  scoring     │                    │
│              └──────┬───────┘                    │
│                     │                            │
│         ┌───────────┼───────────┐                │
│         ▼           ▼           ▼                │
│    ┌─────────┐ ┌─────────┐ ┌─────────┐          │
│    │ Report  │ │ Graph   │ │ Cleaner │          │
│    │ (CLI)   │ │ (Seba)  │ │ (safe)  │          │
│    └─────────┘ └─────────┘ └─────────┘          │
└─────────────────────────────────────────────────┘
```

### Module: `internal/mirror/`

```
internal/mirror/
├── scanner.go       # Directory walker, file grouping, size pre-filter
├── hasher.go        # SHA-256, perceptual hash (pHash), audio fingerprint
├── ranker.go        # Importance scoring engine
├── dedup.go         # Duplicate group management, selection logic
├── types.go         # Core types: DuplicateGroup, FileEntry, ImportanceScore
└── mirror_test.go   # Tests
```

### Key Types

```go
type FileEntry struct {
    Path         string
    Size         int64
    ModTime      time.Time
    SHA256       string
    PHash        string   // perceptual hash (images)
    AudioFP      string   // audio fingerprint (music)
    Importance   float64  // 0.0-1.0 (pro tier)
    HasFaces     bool     // CoreML face detection (pro)
    SceneType    string   // "landscape", "portrait", "document" (pro)
    IsProtected  bool     // In a safe-list directory
    References   int      // Number of apps/libraries referencing this file
    MediaType    string   // "photo", "music", "video", "document", "other"
}

type DuplicateGroup struct {
    ID           string
    Files        []FileEntry
    MatchType    string   // "exact", "perceptual", "audio", "semantic"
    Recommended  int      // Index of file to keep
    Confidence   float64  // How confident is the recommendation
    TotalWaste   int64    // Bytes recoverable by removing duplicates
}

type MirrorResult struct {
    Groups         []DuplicateGroup
    TotalFiles     int
    TotalDuplicates int
    TotalWasteBytes int64
    ScanDuration   time.Duration
    ModelUsed      string  // "hash-only" or brain model name
}
```

---

## Importance Scoring (Pro)

The importance score is a weighted composite:

| Signal | Weight | Description |
|:-------|:-------|:------------|
| **Has faces** | 0.25 | Photos with detected faces are important |
| **Has GPS/EXIF** | 0.10 | Rich metadata = original source |
| **Album membership** | 0.15 | Referenced by Photos.app albums |
| **File age** | 0.05 | Older = more likely original |
| **File size** | 0.10 | Larger = less compressed = higher quality |
| **Directory depth** | 0.05 | Shallow = intentionally placed |
| **Protected dir** | 0.15 | In user's safe list |
| **Reference count** | 0.10 | Other files/apps point to this |
| **Finder tags** | 0.05 | User has manually tagged this file |

Score = Σ(signal × weight), normalized to [0.0, 1.0]

The file with the **highest importance score** in a duplicate group is the keeper.

---

## Implementation Phases

### Phase 1: Hash Scanner (Ship Now — Free Tier)
- [ ] `internal/mirror/scanner.go` — parallel directory walker
- [ ] `internal/mirror/hasher.go` — SHA-256 + size-based grouping
- [ ] `internal/mirror/types.go` — core types
- [ ] `internal/mirror/dedup.go` — duplicate group management
- [ ] `cmd/anubis/mirror.go` — CLI command
- [ ] Tests

### Phase 2: Perceptual Hashing (Free Tier Enhancement)
- [ ] pHash implementation for images (DCT-based)
- [ ] Hamming distance threshold for "similar enough"
- [ ] Audio fingerprinting (Chromaprint-compatible)
- [ ] `--photos` and `--music` flags

### Phase 3: Importance Ranking (Pro Tier)
- [ ] `internal/mirror/ranker.go` — scoring engine
- [ ] EXIF/metadata extraction
- [ ] Directory protection / safe lists
- [ ] Reference counting (Spotlight metadata)
- [ ] `--rank` flag

### Phase 4: Neural Classification (Pro Tier — ANE)
- [ ] CoreML face detection model
- [ ] Scene classification model
- [ ] Integration with Brain module download pipeline
- [ ] `--protect-faces` flag

### Phase 5: Knowledge Graph (Pro Tier — Seba)
- [ ] File relationship graph generation
- [ ] "Why are these duplicates?" edge labels
- [ ] Importance visualization (node size = importance)
- [ ] Interactive deletion from graph UI
- [ ] `--graph` flag

---

## Competitive Landscape

| Tool | Exact Dedup | Perceptual | Importance | On-Device ML | Knowledge Graph |
|:-----|:----------:|:----------:|:----------:|:------------:|:---------------:|
| **fdupes** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **rdfind** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Gemini 2** | ✅ | ✅ | ❌ | ❌ | ❌ |
| **dupeGuru** | ✅ | ✅ | ❌ | ❌ | ❌ |
| **CleanMyMac** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Anubis Mirror** | ✅ | ✅ | ✅ | ✅ (ANE) | ✅ (Seba) |

**The moat**: Nobody else combines deduplication + on-device neural importance ranking +
knowledge graph visualization. And it's open source (free tier) with a premium upgrade path.

---

## Revenue Model

- **Free**: Hash dedup, perceptual hashing, CLI reports — open source, forever free
- **Pro ($9/mo or $79/yr)**: ANE importance ranking, face protection, knowledge graph, priority support
- **Enterprise**: Fleet dedup across teams, policy enforcement via Scales
