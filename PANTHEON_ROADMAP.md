# Pantheon Hardening Roadmap v4.0.0
## ⚖️ Ma'at Ground Truth — March 30, 2026

> **Measured by Ma'at Pulse. Not aspirational. Not stale.**

---

## 📊 Live Metrics (Session 38 — Ma'at Audit)

| Metric | Value | Source |
|:---|:---|:---|
| **Test Count** | **1,450+** | `go test -v ./... \| grep -c "=== RUN"` |
| **Packages Passing** | **28/28** | `go test ./...` |
| **Weighted Coverage** | **~86.2%** | `go test -cover ./...` |
| **Binary Size** | 11.4 MB | `ls -lh pantheon` |
| **Internal Modules** | 27 | `ls internal/` |
| **Version** | v1.0.0-rc1 | `git tag` ✅ |
| **Total Commits** | 230+ | `git rev-list --count main` |

---

## 📈 Coverage by Package (Live Measured)

### 🟢 ≥ 85% (Production-Ready)
| Package | Coverage | Status |
|:---|:---|:---|
| scales | **99.2%** | ✅ |
| logging | 95.2% | ✅ |
| jackal | 94.6% | ✅ |
| osiris | 92.8% | ✅ |
| ka | 92.6% | ✅ |
| ignore | 91.8% | ✅ |
| brain | 90.0% | ✅ |
| horus | 89.5% | ✅ |
| seba | 87.8% | ✅ |
| guard | 87.8% | ✅ |
| output | **87.5%** | ✅ |
| updater | 87.7% | ✅ |
| cleaner | 85.7% | ✅ |
| hapi | **85.5%** | ✅ |
| thoth | 85.4% | ✅ |
| profile | 85.1% | ✅ |
| seshat | **84.9%** | ✅ |

### 🟡 70–84% (Approaching Target)
| Package | Coverage | Action |
|:---|:---|:---|
| neith | **100.0%** | Exceeds target |
| scarab | 94.8% | Near target |
| yield | 83.9% | Near target |
| stealth | 82.6% | Near target |
| sight | 81.6% | Near target |
| maat | 79.4% | Isis self-heal needed |

### 🔴 < 70% (Requires Remediation)
| Package | Coverage | Action |
|:---|:---|:---|
| mirror | **80.0%** | Server/scanner unit tests (up from 66%) |
| jackal/rules | **64.5%** | Rule execution paths (up from 35%) |
| platform | **66.5%** | Darwin/Singleton/Detect (up from 62%) |

---

## 🛤 Phase 1: Interface Injection (COMPLETE ✅)
- [x] **Platform Interface Expansion**: `MoveToTrash`, `PickFolder`, `OpenBrowser`, `ReadDir`.
- [x] **Mirror Module**: Refactored `Server` to use injected `Platform`. (65.9%)
- [x] **Sight Module**: Stabilized `parseLSRegisterDump` and `Fix` logic. (81.6%)
- [x] **Rule A16 Established**: Interface injection standard project-wide.

## 🛤 Phase 2: Deity Hardening (ACTIVE 🚧)
- [x] **Neith**: 0% → 100% coverage — Tests added Session 38.
- [x] **Thoth**: 0% → 85.4% coverage — Tests added Session 38.
- [x] **Seshat**: 2.1% → TBD — Sprint tests added.
- [ ] **Hapi**: 78.2% — Target 85%+ (accelerator mocking needed).
- [ ] **Ma'at**: 79.4% → Target 85% (heal thyself).
- [ ] **Isis**: 71.0% → Target 85%.

## 🛤 Phase 3: Production Hardening (NEXT)
- [ ] Run full Ma'at assessment on all 28 modules.
- [ ] Achieve **Feather Weight 85+** across the entire ecosystem.
- [ ] Optimize `mcp` test duration (currently dominates suite).
- [ ] Add `testing.Short()` guards to slow integration tests.

## 🛤 Phase 4: Cross-Platform Truth
- [ ] **Windows**: Implement `internal/platform/windows.go`.
- [ ] **Linux CI**: Add `ubuntu-latest` runner verification.
- [ ] **macOS ARM**: Verify on `macos-14` (Apple Silicon).
- [ ] `go test -race ./...` passes on all platforms.

---

## 📋 Verification Checklist
- [x] `go test ./...` all 28 packages pass.
- [x] `v1.0.0-rc1` git tag exists.
- [ ] `go test -race ./...` passes on macOS, Linux, and Windows.
- [ ] Zero instances of `runtime.GOOS` outside of `internal/platform`.
- [ ] Zero instances of `os` or `exec` side-effects outside of `internal/platform`.

---

*Last verified: 2026-03-30T11:30:00-04:00 by Ma'at Ground Truth Audit.*
