import Foundation

/// Turns the config object `FieldClient` passes to `configure` (the JS `FieldClientConfig`) into a field-ios
/// `FieldConfig`. The same mapping the Android bridge applies (`SessionJson.config`).
///
/// `FieldSession.get(config:)` refuses a config that differs from the one it was first given (`CONFIG_MISMATCH`).
/// Native code that runs without JS (a background location module after a relaunch) must therefore rebuild exactly
/// the config JS used: store the JSON object JS passed to `configure`, and call this function on it. Same input,
/// same `FieldConfig`.
public enum FieldBridgeConfig {
    /// - Parameter dictionary: `platformUrl` and `appCode` (strings, required); `clientIdentifier` (string);
    ///   `requireUnlock` (`{ afterColdStart: Bool, afterBackgroundMs: Number }`); `expiryWarningsMs` ([Number]);
    ///   `keepAliveLeadMs` (Number); `allowedPlatformOrigins` ([String]). Keys that are absent, null or of another
    ///   type fall back to the `FieldConfig` defaults. Durations are milliseconds here and seconds in `FieldConfig`.
    /// - Throws: `FieldAuthError` with code `INVALID_REQUEST` when `platformUrl` or `appCode` is missing.
    public static func fieldConfig(from dictionary: [String: Any]) throws -> FieldConfig {
        guard let platformUrl = string(dictionary["platformUrl"]) else {
            throw FieldAuthError(code: "INVALID_REQUEST", message: "platformUrl is required")
        }
        guard let appCode = string(dictionary["appCode"]) else {
            throw FieldAuthError(code: "INVALID_REQUEST", message: "appCode is required")
        }
        let defaults = FieldConfig(platformUrl: platformUrl, appCode: appCode)

        var unlock = UnlockPolicy.never
        if let u = dictionary["requireUnlock"] as? [String: Any] {
            unlock = UnlockPolicy(
                afterColdStart: bool(u["afterColdStart"]) ?? false,
                afterBackground: number(u["afterBackgroundMs"]).map { seconds(fromMilliseconds: $0) })
        }
        var warnings = defaults.expiryWarnings
        if let a = dictionary["expiryWarningsMs"] as? [Any] {
            warnings = a.compactMap { number($0) }.map { seconds(fromMilliseconds: $0) }
        }
        var origins: [String] = []
        if let a = dictionary["allowedPlatformOrigins"] as? [Any] {
            origins = a.compactMap { string($0) }
        }
        return FieldConfig(
            platformUrl: platformUrl,
            appCode: appCode,
            clientIdentifier: string(dictionary["clientIdentifier"]),
            requireUnlock: unlock,
            expiryWarnings: warnings,
            keepAliveLead: number(dictionary["keepAliveLeadMs"]).map { seconds(fromMilliseconds: $0) } ?? defaults.keepAliveLead,
            allowedPlatformOrigins: origins)
    }

    /// The same, from the JSON text of that object (for a config stored as JSON).
    /// - Throws: `FieldAuthError` `INVALID_REQUEST` when the data is not a JSON object, or as above.
    public static func fieldConfig(fromJSON data: Data) throws -> FieldConfig {
        guard let obj = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any] else {
            throw FieldAuthError(code: "INVALID_REQUEST", message: "the config is not a JSON object")
        }
        return try fieldConfig(from: obj)
    }

    // Whole milliseconds (the Android bridge truncates with `toLong()`), as seconds.
    private static func seconds(fromMilliseconds ms: Double) -> TimeInterval { Double(Int64(ms)) / 1000 }

    private static func string(_ v: Any?) -> String? { v as? String }

    private static func isBool(_ v: Any?) -> Bool {
        guard let n = v as? NSNumber else { return false }
        return CFGetTypeID(n) == CFBooleanGetTypeID()
    }

    private static func bool(_ v: Any?) -> Bool? { isBool(v) ? (v as? NSNumber)?.boolValue : nil }

    private static func number(_ v: Any?) -> Double? {
        guard let n = v as? NSNumber, !isBool(v) else { return nil }
        let d = n.doubleValue
        return d.isFinite ? d : nil
    }
}
