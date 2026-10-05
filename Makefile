# Sirsi Pantheon Build System

VERSION := $(shell cat VERSION)
BUILD_DIR ?= bin
INSTALL_DIR ?= $(HOME)/.local/bin
# Release builds strip debug symbols (-s) and DWARF tables (-w) → ~13MB core binary.
# Use `make build-debug` for full 20MB binary with symbols for dlv/pprof.
GO_LDFLAGS ?= -s -w -X github.com/SirsiMaster/sirsi-pantheon/internal/version.Version=v$(VERSION)
GO_FLAGS ?= -ldflags="$(GO_LDFLAGS)"

# Code-signing identity. Default `-` is ad-hoc (portable, but macOS keys TCC/FDA
# grants to the cdhash → every rebuild wipes Full Disk Access and re-prompts).
# Set SIGN_ID to a STABLE identity so grants survive rebuilds:
#   - a self-signed code-signing cert (free): SIGN_ID="Sirsi Pantheon Code Signing"
#   - a Developer ID (notarizable):           SIGN_ID="Developer ID Application: …"
# macOS then keys grants to the signing identity, not the bytes. See
# docs/APPLE-NOTARIZATION-CHECKLIST.md.
SIGN_ID ?= -

.PHONY: all clean build build-debug install uninstall build-agent build-menubar bundle bundle-dev dmg dmg-dev pkg pkg-dev release-dmg release-pkg publish test test-proof ios ios-framework android-aar brain-train brain-install

all: build

# --- Primary Build (stripped, ~13MB) ---
build:
	go build $(GO_FLAGS) -o $(BUILD_DIR)/sirsi ./cmd/sirsi/

# --- Debug Build (with symbols, ~20MB) ---
build-debug:
	go build -ldflags="-X github.com/SirsiMaster/sirsi-pantheon/internal/version.Version=v$(VERSION)" -o $(BUILD_DIR)/sirsi ./cmd/sirsi/

# --- Install to PATH ---
install: build
	@mkdir -p $(INSTALL_DIR)
	@tmp="$(INSTALL_DIR)/.sirsi.$$$$.new"; \
		cp $(BUILD_DIR)/sirsi "$$tmp"; \
		chmod +x "$$tmp"; \
		if [ "$$(uname -s)" = "Darwin" ] && command -v codesign >/dev/null 2>&1; then \
			codesign --force --sign "$(SIGN_ID)" "$$tmp" || { rm -f "$$tmp"; exit 1; }; \
		fi; \
		mv -f "$$tmp" "$(INSTALL_DIR)/sirsi"
	@# macOS arm64: sign the staged inode before the atomic rename so AMFI never
	@# sees an unsigned or partially-written binary at the canonical PATH.
	@# SIGN_ID="<stable identity>" keeps FDA grants across rebuilds (A6; A27 drift).
	@# Re-sign changes the cdhash; launchd keeps enforcing the OLD one per job
	@# (stale LWCR -> "Launch Constraint Violation" crash-loop; kickstart can't
	@# clear it). Bootout+bootstrap every ai.sirsi.* job with the NEW binary.
	@"$(INSTALL_DIR)/sirsi" launchd refresh || true
	@echo "✅ sirsi installed to $(INSTALL_DIR)/sirsi"

uninstall:
	rm -f $(INSTALL_DIR)/sirsi
	@echo "✅ sirsi removed from $(INSTALL_DIR)"

# --- Gemma worker + tooling (canonical source: scripts/gemma/) ---
install-gemma-worker:
	cp scripts/gemma/sirsi-gemma-worker.sh $(INSTALL_DIR)/sirsi-gemma-worker.sh
	cp scripts/gemma/sirsi-gemma-model-resolver.sh $(INSTALL_DIR)/sirsi-gemma-model-resolver.sh
	cp scripts/gemma/sirsi-gemma-triage.sh $(INSTALL_DIR)/sirsi-gemma-triage.sh
	chmod +x $(INSTALL_DIR)/sirsi-gemma-worker.sh $(INSTALL_DIR)/sirsi-gemma-model-resolver.sh $(INSTALL_DIR)/sirsi-gemma-triage.sh
	@# Restart the daemon so the running copy matches the repo (KeepAlive respawns it).
	@launchctl kickstart -k gui/$$(id -u)/ai.sirsi.gemma-worker 2>/dev/null || true
	@echo "✅ gemma worker + resolver + triage installed; daemon kickstarted"

