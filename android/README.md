# Pantheon Android App

Android companion app for Sirsi Pantheon, built with Jetpack Compose and backed by the shared Go mobile binding layer.

## Architecture

```
android/app/src/main/java/ai/sirsi/pantheon/
  bridge/         Kotlin <-> Go bridge (JSON string I/O)
  models/         Data classes matching Go JSON responses
  ui/theme/       Material 3 theme (Gold/Black/Lapis)
  ui/screens/     Per-deity Compose screens
  ui/components/  Reusable composables
```

The Go `mobile/` package exports functions that return JSON strings. `PantheonBridge.kt` calls these via the gomobile-generated AAR and deserializes responses into Kotlin data classes using `kotlinx.serialization`.

## Prerequisites

- Android Studio Ladybug or later
- JDK 17
- Go 1.22+
- gomobile (`go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init`)
- Android SDK (API 35)
- Android NDK (installed via SDK Manager)

## Build

### 1. Build the Go AAR

```bash
make android-aar
```

This produces `bin/android/pantheon.aar`.

### 2. Copy AAR to libs

```bash
mkdir -p android/app/libs
cp bin/android/pantheon.aar android/app/libs/
```

### 3. Build the app

Open `android/` in Android Studio and run, or:

```bash
cd android
./gradlew assembleDebug
```

## Brand

| Element | Value |
|---------|-------|
| Gold | `#C8A951` |
| Black | `#0F0F0F` |
| Deep Lapis | `#1A1A5E` |
| Min SDK | API 26 (Android 8.0) |
| Target SDK | API 35 |
| Bundle ID | `ai.sirsi.pantheon` |
