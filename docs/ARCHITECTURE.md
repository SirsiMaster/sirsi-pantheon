# Sirsi — Architecture

> Canonical architecture of the Sirsi stack and how Pantheon fits it.
> Local-first. Topology-agnostic. Last set: 2026-06-16 (claude-pantheon).
> Companion: [VISION.md](VISION.md) (the why), [TRACEABILITY.md](TRACEABILITY.md)
> (feature → surface), the deity roster in [DEITY_REGISTRY.md](DEITY_REGISTRY.md).

## The shape

**Sirsi Nexus** is the linchpin — the control plane and command layer of the
whole stack. It ships *later*; when it does, it **commands every service,
surface, and device** beneath it, down to the **Sirsi Sleeve** and **Sirsi Cube**
hardware. Its defining property is **location-transparency**: it unifies the view
across local machines, network machines, servers, data stores, services, remote
databases, proxies, and other engineered devices — so **local / remote / hybrid
is immaterial** to Sirsi.

Three layers:

1. **Frontends** — CLI, Menubar, TUI, GUI, iOS. They are **clients of Nexus**.
   *Today they also run standalone* for users who don't need Nexus (the products
   we ship in 2026).
2. **Sirsi Nexus (the Fabric)** — the command plane. When it launches it
   **subsumes the standalone backend services as modules in the Nexus Fabric**,
   and directly manages the **HyperGraph** and the **I/O-interconnect services**
   (the network fabric + I/O hardware — RDMA over Thunderbolt 5, Sleeve/Cube).
3. **Services / modules** — **Pantheon** (node infra: clean/hydrate/protect
   sessions), **HyperGraph** (semantic knowledge), and the **inference +
   interconnect fabric** (local MLX LLMs over RDMA/TB5). Pre-Nexus these are
   standalone products; post-Nexus they are Fabric modules.

**Pantheon** is a **node-by-node infrastructure surface**. It cleans, hydrates,
and protects sessions for gamers, productivity users, and developers — see
[TRACEABILITY.md](TRACEABILITY.md) for the deity → feature → value → surface map.

## Ship sequence

- **July 2026** — Pantheon on **local machines, Mac first**, as **CLI · MCP ·
  Menubar** (standalone; the Mac menubar ships Developer-ID-signed + notarized,
  Team `9D382WV988`).
- **Later 2026** — **network services**, shipped as part of the **Sirsi Nexus
  release** (Nexus begins subsuming the services as Fabric modules).
- **2027** — **Windows / Linux** frontends + services.

---

## Neith's Triad (Rule A22)

### 1. Data-flow architecture

```
                 ┌─────────────────────────────────────────────┐
   You ─────────▶│  FRONTENDS  CLI · Menubar · TUI · GUI · iOS  │
                 └───────────────┬─────────────────────────────┘
       standalone today          │ clients of Nexus (post-launch)
                 ┌───────────────▼─────────────────────────────┐
                 │  SIRSI NEXUS — the Fabric (command plane)    │
                 │  location-transparent: local/remote/hybrid   │
                 │  manages → HyperGraph · I/O-interconnect      │
                 └───┬───────────────┬───────────────┬─────────┘
                     │ subsumes as modules            │ manages
        ┌────────────▼──────┐ ┌──────▼──────┐ ┌───────▼─────────────┐
        │ PANTHEON          │ │ HYPERGRAPH  │ │ INFERENCE + I/O      │
        │ clean·hydrate·    │ │ semantic    │ │ local MLX LLMs over  │
        │ protect sessions  │ │ knowledge   │ │ RDMA / Thunderbolt-5 │
        │ (node-by-node)    │ │ graph       │ │ (Sleeve / Cube HW)   │
        └─────────┬─────────┘ └─────────────┘ └─────────────────────┘
                  │ acts on
        ┌─────────▼───────────────────────────────────────────────┐
        │ SCOPE (unified, topology-agnostic):                      │
        │ local & network machines · servers · stores · remote DBs │
        │ · proxies · Sleeve + Cube hardware                       │
        └─────────────────────────────────────────────────────────┘
```

Error/fallback paths: a frontend run **without** Nexus falls back to the
standalone service directly (today's default). A service with **no AI backend**
degrades deterministically (e.g. `sirsi insight --no-ai`). Local-first means no
remote dependency is ever on the critical path.

### 2. Recommended implementation order

1. **Standalone Pantheon, Mac (July 2026)** — CLI + MCP + Menubar; the
   minimum-viable surface set. *(Required.)*
2. **Session-protection backlog** — frame-drop/hang detection, swap-thrash
   relief, thread-leak/abnormal-exit forensics (see TRACEABILITY backlog). *(Required for the gamer/productivity promise.)*
3. **Network services (later 2026)** — Pantheon node-to-node + the first Nexus
   Fabric subsumption. *(Required for the Nexus release.)*
4. **Windows / Linux (2027)** — frontend + service parity. *(Required for reach.)*
5. **HyperGraph + I/O-interconnect under Nexus management** — RDMA/TB5 fabric,
   Sleeve/Cube. *(Follows hardware availability.)*

### 3. Key decision points

| Question | Options | Recommendation |
|---|---|---|
| Local-first vs web/cloud-first? | (a) cloud SPA + thin agent; (b) **local-first, surfaces served locally** | **(b)** — the machine is the product; scan/clean/protect need real disk + kernel access; cloud is never on the critical path. *(Rejected (a): a remote SPA can't touch the disk.)* |
| Are frontends clients of Pantheon or of Nexus? | (a) of Pantheon; (b) **of Nexus, standalone-capable today** | **(b)** — build the frontends once against the Nexus contract; they run standalone now and light up Nexus later with no rewrite. |
| How do services join Nexus? | (a) rewrite as Nexus-native; (b) **subsume standalone services as Fabric modules** | **(b)** — nothing built standalone is throwaway; Pantheon/HyperGraph/inference become modules. |
| Where does MLX inference live? | (a) a peer service; (b) **rides the I/O-interconnect Nexus manages** | **(b)** — inference is a consumer of the RDMA/TB5 fabric Nexus owns, not a standalone peer. |
