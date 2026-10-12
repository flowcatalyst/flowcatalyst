//! Verifying deliveries the platform signs, and answering them.
//!
//! The platform (the router's mediation call, the dispatch-processing
//! delivery and scheduled-job firings) signs `{timestamp}{body}` with
//! HMAC-SHA256 and sends:
//!
//! - `X-FlowCatalyst-Signature`: the lower-case hex digest (64 characters);
//! - `X-FlowCatalyst-Timestamp`: ISO-8601 UTC with milliseconds
//!   (`2026-05-24T08:30:00.123Z`). A bare Unix-seconds value is accepted for
//!   backward compatibility.
//!
//! The rules are the Java SDK's `WebhookSignature` (which the Go and
//! TypeScript verifiers share): an empty secret fails closed; the delivery may
//! be at most 300 s old (configurable) and 60 s in the future, compared in
//! whole seconds; the digest is compared in constant time, ignoring case; an
//! optional bearer token is AND-ed with the signature, never a substitute.
//! Verify the **raw** body bytes, before any JSON parsing.
//!
//! ```
//! use flowcatalyst_sdk::webhook::{WebhookVerifier, sign};
//!
//! let verifier = WebhookVerifier::new("secret");
//! let ts = chrono_free_now();
//! let sig = sign("secret", &ts, b"{\"id\":\"1\"}");
//! assert!(verifier.verify(b"{\"id\":\"1\"}", Some(&sig), Some(&ts)).is_ok());
//! # fn chrono_free_now() -> String {
//! #     let s = std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap().as_secs();
//! #     s.to_string()
//! # }
//! ```

use std::env;
use std::hint::black_box;
use std::time::{SystemTime, UNIX_EPOCH};

use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

/// Header carrying the hex HMAC-SHA256 signature (HTTP header names are
/// case-insensitive; the router writes `X-FLOWCATALYST-SIGNATURE`).
pub const SIGNATURE_HEADER: &str = "X-FlowCatalyst-Signature";
/// Header carrying the signed timestamp.
pub const TIMESTAMP_HEADER: &str = "X-FlowCatalyst-Timestamp";
/// Header naming the tenant of a delivery: `{clientId}:{clientCode}`.
pub const CLIENT_HEADER: &str = "X-FlowCatalyst-Client";
/// The dispatch job's id on a dispatch-processing delivery.
pub const DISPATCH_JOB_ID_HEADER: &str = "X-Dispatch-Job-Id";
/// The dispatch job's code (event type) on a dispatch-processing delivery.
pub const EVENT_TYPE_HEADER: &str = "X-Event-Type";
/// Environment variable [`WebhookVerifier::from_env`] reads.
pub const SECRET_ENV: &str = "FLOWCATALYST_SIGNING_SECRET";

/// Default replay tolerance: a delivery older than this is refused.
pub const DEFAULT_TOLERANCE_SECS: u64 = 300;
/// How far in the future a timestamp may be (clock skew).
pub const FUTURE_GRACE_SECS: u64 = 60;

/// Why a delivery was refused. Messages never echo a header value or a secret.
#[derive(Debug, Clone, Copy, PartialEq, Eq, thiserror::Error)]
#[non_exhaustive]
pub enum WebhookError {
    #[error("webhook signing secret is not configured")]
    MissingSecret,
    #[error("missing {SIGNATURE_HEADER} header")]
    MissingSignature,
    #[error("missing {TIMESTAMP_HEADER} header")]
    MissingTimestamp,
    #[error("unparseable {TIMESTAMP_HEADER} header")]
    InvalidTimestamp,
    #[error("delivery timestamp is older than the replay tolerance")]
    TimestampExpired,
    #[error("delivery timestamp is in the future; check clock sync")]
    TimestampInFuture,
    #[error("signature mismatch")]
    InvalidSignature,
    #[error("missing Authorization bearer token")]
    MissingBearer,
    #[error("invalid bearer token")]
    InvalidBearer,
}

