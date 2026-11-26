import Foundation

/// Conversions between the JS shapes (field-js `Session` etc.) and field-ios models: the same shapes the Android
/// bridge builds (`SessionJson.kt`). Nothing here logs.
enum FcFieldJson {

    /// `{ photo: NSNull }` or `{ photo: { bytes: base64url | NSNull, contentType, version, notModified } }`.
    static func photo(_ p: WorkerPhoto?) -> [String: Any] {
        guard let p else { return ["photo": NSNull()] }
        return ["photo": [
            "bytes": p.bytes.map { Base64URL.encode($0) as Any } ?? NSNull(),
            "contentType": p.contentType.map { $0 as Any } ?? NSNull(),
            "version": p.version.map { $0 as Any } ?? NSNull(),
            "notModified": p.notModified,
        ] as [String: Any]]
    }

    static func session(_ s: Session) -> [String: Any] {
        var actor: Any = NSNull()
        if let a = s.actor { actor = ["id": a.id, "type": a.type, "name": a.name] as [String: Any] }
        return [
            "worker": [
                "id": s.worker.id, "workerCode": s.worker.workerCode, "name": s.worker.name,
                "photoVersion": s.worker.photoVersion.map { $0 as Any } ?? NSNull(), "canSetOwnPhoto": s.worker.canSetOwnPhoto,
            ] as [String: Any],
            "actor": actor,
            "test": s.test,
            "deviceMode": s.deviceMode.rawValue,
            "deviceId": s.deviceId.map { $0 as Any } ?? NSNull(),
            "appCode": s.appCode,
            "expiresAt": Double(s.expiresAt),
            "idleExpiresAt": Double(s.idleExpiresAt),
            "refreshExpiresAt": Double(s.refreshExpiresAt),
            "pinChangeRequired": s.pinChangeRequired,
            "clientId": s.clientId.map { $0 as Any } ?? NSNull(),
            "clientIdentifier": s.clientIdentifier.map { $0 as Any } ?? NSNull(),
            "amr": s.amr,
            "unlockKeys": s.unlockKeys.map { unlockKeys($0) as Any } ?? NSNull(),
        ]
    }

    /// `{allowed, attestationRequired, registerExpiresAt: ms number | null}` (Android's `SessionJson` shape).
    static func unlockKeys(_ k: SessionUnlockKeys) -> [String: Any] {
        [
            "allowed": k.allowed, "attestationRequired": k.attestationRequired,
            "registerExpiresAt": k.registerExpiresAt.map { Double($0) as Any } ?? NSNull(),
        ]
    }

    /// `UnlockKeyInfo` as the Android bridge builds it: times are ms numbers, `pinRequiredBy` null when there is none.
    static func unlockKey(_ k: UnlockKeyInfo) -> [String: Any] {
        [
            "unlockKeyId": k.unlockKeyId, "workerId": k.workerId, "workerCode": k.workerCode, "workerName": k.workerName,
            "deviceId": k.deviceId, "createdAt": Double(k.createdAt),
            "pinRequiredBy": k.pinRequiredBy.map { Double($0) as Any } ?? NSNull(),
            "attestationVerdict": k.attestationVerdict,
        ]
    }

    /// `{ key: UnlockKeyInfo | null }`.
    static func unlockKeyOrNull(_ k: UnlockKeyInfo?) -> [String: Any] {
        ["key": k.map { unlockKey($0) as Any } ?? NSNull()]
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
        case .unlockKeyChanged(let k):
            return ("UnlockKeyChanged", unlockKeyOrNull(k))
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

    /// `{ title, cancel, subtitle? }`; title and cancel are required (the library has no words of its own).
    static func unlockKeyPrompt(_ m: [String: Any]) throws -> UnlockKeyPrompt {
        guard let title = str(m, "title"), !title.isEmpty, let cancel = str(m, "cancel"), !cancel.isEmpty else {
            throw FieldAuthError(code: "INVALID_REQUEST", message: "the prompt needs a title and a cancel label")
        }
        return UnlockKeyPrompt(title: title, cancel: cancel, subtitle: str(m, "subtitle"))
    }

    static func logoutScope(_ s: String) throws -> LogoutScope {
        switch s {
        case "session": return .session
        case "device-all": return .deviceAll
        default: throw FieldAuthError(code: "INVALID_REQUEST", message: "unknown logout scope")
        }
    }
}
