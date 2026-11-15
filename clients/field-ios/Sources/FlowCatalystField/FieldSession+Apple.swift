#if canImport(Security) && canImport(LocalAuthentication)
import Foundation
import FlowCatalystFieldCore
#if os(iOS)
import UIKit
#endif

private final class SessionRegistry: @unchecked Sendable {
    static let shared = SessionRegistry()
    let lock = NSLock()
    var instance: FieldSession?
    #if os(iOS)
    var visibility: AppVisibility?
    #endif
}

extension FieldSession {
    /// The process-wide owner (Kotlin `FieldSession.get(context, config)`). The first call configures it with the
    /// Secure Enclave key, the Keychain, LocalAuthentication and `URLSession`, and starts restoring the stored
    /// session in the background; later calls must pass an equal config, else `CONFIG_MISMATCH` is thrown.
    /// Call it from anywhere in the app's process: the tracking code and the UI must share this one object.
    public static func get(config: FieldConfig) throws -> FieldSession {
        let reg = SessionRegistry.shared
        reg.lock.lock()
        defer { reg.lock.unlock() }
        if let existing = reg.instance {
            if existing.config != config {
                throw FieldAuthError(code: "CONFIG_MISMATCH", message: "FieldSession is already configured with a different FieldConfig")
            }
            return existing
        }
        let s = FieldSession(
            config: config,
            storage: KeychainStorage(),
            keys: SecureEnclaveKeyProvider(),
            transport: URLSessionTransport(),
            clock: SystemClock(),
            unlockPrompter: LocalAuthenticationUnlocker())
        #if os(iOS)
        reg.visibility = AppVisibility(session: s)
        #endif
        Task { _ = try? await s.restore() }
        reg.instance = s
        return s
    }
}

extension DeviceInfo {
    /// The running device: platform `ios` (`macos` on a Mac), model from the kernel's machine identifier (for example
    /// `iPhone15,2`), no label.
    public static var current: DeviceInfo {
        var u = utsname()
        uname(&u)
        let machine = withUnsafePointer(to: &u.machine) {
            $0.withMemoryRebound(to: CChar.self, capacity: Int(_SYS_NAMELEN)) { String(cString: $0) }
        }
        let model = ProcessInfo.processInfo.environment["SIMULATOR_MODEL_IDENTIFIER"] ?? machine
        #if os(iOS)
        return DeviceInfo(platform: "ios", model: model)
        #else
        return DeviceInfo(platform: "macos", model: model)
        #endif
    }
}

#if os(iOS)
/// Reports the app going to the background and coming back (Kotlin `Visibility`, activity lifecycle callbacks).
/// Events are consumed in order by one task, so a quick background/foreground pair cannot be reordered.
final class AppVisibility: @unchecked Sendable {
    private var tokens: [NSObjectProtocol] = []
    private var task: Task<Void, Never>?

    init(session: FieldSession) {
        let (stream, cont) = AsyncStream<Bool>.makeStream()
        let nc = NotificationCenter.default
        tokens.append(nc.addObserver(forName: UIApplication.didEnterBackgroundNotification, object: nil, queue: nil) { _ in cont.yield(false) })
        tokens.append(nc.addObserver(forName: UIApplication.willEnterForegroundNotification, object: nil, queue: nil) { _ in cont.yield(true) })
        task = Task {
            for await foreground in stream {
                if foreground { await session.noteForegrounded() } else { await session.noteBackgrounded() }
            }
        }
    }

    deinit {
        for t in tokens { NotificationCenter.default.removeObserver(t) }
        task?.cancel()
    }
}
#endif
#endif