impl WebhookError {
    /// The upper-snake code the Java SDK's `ErrorCode` uses for this refusal.
    pub const fn code(self) -> &'static str {
        match self {
            WebhookError::MissingSecret => "MISSING_SECRET",
            WebhookError::MissingSignature => "MISSING_SIGNATURE",
            WebhookError::MissingTimestamp => "MISSING_TIMESTAMP",
            WebhookError::InvalidTimestamp => "INVALID_TIMESTAMP",
            WebhookError::TimestampExpired => "TIMESTAMP_EXPIRED",
            WebhookError::TimestampInFuture => "TIMESTAMP_IN_FUTURE",
            WebhookError::InvalidSignature => "INVALID_SIGNATURE",
            WebhookError::MissingBearer => "MISSING_BEARER",
            WebhookError::InvalidBearer => "INVALID_BEARER",
        }
    }
}

/// Verifies signed deliveries.
#[derive(Clone)]
pub struct WebhookVerifier {
    secret: Vec<u8>,
    tolerance_secs: u64,
    bearer_token: Option<Vec<u8>>,
    now: fn() -> u64,
}

impl std::fmt::Debug for WebhookVerifier {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("WebhookVerifier")
            .field("secret", &"<redacted>")
            .field("tolerance_secs", &self.tolerance_secs)
            .field(
                "bearer_token",
                &self.bearer_token.as_ref().map(|_| "<redacted>"),
            )
            .finish()
    }
}

impl WebhookVerifier {
    /// A verifier for this signing secret. An empty secret is allowed here and
    /// refuses every delivery with [`WebhookError::MissingSecret`].
    pub fn new(secret: impl AsRef<[u8]>) -> Self {
        Self {
            secret: secret.as_ref().to_vec(),
            tolerance_secs: DEFAULT_TOLERANCE_SECS,
            bearer_token: None,
            now: unix_now,
        }
    }

    /// A verifier for `FLOWCATALYST_SIGNING_SECRET`.
    pub fn from_env() -> Result<Self, WebhookError> {
        env::var(SECRET_ENV)
            .ok()
            .filter(|s| !s.is_empty())
            .map(Self::new)
            .ok_or(WebhookError::MissingSecret)
    }

    /// The replay tolerance in seconds (default 300).
    pub fn with_tolerance(mut self, seconds: u64) -> Self {
        self.tolerance_secs = seconds;
        self
    }

    /// Also require `Authorization: Bearer <token>` (AND-ed with the
    /// signature). An empty token leaves the gate off.
    pub fn with_bearer_token(mut self, token: impl AsRef<[u8]>) -> Self {
        let token = token.as_ref();
        self.bearer_token = (!token.is_empty()).then(|| token.to_vec());
        self
    }

    #[cfg(test)]
    fn with_clock(mut self, now: fn() -> u64) -> Self {
        self.now = now;
        self
    }

    /// Verify a delivery's signature and timestamp.
    ///
    /// `body` must be the raw bytes received. An absent or empty header is
    /// "missing".
    pub fn verify(
        &self,
        body: &[u8],
        signature: Option<&str>,
        timestamp: Option<&str>,
    ) -> Result<(), WebhookError> {
        self.verify_with_authorization(body, signature, timestamp, None)
    }

