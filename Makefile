# Pantheon Modular Build System

VERSION ?= v0.4.0-alpha
BUILD_DIR ?= bin
GO_FLAGS ?= -ldflags="-X main.Version=$(VERSION)"

.PHONY: all clean build-all build-anubis build-thoth build-maat build-scarab build-guard build-agent build-menubar bundle publish test-proof ios ios-framework

all: build-all

# --- Standard Build ---
build-all: build-anubis build-thoth build-maat build-scarab build-guard build-agent build-menubar

clean:
	rm -rf $(BUILD_DIR)
	rm -rf Pantheon.app

# --- Individual Deity Binaries ---
build-anubis:
	go build $(GO_FLAGS) -o $(BUILD_DIR)/anubis ./cmd/anubis/

build-thoth:
	go build $(GO_FLAGS) -o $(BUILD_DIR)/thoth ./cmd/thoth/

build-maat:
	go build $(GO_FLAGS) -o $(BUILD_DIR)/maat ./cmd/maat/

build-scarab:
	go build $(GO_FLAGS) -o $(BUILD_DIR)/scarab ./cmd/scarab/

build-guard:
	go build $(GO_FLAGS) -o $(BUILD_DIR)/guard ./cmd/guard/

build-agent:
	go build $(GO_FLAGS) -o $(BUILD_DIR)/sirsi-agent ./cmd/sirsi-agent/

# --- Menu Bar App (ADR-010) ---
build-menubar:
	go build $(GO_FLAGS) -o $(BUILD_DIR)/sirsi-menubar ./cmd/sirsi-menubar/

# --- macOS .app Bundle ---
# Creates Pantheon.app suitable for /Applications
bundle: build-menubar
	@echo "📦 Building Pantheon.app bundle..."
	@rm -rf Pantheon.app
	@mkdir -p Pantheon.app/Contents/MacOS
	@mkdir -p Pantheon.app/Contents/Resources
	@cp $(BUILD_DIR)/sirsi-menubar Pantheon.app/Contents/MacOS/sirsi-menubar
	@cp cmd/sirsi-menubar/bundle/Info.plist Pantheon.app/Contents/Info.plist
	@cp cmd/sirsi-menubar/bundle/PkgInfo Pantheon.app/Contents/PkgInfo
	@echo "✅ Pantheon.app created — install with: cp -R Pantheon.app /Applications/"

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

# --- Public Proof of Testing ---
test-proof:
	go test -v -coverprofile=$(BUILD_DIR)/coverage.out ./...
	go tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html
	@echo "Public proof generated in $(BUILD_DIR)/coverage.html"
