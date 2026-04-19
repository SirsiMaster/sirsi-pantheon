# Android Architecture Design

**Custodian:** Net (Neith) -- The Weaver  
**Status:** v0.16.0  
**Scope:** Pantheon Android companion app

---

## 1. Data Flow Architecture

```mermaid
graph TD
    subgraph Android App
        UI[Jetpack Compose Screens]
        VM[ViewModel / StateFlow]
        BR[PantheonBridge.kt]
        MD[Kotlin Data Models]
    end

    subgraph Go Mobile Layer
        MO[mobile/ package]
        AN[mobile/anubis.go]
        KA[mobile/ka.go]
        TH[mobile/thoth.go]
        SE[mobile/seba.go]
        SS[mobile/seshat.go]
    end

    subgraph Go Internal
        JK[internal/jackal]
        KAI[internal/ka]
        THI[internal/thoth]
        SEI[internal/seba]
        SSI[internal/seshat]
        PL[internal/platform/android.go]
    end

    UI -->|user action| VM
    VM -->|suspend call| BR
    BR -->|Mobile.xxxFunction JSON string| MO
    MO -->|delegates| AN & KA & TH & SE & SS
    AN -->|scan| JK
    KA -->|hunt| KAI
    TH -->|sync/compact| THI
    SE -->|detect| SEI
    SS -->|ingest| SSI
    JK & KAI & THI & SEI & SSI -->|platform ops| PL

    MO -->|JSON string response| BR
    BR -->|kotlinx.serialization decode| MD
    MD -->|typed data| VM
    VM -->|StateFlow emit| UI
```

### Data Transformation Chain

| Step | Input | Output | Where |
|------|-------|--------|-------|
| 1 | User tap | Coroutine launch | Screen composable |
| 2 | Function call | JSON options string | PantheonBridge.kt |
| 3 | JSON options string | Go struct | mobile/*.go |
| 4 | Go scan/detect | Go result struct | internal/* |
| 5 | Go result struct | JSON response string | mobile/mobile.go (successJSON) |
| 6 | JSON response string | BridgeResponse<T> | PantheonBridge.kt (decode) |
| 7 | Typed Kotlin data class | UI state | ViewModel / StateFlow |

---

## 2. Recommended Implementation Order

```mermaid
gantt
    title Android Implementation Phases
    dateFormat YYYY-MM-DD
    axisFormat %b %d

    section Phase 1 - Foundation
    Platform android.go           :done, p1a, 2026-04-18, 1d
    Makefile android-aar target   :done, p1b, 2026-04-18, 1d
    Gradle build scaffold         :done, p1c, 2026-04-18, 1d

    section Phase 2 - Core
    Data models (Kotlin)          :done, p2a, 2026-04-18, 1d
    PantheonBridge.kt             :done, p2b, 2026-04-18, 1d
    Theme + Typography            :done, p2c, 2026-04-18, 1d

    section Phase 3 - UI
    HomeScreen                    :done, p3a, 2026-04-18, 1d
    AnubisScreen                  :done, p3b, 2026-04-18, 1d
    KaScreen                      :done, p3c, 2026-04-18, 1d
    ThothScreen + SebaScreen      :done, p3d, 2026-04-18, 1d

    section Phase 4 - Polish
    CI workflow                   :done, p4a, 2026-04-18, 1d
    ViewModels (refactor)         :p4b, after p3d, 3d
    Unit + UI tests               :p4c, after p4b, 5d
    App icon + splash             :p4d, after p4b, 2d

    section Phase 5 - Release
    Signing config                :p5a, after p4c, 1d
    Play Store listing            :p5b, after p5a, 2d
    Beta release                  :p5c, after p5b, 1d
```

### Minimum Viable Pipeline

Phases 1-3 constitute the minimum viable Android app. The app can build, install, and invoke all Go mobile functions via the bridge. Phase 4 adds production polish (ViewModels, tests, branding assets). Phase 5 is distribution.

---

## 3. Key Decision Points

| Question | Options | Recommendation |
|----------|---------|----------------|
| UI framework | Jetpack Compose vs XML Views | **Jetpack Compose** -- modern, declarative, matches SwiftUI parity with iOS app. No XML layouts needed. |
| JSON parsing | Gson vs Moshi vs kotlinx.serialization | **kotlinx.serialization** -- compile-time safe, no reflection, multiplatform-ready, matches the `@SerialName` annotation pattern used by Go JSON tags. |
| State management | LiveData vs StateFlow vs Compose state | **Compose mutableStateOf** for initial scaffold; upgrade to **ViewModel + StateFlow** in Phase 4 for lifecycle awareness and testability. |
| Navigation | Fragment Navigation vs Compose NavHost | **Compose NavHost** -- single-activity architecture, no fragments, type-safe routes. |
| Go binding | JNI manual vs gomobile | **gomobile** -- generates AAR automatically, same toolchain as iOS xcframework. Zero JNI boilerplate. |
| Min SDK | API 21 (5.0) vs API 26 (8.0) | **API 26** -- drops legacy support burden, enables Java 8 desugaring-free APIs, covers 95%+ of active devices. |
| Theme | Light+Dark vs Dark-only | **Dark-only** -- matches Pantheon brand identity (gold on black). Egyptian aesthetic requires dark background. |
| Concurrency | RxJava vs Coroutines | **Kotlin Coroutines** -- native language support, structured concurrency, `Dispatchers.IO` for Go bridge calls. |