clean:
	rm -rf $(BUILD_DIR)
	rm -rf Pantheon.app

# --- Agent Binary (stripped, static) ---
build-agent:
	CGO_ENABLED=0 go build $(GO_FLAGS) -o $(BUILD_DIR)/sirsi-agent ./cmd/sirsi-agent/

# --- Native Swift menu bar app ---
# Pantheon has one shipped macOS interface.  Do not fall back to the retired
# Go/systray implementation when the canonical Swift app is unavailable: a
# package that silently changes its interaction model is not a valid build.
build-menubar:
	@test -f macapp/Package.swift || (echo "ERROR: canonical Swift menubar source is missing" >&2; exit 1)
	@(cd macapp && swift build -c release)
	@mkdir -p $(BUILD_DIR)
	@cp macapp/.build/release/SirsiMenubar $(BUILD_DIR)/sirsi-menubar

# --- macOS development .app Bundle ---
# Creates Pantheon.app suitable for /Applications. The menu-bar surface resolves
# its operational engine as a sibling, so both executables are one package unit.
# This is deliberately not a commercial output: use release-dmg/release-pkg
# for Developer-ID signed, notarized, stapled artifacts.
bundle: bundle-dev

bundle-dev: build build-menubar
	@echo "📦 Building Pantheon.app development bundle..."
	@rm -rf Pantheon.app
	@mkdir -p Pantheon.app/Contents/MacOS
	@mkdir -p Pantheon.app/Contents/Resources
	@cp $(BUILD_DIR)/sirsi Pantheon.app/Contents/MacOS/sirsi
	@cp $(BUILD_DIR)/sirsi-menubar Pantheon.app/Contents/MacOS/sirsi-menubar
	@cp cmd/sirsi-menubar/bundle/Info.plist Pantheon.app/Contents/Info.plist
	@cp cmd/sirsi-menubar/bundle/PkgInfo Pantheon.app/Contents/PkgInfo
	@cp cmd/sirsi-menubar/bundle/ai.sirsi.pantheon.plist Pantheon.app/Contents/Resources/ai.sirsi.pantheon.plist
	@cp docs/assets/pantheon-icon.png Pantheon.app/Contents/Resources/pantheon-icon.png
	@cp -R contracts/stacklab Pantheon.app/Contents/Resources/StackLab
	@# The bundle, CLI and eventual DMG/PKG are one product identity. Never let
	@# the source plist's historical template version survive into a local app.
	@/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $(VERSION)" Pantheon.app/Contents/Info.plist
	@/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $(VERSION)" Pantheon.app/Contents/Info.plist
	@codesign --force --deep --sign - Pantheon.app
	@test -x Pantheon.app/Contents/MacOS/sirsi
	@test -x Pantheon.app/Contents/MacOS/sirsi-menubar
	@$(BUILD_DIR)/sirsi package-inventory \
		--app "$(abspath Pantheon.app)" \
		--version "$(VERSION)" \
		--build "$(VERSION)" \
		--info-plist "$(abspath Pantheon.app/Contents/Info.plist)" \
		--pkg-info "$(abspath cmd/sirsi-menubar/bundle/PkgInfo)" \
		--launch-agent "$(abspath cmd/sirsi-menubar/bundle/ai.sirsi.pantheon.plist)" \
		--brand-logo "$(abspath docs/assets/pantheon-icon.png)" \
		--require-code-signature
	@echo "✅ Pantheon.app development bundle created (ad-hoc signed; not for distribution)"

# --- macOS DMG Installer ---
dmg: dmg-dev

dmg-dev:
	@echo "📦 Creating development DMG..."
	scripts/build-dmg.sh --development --version $(VERSION) --arch $(shell uname -m)

release-dmg:
	@echo "📦 Creating commercial release DMG..."
	scripts/build-dmg.sh --release --version $(VERSION) --arch $(shell uname -m)

# --- macOS PKG Installer ---
# Produces an installer from the same assembled Pantheon.app as the DMG.
pkg: pkg-dev

pkg-dev:
	@echo "📦 Creating development PKG..."
	scripts/build-dmg.sh --development --version $(VERSION) --arch $(shell uname -m)
	scripts/build-pkg.sh --development --version $(VERSION) --arch $(shell uname -m) --app Pantheon.app

release-pkg:
	@echo "📦 Creating commercial release PKG..."
	scripts/build-dmg.sh --release --version $(VERSION) --arch $(shell uname -m)
	scripts/build-pkg.sh --release --version $(VERSION) --arch $(shell uname -m) --app Pantheon.app