    /// [`verify`](Self::verify) plus the bearer gate, when one is configured.
    pub fn verify_with_authorization(
        &self,
        body: &[u8],
        signature: Option<&str>,
        timestamp: Option<&str>,
        authorization: Option<&str>,
    ) -> Result<(), WebhookError> {
        if self.secret.is_empty() {
            return Err(WebhookError::MissingSecret);
        }
        let signature = signature
            .filter(|s| !s.is_empty())
            .ok_or(WebhookError::MissingSignature)?;
        let timestamp = timestamp
            .filter(|s| !s.is_empty())
            .ok_or(WebhookError::MissingTimestamp)?;

        let ts = parse_timestamp(timestamp).ok_or(WebhookError::InvalidTimestamp)?;
        let now = i128::from((self.now)());
        if ts < now - i128::from(self.tolerance_secs) {
            return Err(WebhookError::TimestampExpired);
        }
        if ts > now + i128::from(FUTURE_GRACE_SECS) {
            return Err(WebhookError::TimestampInFuture);
        }

        let expected = hmac_sha256(&self.secret, &[timestamp.as_bytes(), body]);
        let presented = signature.to_ascii_lowercase();
        if !constant_time_eq(to_hex(&expected).as_bytes(), presented.as_bytes()) {
            return Err(WebhookError::InvalidSignature);
        }

        if let Some(want) = &self.bearer_token {
            let presented = authorization
                .and_then(|a| a.strip_prefix("Bearer "))
                .ok_or(WebhookError::MissingBearer)?;
            // Digests, so the comparison does not reveal the token's length.
            let a = Sha256::digest(presented.as_bytes());
            let b = Sha256::digest(want);
            if !constant_time_eq(&a, &b) {
                return Err(WebhookError::InvalidBearer);
            }
        }
        Ok(())
    }
}

/// `hex(HMAC-SHA256(secret, timestamp || body))`, lower case: what the
/// platform sends as the signature. Exposed for tests and for signing
/// callbacks with the same scheme.
pub fn sign(secret: impl AsRef<[u8]>, timestamp: &str, body: &[u8]) -> String {
    to_hex(&hmac_sha256(secret.as_ref(), &[timestamp.as_bytes(), body]))
}

/// The tenant a delivery is for, from `X-FlowCatalyst-Client: {id}:{code}`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DeliveryClient {
    /// The platform client id (`clt_…`).
    pub id: String,
    /// The client's identifier (its code).
    pub code: String,
}

/// Parse `X-FlowCatalyst-Client`. `None` unless the value is exactly
/// `{id}:{code}` with each part `[A-Za-z0-9._-]{1,100}` (the platform never
/// sends half a pair).
pub fn parse_client_header(value: &str) -> Option<DeliveryClient> {
    let (id, code) = value.split_once(':')?;
    let ok = |s: &str| {
        (1..=100).contains(&s.len())
            && s.bytes()
                .all(|b| b.is_ascii_alphanumeric() || matches!(b, b'.' | b'_' | b'-'))
    };
    (ok(id) && ok(code)).then(|| DeliveryClient {
        id: id.to_owned(),
        code: code.to_owned(),
    })
}

/// The JSON a mediation target may answer a delivery with (docs/wire-contract.md,
/// "Mediation response"). An empty 2xx is plain success; build one of these
/// only to defer or to flush the message group.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MediationResponse {
    /// `false`: not delivered, requeue with backoff. Absent means `true`.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ack: Option<bool>,
    /// With `ack: false` the minimum requeue delay; with `flush_group` the
    /// suppression window.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub delay_seconds: Option<u32>,
    /// Delivered, and stop delivering this message's group (only safe for
    /// targets that own the records the messages point at).
    #[serde(skip_serializing_if = "Option::is_none")]
    pub flush_group: Option<bool>,
}

impl MediationResponse {
    /// `{"ack":true}`.
    pub const fn ack() -> Self {
        Self {
            ack: Some(true),
            delay_seconds: None,
            flush_group: None,
        }
    }

    /// `{"ack":false,"delaySeconds":n}`: requeue no sooner than `n` seconds.
    pub const fn defer(delay_seconds: u32) -> Self {
        Self {
            ack: Some(false),
            delay_seconds: Some(delay_seconds),
            flush_group: None,
        }
    }

    /// `{"ack":true,"flushGroup":true[,"delaySeconds":n]}`.
    pub const fn flush_group(delay_seconds: Option<u32>) -> Self {
        Self {
            ack: Some(true),
            delay_seconds,
            flush_group: Some(true),
        }
    }
}

/// Unix seconds of a timestamp header: ASCII digits only for the Unix form
/// (no sign), otherwise RFC 3339 / ISO-8601 with an offset or `Z`.
fn parse_timestamp(s: &str) -> Option<i128> {
    if s.len() > 64 {
        return None;
    }
    if s.bytes().all(|b| b.is_ascii_digit()) {
        return s.parse::<u64>().ok().map(i128::from);
    }
    parse_rfc3339_seconds(s)
}

