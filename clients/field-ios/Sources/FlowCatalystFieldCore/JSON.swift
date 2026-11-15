import Foundation

typealias JObject = [String: Any]

enum JSON {
    static func parseObject(_ data: Data) -> JObject? {
        guard !data.isEmpty, let v = try? JSONSerialization.jsonObject(with: data, options: []) else { return nil }
        return v as? JObject
    }

    static func parseObject(_ text: String) -> JObject? { parseObject(Data(text.utf8)) }

    /// Compact JSON, sorted keys, no escaped slashes.
    static func data(_ obj: JObject) -> Data {
        let opts: JSONSerialization.WritingOptions = [.sortedKeys, .withoutEscapingSlashes]
        return (try? JSONSerialization.data(withJSONObject: obj, options: opts)) ?? Data("{}".utf8)
    }

    static func string(_ obj: JObject) -> String { String(decoding: data(obj), as: UTF8.self) }

    static func int64(_ v: Any?) -> Int64? {
        switch v {
        case let i as Int64: return i
        case let i as Int: return Int64(i)
        case let d as Double: return d.isFinite ? Int64(d) : nil
        case let n as NSNumber: return n.int64Value
        case let s as String: return Int64(s.trimmingCharacters(in: .whitespaces))
        default: return nil
        }
    }
}

/// `org.json`-style optional accessors, so a malformed stored or received value reads the way it does on Android.
extension Dictionary where Key == String, Value == Any {
    func has(_ k: String) -> Bool { self[k] != nil }

    /// "" when missing or null.
    func optString(_ k: String) -> String {
        switch self[k] {
        case let s as String: return s
        case is NSNull, nil: return ""
        case let n as NSNumber: return n.stringValue
        default: return ""
        }
    }

    func optLong(_ k: String) -> Int64 { JSON.int64(self[k]) ?? 0 }

    func optBool(_ k: String, _ fallback: Bool = false) -> Bool {
        switch self[k] {
        case let b as Bool: return b
        case let s as String:
            if s.lowercased() == "true" { return true }
            if s.lowercased() == "false" { return false }
            return fallback
        default: return fallback
        }
    }

    func optObject(_ k: String) -> JObject? { self[k] as? JObject }
    func optArray(_ k: String) -> [Any]? { self[k] as? [Any] }
    func isNull(_ k: String) -> Bool { self[k] == nil || self[k] is NSNull }
}