# --- Horus Auto-Publish ---
# Generates docs/build-log.html and docs/case-studies.html
publish:
	@echo "𓂀 Horus Auto-Publish..."
	@go run ./cmd/sirsi-menubar/ -publish 2>/dev/null || \
		echo "  ℹ️  Publish via Go: go run -tags publish ./internal/horus/..."

# --- LaunchAgent (auto-start at login) ---
install-launchagent:
	@echo "📋 Installing LaunchAgent..."
	@mkdir -p ~/Library/LaunchAgents
	@sed "s|BINARY_PATH|$(shell pwd)/$(BUILD_DIR)/sirsi-menubar|g" \
		cmd/sirsi-menubar/bundle/ai.sirsi.pantheon.plist > \
		~/Library/LaunchAgents/ai.sirsi.pantheon.plist
	@launchctl load ~/Library/LaunchAgents/ai.sirsi.pantheon.plist
	@echo "✅ Pantheon will start at login"

uninstall-launchagent:
	@launchctl unload ~/Library/LaunchAgents/ai.sirsi.pantheon.plist 2>/dev/null || true
	@rm -f ~/Library/LaunchAgents/ai.sirsi.pantheon.plist
	@echo "✅ LaunchAgent removed"

# --- iOS Framework (gomobile) ---
# Builds PantheonCore.xcframework for the SwiftUI app
ios-framework:
	@echo "📱 Building PantheonCore.xcframework..."
	@which gomobile > /dev/null 2>&1 || (echo "❌ gomobile not found. Install: go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init" && exit 1)
	@mkdir -p $(BUILD_DIR)/ios
	gomobile bind -target=ios -o $(BUILD_DIR)/ios/PantheonCore.xcframework $(GO_FLAGS) ./mobile/
	@echo "✅ Framework built: $(BUILD_DIR)/ios/PantheonCore.xcframework"
	@echo "   Add to Xcode: ios/Pantheon.xcodeproj → Frameworks, Libraries"

# Full iOS build: framework + Xcode archive
ios: ios-framework
	@echo "📱 Building Pantheon iOS app..."
	@if [ ! -d "ios/Pantheon.xcodeproj" ]; then \
		echo "⚠️  Xcode project not found. Open ios/ in Xcode to create it, then add PantheonCore.xcframework."; \
		exit 1; \
	fi
	@cp -R $(BUILD_DIR)/ios/PantheonCore.xcframework ios/
	xcodebuild -project ios/Pantheon.xcodeproj \
		-scheme Pantheon \
		-destination 'generic/platform=iOS' \
		-configuration Release \
		archive -archivePath $(BUILD_DIR)/ios/Pantheon.xcarchive
	@echo "✅ iOS archive: $(BUILD_DIR)/ios/Pantheon.xcarchive"

# --- Android AAR (gomobile) ---
# Builds pantheon.aar for the Android app
android-aar:
	@echo "🤖 Building pantheon.aar..."
	@which gomobile > /dev/null 2>&1 || (echo "❌ gomobile not found. Install: go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init" && exit 1)
	@mkdir -p $(BUILD_DIR)/android
	gomobile bind -target=android -o $(BUILD_DIR)/android/pantheon.aar $(GO_FLAGS) ./mobile/
	@echo "✅ AAR built: $(BUILD_DIR)/android/pantheon.aar"

# --- Brain Model Training Pipeline ---
brain-train:
	@echo "Training Brain classifier..."
	cd scripts/brain-training && pip install -r requirements.txt && python generate_training_data.py && python train_model.py
	@echo "Model at scripts/brain-training/classifier.mlmodelc"

brain-install: brain-train
	@mkdir -p $(HOME)/.config/sirsi/brain
	cp -R scripts/brain-training/classifier.mlmodelc $(HOME)/.config/sirsi/brain/classifier.mlmodelc
	@echo "Brain model installed to ~/.config/sirsi/brain/classifier.mlmodelc"

# --- Test ---
test:
	go test -race -short ./...

test-cover:
	go test -race -short -coverprofile=$(BUILD_DIR)/coverage.out ./...
	@go tool cover -func=$(BUILD_DIR)/coverage.out | tail -1

test-proof:
	go test -v -race -coverprofile=$(BUILD_DIR)/coverage.out ./...
	go tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html
	@echo "Public proof generated in $(BUILD_DIR)/coverage.html"
