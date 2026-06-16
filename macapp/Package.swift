// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "SirsiMenubar",
    platforms: [.macOS(.v13)],
    targets: [
        .executableTarget(
            name: "SirsiMenubar",
            path: "Sources/SirsiMenubar"
        )
    ]
)