/// A small RFC 3339 parser (`YYYY-MM-DDTHH:MM:SS[.frac](Z|±HH:MM)`), so this
/// module needs no date library. Returns whole Unix seconds (fraction dropped,
/// as every SDK compares in whole seconds).
fn parse_rfc3339_seconds(s: &str) -> Option<i128> {
    let b = s.as_bytes();
    if b.len() < 20 {
        return None;
    }
    let num = |from: usize, len: usize| -> Option<i64> {
        let part = b.get(from..from + len)?;
        if !part.iter().all(u8::is_ascii_digit) {
            return None;
        }
        std::str::from_utf8(part).ok()?.parse().ok()
    };
    let sep = |i: usize, c: u8| b.get(i) == Some(&c);
    if !(sep(4, b'-')
        && sep(7, b'-')
        && (sep(10, b'T') || sep(10, b't'))
        && sep(13, b':')
        && sep(16, b':'))
    {
        return None;
    }
    let (year, month, day) = (num(0, 4)?, num(5, 2)?, num(8, 2)?);
    let (hour, minute, second) = (num(11, 2)?, num(14, 2)?, num(17, 2)?);
    if !(1..=12).contains(&month)
        || day < 1
        || day > days_in_month(year, month)
        || hour > 23
        || minute > 59
        || second > 60
    {
        return None;
    }
    let mut i = 19;
    if sep(i, b'.') {
        i += 1;
        let start = i;
        while b.get(i).is_some_and(u8::is_ascii_digit) {
            i += 1;
        }
        if i == start {
            return None;
        }
    }
    let offset_secs = match b.get(i) {
        Some(b'Z' | b'z') if i + 1 == b.len() => 0,
        Some(&sign @ (b'+' | b'-')) if i + 6 == b.len() && sep(i + 3, b':') => {
            let (oh, om) = (num(i + 1, 2)?, num(i + 4, 2)?);
            if oh > 23 || om > 59 {
                return None;
            }
            let o = oh * 3600 + om * 60;
            if sign == b'+' { o } else { -o }
        }
        _ => return None,
    };
    let days = days_from_civil(year, month, day);
    Some(
        i128::from(days) * 86_400 + i128::from(hour * 3600 + minute * 60 + second)
            - i128::from(offset_secs),
    )
}

fn days_in_month(year: i64, month: i64) -> i64 {
    match month {
        2 if (year % 4 == 0 && year % 100 != 0) || year % 400 == 0 => 29,
        2 => 28,
        4 | 6 | 9 | 11 => 30,
        _ => 31,
    }
}

/// Days since 1970-01-01 (Howard Hinnant's `days_from_civil`).
fn days_from_civil(y: i64, m: i64, d: i64) -> i64 {
    let y = if m <= 2 { y - 1 } else { y };
    let era = y.div_euclid(400);
    let yoe = y - era * 400;
    let mp = (m + 9) % 12;
    let doy = (153 * mp + 2) / 5 + d - 1;
    let doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    era * 146_097 + doe - 719_468
}

fn unix_now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

/// HMAC-SHA256 (RFC 2104) over the concatenation of `parts`.
fn hmac_sha256(key: &[u8], parts: &[&[u8]]) -> [u8; 32] {
    const BLOCK: usize = 64;
    let mut block_key = [0u8; BLOCK];
    if key.len() > BLOCK {
        block_key[..32].copy_from_slice(&Sha256::digest(key));
    } else {
        block_key[..key.len()].copy_from_slice(key);
    }
    let mut inner = Sha256::new();
    inner.update(block_key.map(|b| b ^ 0x36));
    for part in parts {
        inner.update(part);
    }
    let mut outer = Sha256::new();
    outer.update(block_key.map(|b| b ^ 0x5c));
    outer.update(inner.finalize());
    outer.finalize().into()
}

