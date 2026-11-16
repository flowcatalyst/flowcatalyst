// swift-tools-version:5.9
// Check-only package; apps consume the pod (../FlowcatalystFieldReactNative.podspec), which has no test target.
//
//   FcFieldBridge    every Swift file of the pod as ONE module, the way the pod compiles them (the bridge, the
//                    vendored field-ios copy in FieldIos/, Ordering/). Building it catches what a sync of FieldIos can
//                    break: a name defined twice, an import of a module that does not exist in the pod. The
//                    Objective-C++ module class (FcField.mm) needs React Native and is not part of it.
//                    Its tests cover the pure-Swift helpers (the ordering helper).
//
//   swift build --package-path ios      swift test --package-path ios
import PackageDescription

let package = Package(
    name: "FcFieldBridge",
    platforms: [.iOS(.v15), .macOS(.v12)],
    products: [.library(name: "FcFieldBridge", targets: ["FcFieldBridge"])],  // gives xcodebuild a scheme
    targets: [
        .target(
            name: "FcFieldBridge",
            path: ".",
            exclude: ["FcField.mm", "Tests", "FieldIos/VERSION"],
            sources: ["FcFieldHost.swift", "FcFieldJson.swift", "FieldBridgeConfig.swift", "SecureStoreKeychain.swift", "FieldIos/Core", "FieldIos/Apple", "Ordering"]
        ),
        .testTarget(name: "FcFieldOrderingTests", dependencies: ["FcFieldBridge"], path: "Tests/FcFieldOrderingTests"),
    ]
)
