# Sirsi Master Execution Plan
**Version:** 1.0.0 | **Date:** April 4, 2026 | **Author:** Cylton Collymore + Claude Opus 4.6
**Custodian:** Neith (scope alignment) + Ra (orchestration) + Ma'at (quality gates)

> This document is the single source of truth for all Sirsi portfolio development.
> Neith reads this to assemble scope prompts. Ra reads this to prioritize agent deployment.
> Every Ra-spawned agent inherits this context. Update this document as milestones are hit.

---

## 1. Portfolio Overview

| Repo | Product | Client | Contract | Deadline | Ship Target |
|------|---------|--------|----------|----------|-------------|
| **FinalWishes** | Estate Operating System | Tameeka Lockhart | $95K fixed-bid (Tier 1) | May 15, 2026 (web MVP) | July 2026 (full w/ mobile) |
| **Assiduous** | Real Estate Platform | Owner TBD | $110.4K (368 hrs) | ASAP | June 2026 (full w/ mobile) |
| **sirsi-pantheon** | Infrastructure Hygiene CLI | Open-source | N/A | Ongoing | v1.0 June 2026 |
| **SirsiNexusApp** | Platform Monorepo | Internal | N/A | Ongoing | v1.0-beta May 2026 |

---

## 2. Priority Stack

**Rule: Revenue-generating client work always outranks internal tooling.**

| Priority | Repo | Why |
|----------|------|-----|
| **P0** | **FinalWishes** | Contracted client. May 15 deadline. Sprint 6 (Shepherd AI) = next payment milestone ($23,750). |
| **P0** | **Assiduous** | Contracted client. ASAP. Sprint 13 (E-Signatures) = deal-closing capability. |
| **P1** | **NexusApp** | Internal infrastructure. Powers FW + Assiduous. Epoch 1-2 = commerce engine. |
| **P2** | **Pantheon** | Orchestration tooling. Background work. Supports P0 repos via Ra/Neith/Ma'at. |

**Ra Deploy Rule:** When deploying agents, always allocate at least one window each to FinalWishes and Assiduous. Pantheon and NexusApp get remaining capacity.

---

## 3. FinalWishes — Sprint Plan & Stage Gates

### Current State (April 4, 2026)
- **Version:** 0.10.0-alpha
- **Sprints Complete:** 4 of 16
- **What works:** Document Vault, YouTube Memorials, Photo Gallery, PII Vault (Cloud KMS + AES-256), Firebase Auth
- **What doesn't:** Shepherd AI, Assets, Beneficiary Invitations, Time Capsules, Directives, Mobile, Desktop

### Contract Milestones (Payment Gates)

| Milestone | Trigger | Payment | Status |
|-----------|---------|---------|--------|
| M1: Kickoff | Contract signing | $23,750 | PAID |
| **M2: Alpha** | **Shepherd AI logic working** | **$23,750** | **NEXT — Sprint 6** |
| M3: Beta | Integrations live (assets, beneficiaries, capsules) | $23,750 | Sprints 7-9 |
| M4: Launch | App Store approval | $23,750 | Sprint 15 |

### Sprint Execution Plan

**PHASE A: Web MVP (May 15 deadline)**

| Sprint | Scope | Duration | Depends On | Gate |
|--------|-------|----------|------------|------|
| **5** | Production Hardening — Cloud Run deploy, SendGrid key, WAF, SMS MFA | 1 week | Nothing | Cloud Run serves API |
| **6** | Shepherd AI — 4 Genkit flows (completion score, chat, obituary, suggestions) | 2 weeks | Sprint 5 (API live) | **M2 PAYMENT GATE: Shepherd demo to client** |
| **7** | Asset Management — 5-category CRUD, heir designation, doc linking | 1.5 weeks | Sprint 5 | Assets functional |
| **8** | Beneficiary Management — executor/heir invitations, email flow, priority | 1.5 weeks | Sprint 5 | Invitations sending |

