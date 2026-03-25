# 𓆄 Ma'at Quality Charter — The Pantheon Test Plan

## Overview
This document defines the canonical quality standards for the Sirsi Pantheon and its constituent deities (Anubis, Thoth, Ma'at, Horus, Ra, Seba). 

Quality is not a checkbox; it is the **Weighing of the Heart**. A module without tests is a module without truth.

## 1. The QA Sovereign: Ma'at
**Ma'at** is the deity responsible for the stewardship of this plan. 
- **Development**: Ma'at codifies these rules into executable assessments.
- **Administration**: Ma'at executes the tests and parses the results.
- **Validation**: Ma'at calculates the **Feather Weight (0-100)** score.
- **Insight**: Ma'at notifies other deities of their "quality debt." (e.g., "Horus, your index logic is only 40% verified; you are not yet canon.")

## 2. Test Architecture & Storage

### 2.1 Storage Locations
Tests follow the Go standard but are categorized by depth:
1.  **Unit Tests**: `internal/<pkg>/<pkg>_test.go`. Fast, zero side-effects.
2.  **Integration Tests**: `internal/<pkg>/<pkg>_sprint_test.go`. Exercises side-effects via Interface Injection.
3.  **Governance Tests**: `internal/maat/*_test.go`. Tests the testers — verifying that Ma'at's own logic is sound.

### 2.2 The "Interface Injection" Standard (Rule A16)
To achieve the **99% Coverage Target**, no logic may perform a system-level side effect directly.
- **Mocks**: Every side effect MUST have a mock implementation.
- **Determinism**: Tests must be 100% deterministic. No `time.Sleep`, no real network, no real file deletion on the host.

## 3. Metrics & Thresholds

| Metric | Level 1 (Alpha) | Level 2 (Beta) | Level 3 (Canon) |
|:-------|:----------------|:---------------|:----------------|
| **Weighted Coverage** | > 80% | > 90% | **99.0%** |
| **Critical Path Coverage** | 100% | 100% | 100% |
| **Feather Weight** | > 70 | > 85 | **> 95** |
| **Race Conditions** | Zero | Zero | Zero |

### 3.1 The Feather Weight Formula
The Feather Weight is a composite score calculated by Ma'at:
- **60%**: Statement Coverage (Weighted by module importance)
- **20%**: Canon Linkage (Does the code link to ADRs/Rules?)
- **20%**: Pipeline Integrity (Is CI green for this module?)

## 4. Current State (Session 16b)
- **Status**: Level 2 (Beta) achieved — **90.1% Weighted Coverage**.
- **Test Count**: 768 passing.
- **Projected tests needed for 99%**: ~232 more tests (Total: 1,000+).

## 5. Deployment to Tenant Apps
This Quality Charter applies to **Assiduous** and **FinalWishes** by inheritance (Rule 18).
- Ma'at will be deployed to those repos as a standalone binary in the next phase.
- Each tenant app will maintain its own `docs/QA_PLAN.md` mapping to its specific domain (e.g., Estate Planning logic vs Database integrity).

---
**Custodian**: 𓆄 Ma'at
**Last Assessment**: Session 16b — 90.1% Passing.