fn to_hex(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(bytes.len() * 2);
    for &b in bytes {
        out.push(char::from(HEX[usize::from(b >> 4)]));
        out.push(char::from(HEX[usize::from(b & 0x0f)]));
    }
    out
}

fn constant_time_eq(a: &[u8], b: &[u8]) -> bool {
    if a.len() != b.len() {
        return false;
    }
    let mut diff = 0u8;
    for (x, y) in a.iter().zip(b) {
        diff |= black_box(x ^ y);
    }
    diff == 0
}

#[cfg(test)]
mod tests {
    use super::*;

    const NOW: u64 = 1_786_095_132; // 2026-08-07T09:32:12Z
    fn fixed_now() -> u64 {
        NOW
    }
    fn verifier(secret: &str) -> WebhookVerifier {
        WebhookVerifier::new(secret).with_clock(fixed_now)
    }

    /// The vector the Java, TypeScript and Laravel SDKs pin
    /// (`WebhookSignatureTest.knownVectorMatchesOtherSdks`).
    #[test]
    fn shared_vector_matches_the_other_sdks() {
        assert_eq!(
            sign("secret", "2026-08-07T09:32:12.123Z", b"hello"),
            "a7d62aade7a79e88792f84e035b72788a3c78a30f46ad265ec8d6fb12a4e9f91"
        );
    }

    /// RFC 4231 test case 6: a key longer than the block size.
    #[test]
    fn hmac_matches_rfc_4231_long_key() {
        let key = [0xaau8; 131];
        let mac = hmac_sha256(
            &key,
            &[b"Test Using Larger Than Block-Size Key - Hash Key First"],
        );
        assert_eq!(
            to_hex(&mac),
            "60e431591ee0b67f0d8a26aacbf5b77f8e0bc6213728c5140546040f0ee37f54"
        );
    }

    #[test]
    fn accepts_the_router_format_and_upper_case_hex() {
        let ts = "2026-08-07T09:32:12.123Z";
        let sig = sign("s3", ts, b"{}");
        let v = verifier("s3");
        assert_eq!(v.verify(b"{}", Some(&sig), Some(ts)), Ok(()));
        assert_eq!(
            v.verify(b"{}", Some(&sig.to_ascii_uppercase()), Some(ts)),
            Ok(())
        );
    }

    #[test]
    fn accepts_unix_seconds_and_offsets() {
        let v = verifier("s3");
        for ts in [
            NOW.to_string(),
            "2026-08-07T11:32:12+02:00".to_string(),
            "2026-08-07T09:32:12.123456789Z".to_string(),
        ] {
            let sig = sign("s3", &ts, b"x");
            assert_eq!(v.verify(b"x", Some(&sig), Some(&ts)), Ok(()), "{ts}");
        }
    }

    #[test]
    fn window_edges_are_inclusive_in_whole_seconds() {
        let v = verifier("s");
        let at = |secs: u64| {
            let ts = secs.to_string();
            v.verify(b"", Some(&sign("s", &ts, b"")), Some(&ts))
        };
        assert_eq!(at(NOW - 300), Ok(()));
        assert_eq!(at(NOW - 301), Err(WebhookError::TimestampExpired));
        assert_eq!(at(NOW + 60), Ok(()));
        assert_eq!(at(NOW + 61), Err(WebhookError::TimestampInFuture));
    }

    #[test]
    fn refusals_have_the_java_codes() {
        let ts = NOW.to_string();
        let sig = sign("s", &ts, b"body");
        let v = verifier("s");
        assert_eq!(
            verifier("").verify(b"body", Some(&sig), Some(&ts)),
            Err(WebhookError::MissingSecret)
        );
        assert_eq!(
            v.verify(b"body", None, Some(&ts)),
            Err(WebhookError::MissingSignature)
        );
        assert_eq!(
            v.verify(b"body", Some(""), Some(&ts)),
            Err(WebhookError::MissingSignature)
        );
        assert_eq!(
            v.verify(b"body", Some(&sig), Some("")),
            Err(WebhookError::MissingTimestamp)
        );
        assert_eq!(
            v.verify(b"tampered", Some(&sig), Some(&ts)),
            Err(WebhookError::InvalidSignature)
        );
        assert_eq!(WebhookError::InvalidSignature.code(), "INVALID_SIGNATURE");
    }

