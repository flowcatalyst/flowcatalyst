// swift-tools-version:5.9
// Test-only package: builds the pure-Swift DER helpers so they can run under `swift test` on the host.
// The CocoaPods podspec (../FlowcatalystFieldReactNative.podspec) is what apps consume.
import PackageDescription

let package = Package(
    name: "FcFieldDer",
    platforms: [.iOS(.v13), .macOS(.v11)],
    targets: [
        .target(name: "FcFieldDer", path: "Der"),
        .testTarget(name: "FcFieldDerTests", dependencies: ["FcFieldDer"], path: "Tests/FcFieldDerTests"),
    ]
)
