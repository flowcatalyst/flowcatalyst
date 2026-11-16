// swift-tools-version:5.9
// Test-only package for the pod's pure-Swift helpers, so they run under `swift test` on the host
// (swift test --package-path ios). The pod (../FlowcatalystFieldReactNative.podspec) is what apps consume;
// the pod has no test target.
import PackageDescription

let package = Package(
    name: "FcFieldOrdering",
    platforms: [.iOS(.v15), .macOS(.v12)],
    targets: [
        .target(name: "FcFieldOrdering", path: "Ordering"),
        .testTarget(name: "FcFieldOrderingTests", dependencies: ["FcFieldOrdering"], path: "Tests/FcFieldOrderingTests"),
    ]
)