> **May 15 deliverable:** Web app with Shepherd AI, Asset Management, Beneficiary Invitations, Document Vault, Memorials. Client can create an estate, upload documents, record memorials, add assets, invite executors/heirs, and see AI completion score.

**PHASE B: Core Features (June)**

| Sprint | Scope | Duration | Depends On | Gate |
|--------|-------|----------|------------|------|
| **9** | Time Capsules — trigger types, Cloud Tasks, delivery, cooling-off | 2 weeks | Sprint 8 | Capsules deliver |
| **10** | Final Directives — ethical will editor, funeral prefs, PDF export, e-sign | 2 weeks | Sprint 5 | **M3 PAYMENT GATE: Integrations demo** |

**PHASE C: Mobile + Launch (July)**

| Sprint | Scope | Duration | Depends On | Gate |
|--------|-------|----------|------------|------|
| **11** | React Native Phase 1 — auth, dashboard, vault, memorials | 2 weeks | Sprint 10 | Core screens work |
| **12** | React Native Phase 2 — biometrics, camera, push, offline | 2 weeks | Sprint 11 | Feature parity |
| **13** | Tauri Desktop (macOS) | 1 week | Sprint 10 | Wrapper runs |
| **14** | QA + Security — Playwright E2E, k6 load test, pen test | 2 weeks | Sprint 12 | All gates green |
| **15** | App Store + Play Store Submission | 1.5 weeks | Sprint 14 | **M4 PAYMENT GATE: Store approval** |
| **16** | Production Launch — DNS, monitoring, onboarding | 1 week | Sprint 15 | **SHIPPED** |

### Parallelization Opportunities
- Sprints 7 + 8 can run simultaneously (independent features)
- Sprints 11 + 12 (mobile) can overlap with Sprint 14 (QA on web)
- Sprint 13 (desktop) is independent of everything after Sprint 10

### Key Technical Decisions
- **Shepherd AI:** Firebase Genkit + Gemini Flash (Vertex AI). 4 flows: `compute_completion_score`, `shepherd_chat`, `draft_obituary`, `suggest_next_action`
- **Time Capsules:** Cloud Tasks for deferred delivery. 72-hour cooling-off via Cloud Scheduler.
- **E-Signatures:** OpenSign (via Sirsi Sign). Iframe embed, webhook confirmation.
- **Mobile:** React Native + Expo. Shared hooks/stores with web.
- **Desktop:** Tauri (Rust wrapper, zero custom logic).

### Infrastructure (Live — finalwishes-prod)
- Cloud Run (Go API) — configured, not yet deployed
- Cloud SQL PostgreSQL 15 (PII vault) — live
- Cloud KMS (AES-256, 365-day rotation) — live
- Firestore (16 composite indexes) — live
- Firebase Auth (TOTP MFA) — live
- Firebase Extension (email via SendGrid) — configured, awaiting API key
- OpEx: ~$85/month

---

## 4. Assiduous — Sprint Plan & Stage Gates

### Current State (April 4, 2026)
- **Version:** 0.145.0
- **Phases Complete:** 0-10 (38 of 71 features code-complete)
- **What works:** 34 React pages, Firebase Auth, Firestore hooks for P0/P1, Stripe payments, QR system, MicroFlip analyzer, agent approval, lead management
- **What doesn't:** E-signatures (CRITICAL), deal Kanban, map view, Go backend, 15 P2 pages on mock data, mobile

### Contract Milestones

| Milestone | Trigger | Status |
|-----------|---------|--------|
| M1: Planning | 33 canonical docs approved | DONE |
| **M2: Backend** | **Go services live on Cloud Run, gRPC <200ms** | **NEXT** |
| M3: Web App | React SPA deployed, P0/P1 wired to live data | DONE |
| M4: Mobile | iOS TestFlight + Android Internal Testing | NOT STARTED |
| M5: Acceptance | Owner acceptance certificate, IP transfer | PENDING |

### Sprint Execution Plan

**PHASE A: Deal-Closing Capability (May)**

