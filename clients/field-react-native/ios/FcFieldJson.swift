import Foundation

/// Conversions between the JS shapes (field-js `Session` etc.) and field-ios models: the same shapes the Android
/// bridge builds (`SessionJson.kt`). Nothing here logs.
enum FcFieldJson {

    static func session(_ s: Session) -> [String: Any] {
        var actor: Any = NSNull()
        if let a = s.actor { actor = ["id": a.id, "type": a.type, "name": a.name] as [String: Any] }
        return [
            "worker": ["id": s.worker.id, "workerCode": s.worker.workerCode, "name": s.worker.name] as [String: Any],
            "actor": actor,
            "test": s.test,
            "deviceMode": s.deviceMode.rawValue,
            "deviceId": s.deviceId.map { $0 as Any } ?? NSNull(),
            "appCode": s.appCode,
            "expiresAt": Double(s.expiresAt),
            "idleExpiresAt": Double(s.idleExpiresAt),
            "refreshExpiresAt": Double(s.refreshExpiresAt),
            "pinChangeRequired": s.pinChangeRequired,
        ]
    }

    static func device(_ d: DeviceRecord) -> [String: Any] {
        ["deviceId": d.deviceId, "mode": d.mode, "clientId": d.clientId, "appCodes": d.appCodes, "platformUrl": d.platformUrl]
    }

    static func state(_ st: FieldState, device d: DeviceRecord?) -> [String: Any] {
        var m: [String: Any] = [:]
        switch st {
        case .signedOut:
            m["state"] = "SignedOut"
            m["session"] = NSNull()
        case .signedIn(let s):
            m["state"] = "SignedIn"
            m["session"] = session(s)
        case .locked(let s):
            m["state"] = "Locked"
            m["session"] = session(s)
        }
        m["device"] = d.map { device($0) as Any } ?? NSNull()
        return m
    }

    static func event(_ e: FieldEvent) -> (String, [String: Any]) {
        switch e {
        case .sessionChanged(let s):
            return ("SessionChanged", ["session": s.map { session($0) as Any } ?? NSNull()])
        case .sessionEnded(let reason):
            return ("SessionEnded", ["reason": reason])
        case .pinChangeRequired(let s):
            return ("PinChangeRequired", ["session": session(s)])
        case .expiryWarning(let remaining):
            return ("ExpiryWarning", ["remainingMs": (remaining * 1000).rounded()])
        case .locked(let s):
            return ("Locked", ["session": session(s)])
        case .workerLocked(let hard, let retryAfterSeconds):
            var m: [String: Any] = ["hard": hard]
            if let r = retryAfterSeconds { m["retryAfterSeconds"] = r }
            return ("WorkerLocked", m)
        }
    }

    // MARK: JS -> Swift

    private static func str(_ m: [String: Any], _ k: String) -> String? { m[k] as? String }

    static func deviceInfo(_ m: [String: Any]) -> DeviceInfo {
        let d = DeviceInfo.current
        return DeviceInfo(platform: str(m, "platform") ?? d.platform, model: str(m, "model") ?? d.model, label: str(m, "label"))
    }

    static func credentials(_ m: [String: Any]) -> LoginCredentials {
        LoginCredentials(workerCode: str(m, "workerCode"), badgeToken: str(m, "badgeToken"), pin: str(m, "pin"),
                         clientIdentifier: str(m, "clientIdentifier"))
    }

    static func actAsTarget(_ m: [String: Any]) throws -> ActAsTarget {
        guard let pin = str(m, "pin") else { throw FieldAuthError(code: "INVALID_REQUEST", message: "the supervisor PIN is required") }
        return ActAsTarget(workerCode: str(m, "workerCode"), workerId: str(m, "workerId"), pin: pin)
    }

    static func logoutScope(_ s: String) throws -> LogoutScope {
        switch s {
        case "session": return .session
        case "device-all": return .deviceAll
        default: throw FieldAuthError(code: "INVALID_REQUEST", message: "unknown logout scope")
        }
    }
}
