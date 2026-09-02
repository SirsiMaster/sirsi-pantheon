# Sirsi Hardware Baseline Alignment

**Owner:** Sirsi Hardware Admin  
**Status:** canonical integration contract; implementation gates remain explicit  
**Date:** 2026-08-25

This document is the cross-repository hardware contract for Mac Studio,
Sirsi Sleeve, Pantheon, SNE, and Sirsi Nexus. It prevents a vendor line-rate
claim from becoming an inference or product claim without host-specific
measurement.

## Authority and repository boundaries

| Concern | Authority | Consumers | Current state |
|---|---|---|---|
| Host identity, power, thermal, memory, swap, lock/lid, listeners | Pantheon Hardware Admin | SNE, Nexus, operators | Existing read-only readiness receipts; extend with fabric fields |
| Sleeve identity, port map, negotiated rate, errors, firmware | Pantheon Hardware Admin | Nexus transport planner, SNE admission | Contract defined here; telemetry implementation remains required |
| Intent, placement request, user policy | Sirsi Nexus | Pantheon | Architecture exists; transport intent must bind to a hardware receipt |
| Model/runtime/tokenizer identity and correctness | SNE | Pantheon admission, Nexus | Existing exact identity and fail-closed qualification contracts |
| Distributed partitioning and collectives | SNE | Sleeve transport capability | Two-node JACCL design exists; physical qualification remains open |
| Physical enclosure, per-port TB5-to-optical link engines, rollback slots | Sleeve firmware/electronics | Pantheon | Research/design phase; design in SirsiNexusApp Notebook 1 (hardware) and Notebook 4 §10 (software), 2026-09-02 |

Pantheon is the sole hardware admission authority. Nexus requests a capability;
SNE consumes an admitted capability. Neither may infer a TB5 mode from a port
count or from an Apple marketing maximum.

## Canonical hardware facts

- M5 Ultra Mac Studio: six TB5-capable ports, 1.2 TB/s unified-memory bandwidth,
  up to 512 GB unified memory, and 10GbE.
- M5 Max Mac Studio: four rear TB5 ports; the two front USB-C ports are not
  additional TB5 ports.
- M6 Mac mini: architecture-control platform only for this program; three TB4
  ports, up to 170 GB/s memory bandwidth, and optional 10GbE.
- TB5 normal data model: 80 Gb/s bidirectional per link.
- TB5 Boost hypothesis: up to 120 Gb/s one direction and 40 Gb/s the other;
  whether macOS exposes that allocation to data/RDMA traffic must be measured.