| Sprint | Scope | Duration | Depends On | Gate |
|--------|-------|----------|------------|------|
| **11** | Go Backend Migration — PropertyService, DealService, TransactionService via ConnectRPC | 2-3 weeks | Nothing | **M2 GATE: gRPC endpoints live** |
| **12** | P2 Page Data Wiring — Messages, Documents, Offers, Schedule, Commissions (15 pages) | 1.5 weeks | Sprint 11 (partial) | P2 pages show real data |
| **13** | E-Signatures — OpenSign integration, signing workflow, audit trail | 1 week | Sprint 11 | **CRITICAL: Agents can close deals** |
| **14** | Deal Pipeline — Kanban board, stage management, multi-party tracking | 1.5 weeks | Sprint 11 | Visual deal management |

**PHASE B: Feature Completion (May-June)**

| Sprint | Scope | Duration | Depends On | Gate |
|--------|-------|----------|------------|------|
| **15** | Map + Geo — Google Maps property view, geo-search, neighborhood data | 1 week | Sprint 11 | Location features |
| **16** | Partial Feature Fixes — commission disbursement, alerts, agent metrics, FCM push | 1 week | Sprint 11 | 17 partial features completed |

**PHASE C: Mobile + Launch (June)**

| Sprint | Scope | Duration | Depends On | Gate |
|--------|-------|----------|------------|------|
| **17** | React Native Mobile — iOS + Android core screens, biometrics, camera | 3 weeks | Sprint 16 | **M4 GATE: TestFlight ready** |
| **18** | QA + Security — Dependabot (3 critical), Firestore audit, load test, E2E | 1.5 weeks | Sprint 17 | All gates green |
| **19** | Launch — App Store submission, DNS, monitoring, IP transfer | 1.5 weeks | Sprint 18 | **M5 GATE: SHIPPED** |

### Parallelization Opportunities
- Sprint 11 (Go backend) + Sprint 13 (E-signatures) can partially overlap — e-sign doesn't require full Go migration
- Sprints 12 + 14 (P2 wiring + deal pipeline) can run simultaneously
- Sprint 15 (maps) is independent after Sprint 11

### Key Technical Decisions
- **Go Backend:** Chi router + ConnectRPC. Proto-generated types for frontend.
- **E-Signatures:** OpenSign SDK. 6-step workflow: upload → select signatories → send → sign → collect → store.
- **Mobile:** React Native + Expo. Shared code with web where possible.
- **Data Sources:** Manual property entry for launch. MLS feed integration is post-delivery Phase 2.

### Infrastructure (Live — assiduous-prod)
- Firebase Hosting (React SPA) — live
- Cloud Functions v2 (TypeScript backend) — live (Go migration target)
- Cloud Run (Go API) — live
- Firestore (21 collections, 450+ lines security rules) — live
- Stripe (checkout, subscriptions, refunds) — live
- SendGrid (5 email templates) — live
- OpEx: ~$25-150/month

---

## 5. Sirsi Pantheon — Phase Plan

### Current State (April 4, 2026)
- **Version:** 0.10.0 (tagged, pushed)
- **What's shipped:** 33 modules, Stele universal event bus, Ra orchestrator, ProtectGlyph, Ka v1.1.0, Seshat v2.0, Ma'at governance, 1,450+ tests
- **Role in portfolio:** Provides Ra (agent orchestration), Neith (scope assembly), Ma'at (quality gates) for FW + Assiduous work

### Phase Plan

| Phase | Scope | Status | Est. to Complete |
|-------|-------|--------|-----------------|
| 1-3 (Jackal, Jackal+, Hapi) | Local CLI, ghost detection, hardware profiling | DONE | — |
| **Test Coverage** | Thoth 0%→60%, Neith stub→full, Hapi 62%→84% | IN PROGRESS | 1-2 weeks |
| **4 (Scarab)** | Fleet agent-controller, gRPC, subnet discovery | FOUNDATION | 2-3 weeks |
| **6 (Scales)** | Policy engine, fleet enforcement | PARTIAL | 2 weeks |
| **Cross-platform** | Windows Ka, Linux full coverage | STUBS | 2-3 weeks |
| 5 (Scarab+) | SAN/NAS, cloud storage backends | NOT STARTED | 2 weeks |
| 7 (Temple) | Web dashboard / SwiftUI GUI | NOT STARTED | 3-4 weeks |