    #[test]
    fn unix_seconds_must_be_plain_digits() {
        let v = verifier("s");
        for ts in [
            "+1786095132",
            "-5",
            " 1786095132",
            "1786095132.5",
            "not-a-time",
        ] {
            let sig = sign("s", ts, b"");
            assert_eq!(
                v.verify(b"", Some(&sig), Some(ts)),
                Err(WebhookError::InvalidTimestamp),
                "{ts}"
            );
        }
        let long = "1".repeat(65);
        assert_eq!(
            v.verify(b"", Some(&sign("s", &long, b"")), Some(&long)),
            Err(WebhookError::InvalidTimestamp)
        );
    }

    #[test]
    fn rejects_malformed_iso_timestamps() {
        for ts in [
            "2026-02-30T00:00:00Z",
            "2026-08-07 09:32:12Z",
            "2026-08-07T09:32:12",
            "2026-08-07T09:32:12.Z",
            "2026-08-07T25:00:00Z",
        ] {
            assert_eq!(parse_timestamp(ts), None, "{ts}");
        }
        assert_eq!(
            parse_timestamp("2026-08-07T09:32:12.123Z"),
            Some(i128::from(NOW))
        );
        assert_eq!(parse_timestamp("1970-01-01T00:00:00Z"), Some(0));
    }

    #[test]
    fn bearer_gate_is_anded_with_the_signature() {
        let ts = NOW.to_string();
        let sig = sign("s", &ts, b"b");
        let v = verifier("s").with_bearer_token("tok-123");
        assert_eq!(
            v.verify_with_authorization(b"b", Some(&sig), Some(&ts), Some("Bearer tok-123")),
            Ok(())
        );
        assert_eq!(
            v.verify_with_authorization(b"b", Some(&sig), Some(&ts), None),
            Err(WebhookError::MissingBearer)
        );
        assert_eq!(
            v.verify_with_authorization(b"b", Some(&sig), Some(&ts), Some("Bearer wrong")),
            Err(WebhookError::InvalidBearer)
        );
        assert_eq!(
            v.verify_with_authorization(
                b"b",
                Some(&"0".repeat(64)),
                Some(&ts),
                Some("Bearer tok-123")
            ),
            Err(WebhookError::InvalidSignature)
        );
    }

    #[test]
    fn client_header_is_a_full_pair_or_nothing() {
        assert_eq!(
            parse_client_header("clt_0HZXEQ5Y8JY5Z:acme"),
            Some(DeliveryClient {
                id: "clt_0HZXEQ5Y8JY5Z".into(),
                code: "acme".into()
            })
        );
        assert_eq!(parse_client_header("clt_1:"), None);
        assert_eq!(parse_client_header(":acme"), None);
        assert_eq!(parse_client_header("clt_1"), None);
        assert_eq!(parse_client_header("clt 1:acme"), None);
    }

    #[test]
    fn mediation_responses_serialise_as_the_contract() {
        let s = |r: MediationResponse| serde_json::to_string(&r).unwrap();
        assert_eq!(s(MediationResponse::ack()), r#"{"ack":true}"#);
        assert_eq!(
            s(MediationResponse::defer(30)),
            r#"{"ack":false,"delaySeconds":30}"#
        );
        assert_eq!(
            s(MediationResponse::flush_group(Some(60))),
            r#"{"ack":true,"delaySeconds":60,"flushGroup":true}"#
        );
        assert_eq!(s(MediationResponse::default()), "{}");
    }

    #[test]
    fn debug_never_prints_the_secret() {
        let dbg = format!(
            "{:?}",
            WebhookVerifier::new("hunter2").with_bearer_token("tok")
        );
        assert!(!dbg.contains("hunter2") && !dbg.contains("tok\""));
    }
}