Primary sources: [Apple Mac Studio specifications](https://www.apple.com/mac-studio/specs/),
[Apple M6 Mac mini announcement](https://www.apple.com/newsroom/2026/08/apple-unveils-a-more-powerful-mac-mini-featuring-the-all-new-m6-and-m5-pro/),
and [Intel's Thunderbolt 5 technical brief](https://www.intel.com/content/dam/www/central-libraries/us/en/documents/2023-09/thunderbolt-5-technology-brief.pdf).

## Sleeve capacity model

**Alignment note (2026-09-02, owner decisions in SirsiNexusApp Notebooks 1 and 4):** all six TB5 ports are I/O; the Sleeve converts each port to fiber protocol-transparently and bonds nothing; every link is a native Thunderbolt host-to-host link, so the per-link directional model below is the whole model — there is no aggregate fabric port, no NIC and no Ethernet data plane. Direction is a scheduled resource per link, recorded as requested / negotiated / observed; only observed is ever rendered as fact. Whether a link survives an optical engine with RDMA and Boost intact is unestablished (SSA reading review, 2026-09-02) and is Notebook 4 Phase 0b. First receipt fields ship as `sirsi hardware links` (PR #696).

The model uses per-link directional capacity, not a single symmetric aggregate:

```text
normal:  link tx=80 Gb/s,  rx=80 Gb/s
boost:   link tx=120 Gb/s, rx=40 Gb/s   (unqualified data hypothesis)
ethernet: independent side channel, 10 Gb/s maximum
```

For six M5 Ultra links, the normal physical sum is 60 GB/s in each direction.
If Boost is proven usable for data, six links have a 90/30 GB/s directional
envelope, with dynamic intermediate states and 60/60 GB/s at a balanced 3/3
allocation. Adding 10GbE contributes at most 1.25 GB/s to a direction only
when bonding and routing are independently demonstrated.

These are line-rate ceilings. They are not RDMA goodput, memory bandwidth, model
weight throughput, or SNE tok/s.

## Shared transport capability receipt

Pantheon must publish a machine-readable capability before SNE or Nexus can use
the fabric:

```json
{
  "schema": "sirsi.hardware.transport-capability.v1",
  "host_id": "...",
  "peer_id": "...",
  "sleeve_id": "...",
  "os_build": "...",
  "firmware_sha256": "...",
  "links": [
    {"stable_id":"...","port_a":"...","port_b":"...",
     "transport":"tb5","tx_gbps":80,"rx_gbps":80,
     "state":"observed"}
  ],
  "ethernet": {"present":true,"rate_gbps":10,"bonded":false},
  "rdma": {"present":false,"backend":"none","verified":false},
  "latency_us": {"p50":null,"p99":null},
  "goodput_gbps": {"tx":null,"rx":null},
  "host_admission": "...",
  "observer_provenance": "direct-read-only",
  "decision": "deferred"
}
```

`advertised`, `negotiated`, `observed`, and `qualified` are distinct states.
Unknown or observer-denied fields remain unknown. Positive SSH/5900/Tailscale
transport proves host availability, but does not prove Aqua/GPU eligibility or
TB5 data capability.

## Integration contracts

### Pantheon

Pantheon discovers hardware, validates power/thermal/memory/swap/process
isolation, records per-port transport evidence, and issues an admission receipt.
It may observe and recommend a repair, but must not create a second transport
plane, silently change firmware, or restart protected SNE/Tailscale services.

### Nexus

Nexus submits a signed transport intent containing peer set, directionality,
minimum goodput, latency budget, and failure policy. Pantheon resolves that
intent into a receipt-bound plan. Nexus never receives ambient sudo, TCC,
Keychain, or firmware-control privileges.

### SNE

SNE receives the receipt and decides whether its workload can use local,
pipeline, or collective execution. It must bind model/runtime/package identity
and topology epoch to the receipt. It owns correctness and performance evidence;
it does not claim physical link rate from tok/s.

### Exo adapter

Exo is useful as a bounded research adapter, not as the Sirsi control plane.
Its public project already provides automatic discovery, topology-aware
placement, MLX distributed communication, tensor/pipeline placement choices,
and benchmark tooling. It should be used to accelerate comparative experiments
and to import proven placement heuristics—not embedded as an authority that can
install network profiles, bypass Pantheon admission, or launch model work
without a receipt. See the [Exo project](https://github.com/exo-explore/exo).

Sirsi should derive an `exo-sirsi` adapter rather than fork Exo's whole runtime:

1. Pantheon supplies the signed node and transport capability manifest.
2. Nexus supplies intent and an immutable topology epoch.
3. The adapter translates the manifest into Exo placement inputs.
4. Exo proposes a placement or executes an isolated experiment.
5. SNE validates exact model/runtime identity and correctness.
6. Pantheon records the result and owns lifecycle, rollback, and cleanup.

The adapter must disable Exo's ambient discovery and network-profile mutation in
governed deployments. Exo's current JACCL issue history also demonstrates why
placement success and bare MLX/JACCL success must be recorded separately: a
framework can fail to form its expected topology while the underlying MLX
distributed path works.

## Qualification gates

1. **Two M5 Ultras:** enumerate all six links, establish normal 80/80 behavior,
   measure per-link and aggregate goodput, latency, CPU/copy cost, and failure
   recovery. No model workload.
2. **Three M5 Ultras:** exercise dynamic 6/0 through 0/6 directional demand,
   rebalance hysteresis, contention, and one-link failure.
3. **Four M5 Ultras:** run Sleeve qualification with weight, activation, and
   bidirectional synchronization traffic, including 10GbE side-channel load.
4. **Six to eight M5 Ultras:** only after the four-node stage proves useful
   application goodput and stable recovery.

Every gate records exact host/Sleeve/firmware identity, cable map, requested and
observed allocation, transport, latency, goodput, thermal/power/memory/swap
state, errors/retries, process isolation, cleanup, and rollback status.

## Cross-repository alignment map

| Repository | Required alignment | Current action |
|---|---|---|
| `sirsi-pantheon` | Own capability schema, admission, drift, rollback | This document; implementation next |
| `sirsi-inference` | Consume capability; preserve two-node JACCL design; no custom transport before measured blocker | Link its design to this contract |
| `sirsi-native-rebuild` | Bind SNE qualification to host/fabric receipt; keep model math independent | Add receipt input to future multi-node gate |
| `SirsiNexusApp` | Keep Sleeve implementation and integration notebook aligned | Notebook4 is the hardware implementation blueprint |
| `sne` | No physical or broad-performance claim from line rate | Existing claims remain workload-bound |

## Non-negotiable claim rules

- 60 GB/s symmetric is the six-link normal line-rate sum, not usable application
  throughput.
- 90 GB/s is a one-direction six-link Boost ceiling only if data Boost is proven;
  it is not 90 GB/s symmetric.
- M5 Max four-port and M5 Ultra six-port claims must never be conflated.
- M6 mini results are architecture context, not TB5 Sleeve evidence.
- SNE's measured tok/s, memory-bandwidth proxies, and transport goodput remain
  separate evidence classes.
- A failed observer or missing field produces `deferred`/`unknown`, never a
  fabricated ready state.