**v1.0.0-rc1 target:** May 15 (test gaps closed, Neith implemented, dogfood starts)
**v1.0.0 stable target:** June 15 (after 30-day dogfooding)

### Pantheon's Service Role
Pantheon is not just a product — it's the orchestration layer for the entire portfolio:
- **Ra** deploys parallel agents across FW + Assiduous + NexusApp
- **Neith** reads THIS document to assemble scope prompts
- **Ma'at** validates build + test between sprints
- **Stele** records all deity activity for the Command Center
- **Thoth** compacts memory between sessions

Priority Pantheon work should focus on features that directly improve Ra/Neith/Ma'at effectiveness for P0 repos.

---

## 6. SirsiNexusApp — Epoch Plan

### Current State (April 4, 2026)
- **Version:** 0.9.3-alpha
- **What's shipped:** Firebase Auth + MFA, 50+ UI components, ConnectRPC backend, Sirsi Sign, Admin portal (25 routes), CI/CD green
- **Role in portfolio:** Shared infrastructure. Auth, payments, e-signing, component library consumed by FW + Assiduous.

### Epoch Plan (30 sprints through December 2026)

| Epoch | Sprints | Scope | Target Date | Version |
|-------|:-------:|-------|-------------|---------|
| 0 | 0 | Repo hygiene, merge PRs, Dependabot | April 2026 | v0.9.4-alpha |
| **1** | 1-3 | **Ship the Business** (email, SQL, portals, Stripe onboarding) | April 2026 | v0.9.6-alpha |
| **2** | 4-6 | **Harden & Observe** (WAF, monitoring, Sirsi Sign vault) | May 2026 | **v1.0.0-beta** |
| **3** | 7-9 | **Truth Engine** (AlloyDB AI, Knowledge Graph, Hedera HCS) | June 2026 | v1.1.0-alpha |
| 4 | 10-13 | Agent Swarm (Fire Team Protocol, NebuLang routing) | July 2026 | v1.2.0-alpha |
| 5 | 14-16 | Autonomous CTO (Claude Opus, Neural-Fractal) | August 2026 | v1.3.0-alpha |
| 6-7 | 17-21 | Commerce at Scale + Real-Time Intelligence | Sep 2026 | v1.4-1.5-alpha |
| 8 | 22-24 | Multi-Platform (Rust/Tauri, React Native) | Oct 2026 | v1.6.0-alpha |
| 9-10 | 25-30 | Direct-to-Metal + Sovereign Compute (Mac Studio cluster) | Dec 2026 | **v1.0.0-stable** |

### NexusApp's Service Role
NexusApp provides shared rails consumed by FW and Assiduous:
- **Sirsi Sign** → FinalWishes (directive signing) + Assiduous (contract signing)
- **Stripe** → Both (payment processing)
- **UCS Components** → Both (shared UI kit)
- **Firebase Auth** → Both (authentication backbone)

Epoch 1-2 work should prioritize rails that unblock FW Sprint 10 (e-sign) and Assiduous Sprint 13 (e-sign).

---

## 7. Cross-Repo Dependencies

```
FinalWishes Sprint 10 (Directives + E-Sign)
  └── depends on: Sirsi Sign (NexusApp) being stable
  
Assiduous Sprint 13 (E-Signatures)
  └── depends on: Sirsi Sign (NexusApp) being stable

FinalWishes Sprint 6 (Shepherd AI)
  └── depends on: Sprint 5 (Cloud Run API deployed)
  └── no cross-repo dependency

Assiduous Sprint 11 (Go Backend)
  └── no cross-repo dependency (self-contained)

Pantheon Ra Deploy
  └── reads: configs/scopes/*.yaml
  └── reads: THIS DOCUMENT (via Neith loom)
  └── writes: ~/.config/ra/stele.jsonl
```

