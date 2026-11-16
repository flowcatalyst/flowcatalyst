import Foundation

/// ISO-8601 date-time with an offset (`2030-01-01T00:00:00Z`, `...00.123+02:00`) to epoch milliseconds
/// (Kotlin `OffsetDateTime.parse(s).toInstant().toEpochMilli()`). Nil when malformed.
func parseOffsetDateTimeMillis(_ s: String) -> Int64? {
    let u = Array(s.utf8)
    var i = 0
    func digits(_ n: Int) -> Int? {
        guard i + n <= u.count else { return nil }
        var v = 0
        for _ in 0..<n {
            let c = u[i]
            guard c >= 48 && c <= 57 else { return nil }
            v = v * 10 + Int(c - 48)
            i += 1
        }
        return v
    }
    func eat(_ ch: Character) -> Bool {
        guard i < u.count, u[i] == ch.asciiValue! else { return false }
        i += 1
        return true
    }
    guard let year = digits(4), eat("-"), let month = digits(2), eat("-"), let day = digits(2) else { return nil }
    guard i < u.count, u[i] == 84 || u[i] == 116 else { return nil } // T
    i += 1
    guard let hour = digits(2), eat(":"), let minute = digits(2) else { return nil }
    var second = 0
    var nanos = 0
    if eat(":") {
        guard let sec = digits(2) else { return nil }
        second = sec
        if eat(".") {
            var n = 0
            var count = 0
            while i < u.count, u[i] >= 48, u[i] <= 57 {
                if count < 9 { n = n * 10 + Int(u[i] - 48); count += 1 }
                i += 1
            }
            guard count > 0 else { return nil }
            for _ in count..<9 { n *= 10 }
            nanos = n
        }
    }
    var offsetSeconds = 0
    if eat("Z") || eat("z") {
        // UTC
    } else if i < u.count, u[i] == 43 || u[i] == 45 {
        let sign = u[i] == 45 ? -1 : 1
        i += 1
        guard let oh = digits(2) else { return nil }
        var om = 0
        if eat(":") { guard let m = digits(2) else { return nil }; om = m } else if let m = digits(2) { om = m }
        offsetSeconds = sign * (oh * 3600 + om * 60)
    } else {
        return nil
    }
    guard i == u.count, (1...12).contains(month), (1...31).contains(day), hour < 24, minute < 60, second < 60 else { return nil }
    let days = daysFromCivil(year, month, day)
    let seconds = Int64(days) * 86400 + Int64(hour * 3600 + minute * 60 + second) - Int64(offsetSeconds)
    return seconds * 1000 + Int64(nanos / 1_000_000)
}

/// Days since 1970-01-01 (Howard Hinnant's algorithm).
func daysFromCivil(_ y0: Int, _ m: Int, _ d: Int) -> Int {
    let y = m <= 2 ? y0 - 1 : y0
    let era = (y >= 0 ? y : y - 399) / 400
    let yoe = y - era * 400
    let doy = (153 * (m + (m > 2 ? -3 : 9)) + 2) / 5 + d - 1
    let doe = yoe * 365 + yoe / 4 - yoe / 100 + doy
    return era * 146097 + doe - 719468
}

/// Epoch milliseconds to `yyyy-MM-ddTHH:mm:ss.SSSZ` (used by tests; the platform sends RFC 3339).
func formatInstant(_ millis: Int64) -> String {
    let secs = millis >= 0 ? millis / 1000 : -((-millis + 999) / 1000)
    let ms = Int(millis - secs * 1000)
    let days = Int((secs >= 0 ? secs : secs - 86399) / 86400)
    let rem = Int(secs - Int64(days) * 86400)
    // civil from days
    let z = days + 719468
    let era = (z >= 0 ? z : z - 146096) / 146097
    let doe = z - era * 146097
    let yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365
    let y = yoe + era * 400
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100)
    let mp = (5 * doy + 2) / 153
    let d = doy - (153 * mp + 2) / 5 + 1
    let m = mp < 10 ? mp + 3 : mp - 9
    let year = m <= 2 ? y + 1 : y
    func p(_ v: Int, _ w: Int) -> String { String(repeating: "0", count: max(0, w - String(v).count)) + String(v) }
    return "\(p(year, 4))-\(p(m, 2))-\(p(d, 2))T\(p(rem / 3600, 2)):\(p(rem % 3600 / 60, 2)):\(p(rem % 60, 2)).\(p(ms, 3))Z"
}
