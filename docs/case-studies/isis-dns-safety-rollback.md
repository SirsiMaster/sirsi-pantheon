# Case Study: Isis DNS Safety — Pre-Check, Watchdog, and Auto-Rollback

**Date:** April 5, 2026
**Deity:** Isis (Health & Remediation)
**Severity:** Critical Safety Fix
**Relevance:** Container networking, Kubernetes DNS, public WiFi, restricted networks

---

## The Incident

On April 5, 2026, Pantheon's network security audit (`pantheon isis network --fix`) applied Cloudflare encrypted DNS (1.1.1.1) to a machine connected to in-flight WiFi. The network accepted TCP connections to port 53 but blocked external DNS resolution. The machine lost all internet connectivity. The auto-rollback mechanism failed because it depended on DNS — the very thing that was broken.

The user had to manually revert DNS settings via macOS System Preferences to restore connectivity.

**This is a Rule A1 (Safety First) violation.** A fix that bricks the machine is worse than no fix at all.

---

## Root Cause Analysis

The original implementation had a single verification step:

```
1. Change DNS to Cloudflare (1.1.1.1)
2. Run nslookup api.anthropic.com 1.1.1.1
3. If nslookup fails, revert
```

**Three compounding failures:**

1. **Verify-after-change:** DNS was changed before verifying the target was usable. Once DNS pointed at an unreachable server, the machine couldn't resolve anything — including the verification target.

2. **DNS-dependent probe:** `nslookup` itself requires working DNS infrastructure. When the system's DNS was broken, `nslookup` couldn't function to detect the breakage.

3. **Network deception:** Many restricted networks (captive portals, airline WiFi, corporate firewalls) accept TCP connections on port 53 but return empty responses or block resolution. The port appears "open" but DNS doesn't work.

---

## The Fix: Three-Layer Safety Model

### Layer 1 — Pre-Check Gate (Transport Probe)

Before touching any DNS config, probe the target DNS server with a raw TCP connect:

```go
func dnsReachable(_ platform.Platform, dnsIP string) bool {
    conn, err := net.DialTimeout("tcp", dnsIP+":53", 3*time.Second)
    if err != nil {
        return false
    }
    conn.Close()
    return true
}
```

This uses `net.DialTimeout` — a transport-level TCP SYN that does not depend on DNS resolution. If the port is unreachable, the fix is skipped entirely. **Zero risk to the machine.**

### Layer 2 — Post-Fix Watchdog (Resolution Polling)

Even if the TCP probe passes (port 53 is open), the DNS server may not actually resolve queries. After applying the DNS change, poll actual name resolution:

```go
func verifyDNSOrRollback(p platform.Platform, attempts int, interval time.Duration) bool {
    for i := 0; i < attempts; i++ {
        if dnsResolves() {
            return true
        }
        time.Sleep(interval)
    }
    // DNS never came up — auto-revert to saved prior state
    // [rollback logic]
    return false
}
```

The watchdog makes 3 attempts over ~6 seconds. If resolution never succeeds, it automatically reverts DNS to the saved prior state. The user never has to intervene.

### Layer 3 — Manual Rollback (Escape Hatch)

```bash
pantheon isis network --rollback
```

Restores DNS from `~/.config/pantheon/isis/dns_prior.txt`, which is persisted before every `--fix` operation.

---

## Container and Kubernetes Implications

This same pattern applies directly to container networking:

### Docker Container DNS
Docker containers inherit the host's DNS by default (`/etc/resolv.conf`). If a container management tool changes DNS settings without verification, containers silently lose name resolution. Services fail with opaque "connection refused" or "no such host" errors — not "DNS is broken."

### Kubernetes Pod DNS
Kubernetes uses CoreDNS (or kube-dns) for in-cluster resolution. If a node's upstream DNS is misconfigured:
- Pods can resolve in-cluster services but fail on external domains
- `ExternalName` services break silently
- Init containers that pull from external registries hang indefinitely
- Health checks that depend on external endpoints start failing

### The Pattern: Probe Before, Verify After, Auto-Revert on Failure

Any system that changes network routing or DNS configuration should follow this model:

1. **Probe the target before changing config** — use transport-level checks (TCP connect, ICMP) that don't depend on the service being tested
2. **Verify end-to-end after the change** — actually resolve a hostname, don't just check if a port is open
3. **Auto-revert within seconds if verification fails** — don't leave the system in a broken state waiting for human intervention
4. **Persist prior state** — save the config before changing it so manual rollback is always possible

This is especially critical for:
- **Container orchestrators** changing pod DNS policies
- **Service meshes** modifying iptables rules for traffic routing
- **VPN clients** that modify system DNS on connect/disconnect
- **Infrastructure-as-code** tools applying DNS changes across fleets

---

## Metrics

| Metric | Before (broken) | After (fixed) |
|:-------|:----------------|:--------------|
| Risk of connectivity loss | High — changes DNS before verifying | Zero — probes before touching config |
| Recovery time (auto) | Never — rollback also broken | ~6 seconds (3 polls x 2s) |
| Recovery time (manual) | Minutes — requires System Preferences | Instant — `pantheon isis network --rollback` |
| Networks safely handled | Only open networks | Captive portals, airline WiFi, corporate firewalls |
| Dependencies for probe | DNS (circular) | Raw TCP (independent) |

---

## Files Changed

- `internal/guard/network.go` — Pre-check gate, watchdog polling, TCP probe
- `cmd/pantheon/main.go` — Isis CLI wiring (`pantheon isis network --fix --rollback`)
- `internal/output/tui.go` — TUI intent routing for network security commands

---

*𓁐 Isis heals what is broken — but first, she verifies the cure won't cause harm.*