**Critical Path:** Sirsi Sign must be stable before FW Sprint 10 and Assiduous Sprint 13. If Sirsi Sign has issues, both client products are blocked on e-signatures.

---

## 8. Ra Deployment Configuration

### Scope Priority for Daily Agent Deployment

When running `sirsi ra deploy`, agents should be allocated based on current sprint:

**April 2026:**
| Scope | Repo | Active Sprint | Priority |
|-------|------|---------------|----------|
| finalwishes | FinalWishes | Sprint 5 → 6 | P0 |
| assiduous | Assiduous | Sprint 11 | P0 |
| nexus | SirsiNexusApp | Epoch 0 → 1 | P1 |
| pantheon | sirsi-pantheon | Test coverage | P2 |

**May 2026:**
| Scope | Repo | Active Sprint | Priority |
|-------|------|---------------|----------|
| finalwishes | FinalWishes | Sprint 7-8 (parallel) | P0 |
| assiduous | Assiduous | Sprint 12-14 | P0 |
| nexus | SirsiNexusApp | Epoch 1-2 | P1 |
| pantheon | sirsi-pantheon | Scarab + Scales | P2 |

### Neith Scope Assembly Rules
1. Always include this document's relevant section in the woven prompt
2. Reference the current sprint number and its scope from this plan
3. Include the stage gate criteria so the agent knows what "done" looks like
4. Include cross-repo dependencies if the sprint depends on another repo
5. Never assign work from a future sprint unless the current sprint gate is passed

---

## 9. Quality Gates (Ma'at Enforcement)

### Per-Sprint Gates
Every sprint must pass before the next begins:
- [ ] All new code builds cleanly (`go build ./...` or `npm run build`)
- [ ] All tests pass (`go test ./...` or `npm test`)
- [ ] Zero lint errors
- [ ] Commit pushed to main
- [ ] Changelog updated

### Payment Milestone Gates (FinalWishes)
- **M2 (Shepherd):** Client can see completion score on dashboard + ask Shepherd a question and get a response
- **M3 (Integrations):** Client can add an asset, invite an executor, create a time capsule, sign a directive
- **M4 (Launch):** iOS app approved on TestFlight, Android on internal testing track, web fully functional

### Delivery Gates (Assiduous)
- **M2 (Backend):** All gRPC endpoints respond <200ms, Firestore reads work, Go services on Cloud Run
- **M4 (Mobile):** iOS TestFlight build accepted, Android Internal Testing approved
- **M5 (Acceptance):** Owner signs acceptance certificate, GitHub repo transferred, Firebase project transferred

---

## 10. Expectations & Velocity Notes

This plan assumes conservative sprint estimates. Actual velocity with Ra multi-agent deployment may be significantly faster. Adjust timelines upward as velocity data comes in.

**Velocity tracking:** After each Ra deploy cycle, record:
- Sprints attempted vs completed
- Wall-clock time vs estimated time
- Blockers encountered
- Quality gate pass rate

Update this document weekly with actual progress. Neith should reference the most recent actuals, not just the plan.

---

## 11. The Hedera Bridge

The Stele (v0.10.0) is the local proving ground for distributed event consensus. The migration path:

| Current (Local) | Future (Distributed) |
|-----------------|---------------------|
| `stele.Inscribe()` → `stele.jsonl` | `stele.Inscribe()` → Hedera HCS Topic |
| `stele.NewReader()` → file offset | `stele.NewReader()` → HCS subscription |
| SHA-256 hash chain | Hedera aBFT consensus (strongest possible) |
| Single machine | Every node in fleet |
| `stele.Verify()` → walk local file | `stele.Verify()` → Hedera mirror node query |

**Timeline:** Hedera integration begins in NexusApp Epoch 3 (May-June 2026). Pantheon Stele migrates to Hedera transport in v1.1.0 (post-v1.0 stable).

---

*This document is the canon. Neith weaves from it. Ra deploys by it. Ma'at judges against it. Update it as reality changes.*

**Last updated:** April 4, 2026 02:30 ET
**Next review:** After first Ra multi-agent deploy cycle
