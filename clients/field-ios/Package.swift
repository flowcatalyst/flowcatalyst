// swift-tools-version:5.9
// FlowCatalyst field worker library for iOS (Swift). The counterpart of clients/field-android.
//
//   FlowCatalystFieldCore  Foundation only (plus CryptoKit / swift-crypto for SHA-256): the session state machine,
//                          DPoP proofs, models, errors, timers. Compiles and tests on Linux.
//   FlowCatalystField      Apple implementations (Secure Enclave key, Keychain, LocalAuthentication, URLSession)
//                          and URLRequest helpers. Re-exports the core.
import PackageDescription

let cryptoOffApple: Target.Dependency = .product(
    name: "Crypto",
    package: "swift-crypto",
    condition: .when(platforms: [.linux, .android, .windows, .wasi, .openbsd])
)

let package = Package(
    name: "FlowCatalystField",
    platforms: [.iOS(.v15), .macOS(.v12)],
    products: [
        .library(name: "FlowCatalystFieldCore", targets: ["FlowCatalystFieldCore"]),
        .library(name: "FlowCatalystField", targets: ["FlowCatalystField"]),
    ],
    dependencies: [
        .package(url: "https://github.com/apple/swift-crypto.git", from: "3.0.0"),
    ],
    targets: [
        .target(name: "FlowCatalystFieldCore", dependencies: [cryptoOffApple]),
        .target(name: "FlowCatalystField", dependencies: ["FlowCatalystFieldCore"]),
        .testTarget(name: "FlowCatalystFieldCoreTests", dependencies: ["FlowCatalystFieldCore", cryptoOffApple]),
    ]
)
