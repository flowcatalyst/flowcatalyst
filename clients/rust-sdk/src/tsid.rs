//! Time-sorted ids (TSIDs) as 13-character Crockford Base32 strings.
//!
//! The layout and the encoding are those of every FlowCatalyst implementation
//! (Go `pkg/fcsdk/tsid`, the Java SDK's `Tsid`, the platform), so an id minted
//! here is interchangeable with one minted there:
//!
//! ```text
//! bits 63..22  timestamp (42 bits, milliseconds since the Unix epoch)
//! bits 21..10  sequence  (12 bits; random start each millisecond, then +1)
//! bits  9..0   random    (10 bits, from the operating system's CSPRNG)
//! ```
//!
//! Within one process ids are strictly increasing: the `(millisecond,
//! sequence)` pair only moves forward, and more than 4096 ids in one
//! millisecond borrow the next millisecond instead of repeating a sequence.
//! A typed id is `{prefix}_{tsid}`, e.g. `clt_0HZXEQ5Y8JY5Z`.
//!
//! ```
//! use flowcatalyst_sdk::tsid::{self, EntityType, Tsid};
//!
//! let id = Tsid::generate();
//! assert_eq!(id.to_string().len(), 13);
//! assert_eq!(id.to_string().parse::<Tsid>().unwrap(), id);
//!
//! let client = tsid::generate(EntityType::Client);
//! assert!(client.starts_with("clt_"));
//! ```

use std::fmt;
use std::str::FromStr;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{SystemTime, UNIX_EPOCH};

/// Crockford Base32 alphabet (no I, L, O, U).
const ALPHABET: &[u8; 32] = b"0123456789ABCDEFGHJKMNPQRSTVWXYZ";

/// The last `(millisecond << 12) | sequence` handed out (Go `nextMsSeq`).
static STATE: AtomicU64 = AtomicU64::new(0);

/// A raw TSID: the 64-bit value behind a 13-character Crockford Base32 id.
///
/// `Display` renders the canonical upper-case form; `FromStr` accepts upper or
/// lower case and refuses anything that is not exactly 13 Crockford characters
/// or that would not fit in 64 bits.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub struct Tsid(u64);

impl Tsid {
    /// Mint a new id from the process-wide monotonic generator.
    pub fn generate() -> Self {
        let (ms, seq) = next_ms_seq();
        let random = u64::from(random_u16() & 0x3FF);
        Tsid(((ms & 0x3FF_FFFF_FFFF) << 22) | ((seq & 0xFFF) << 10) | random)
    }

    /// The id with this numeric value.
    pub const fn from_u64(value: u64) -> Self {
        Tsid(value)
    }

    /// The numeric value.
    pub const fn as_u64(self) -> u64 {
        self.0
    }

    /// Milliseconds since the Unix epoch encoded in the id.
    pub const fn timestamp_millis(self) -> u64 {
        self.0 >> 22
    }
}

impl fmt::Display for Tsid {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(&encode_crockford(self.0))
    }
}

/// A string that is not a 13-character Crockford Base32 TSID.
#[derive(Debug, Clone, PartialEq, Eq, thiserror::Error)]
#[error("not a 13-character Crockford Base32 TSID: {0:?}")]
pub struct InvalidTsid(pub String);

impl FromStr for Tsid {
    type Err = InvalidTsid;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        decode_crockford(s)
            .map(Tsid)
            .ok_or_else(|| InvalidTsid(s.to_owned()))
    }
}

/// Well-known entity prefixes of the FlowCatalyst platform (a superset of the
/// Go SDK's list; the shared ones are identical). Use [`generate`] with one of
/// these, or [`generate_with_prefix`] for an application's own entity.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
#[non_exhaustive]
pub enum EntityType {
    Client,
    Principal,
    Application,
    ServiceAccount,
    Role,
    Permission,
    OAuthClient,
    AuthCode,
    LoginAttempt,
    ClientAuthConfig,
    AppClientConfig,
    IdpRoleMapping,
    CorsOrigin,
    AnchorDomain,
    IdentityProvider,
    EmailDomainMapping,
    ClientAccessGrant,
    EventType,
    Event,
    EventRead,
    Connection,
    Subscription,
    DispatchPool,
    DispatchJob,
    DispatchJobRead,
    Schema,
    AuditLog,
    PlatformConfig,
    ConfigAccess,
    PasswordResetToken,
    WebauthnCredential,
    ScheduledJob,
    ScheduledJobInstance,
    ScheduledJobInstanceLog,
    ApplicationOpenApiSpec,
    Process,
    Function,
    FunctionVersion,
    FunctionDomain,
    FunctionRoute,
    /// Portal identity (Go `PortalUser`).
    PortalIdentity,
    PortalApp,
    MfaMethod,
    MfaRecoveryCode,
    MfaEmailPin,
    MfaTrustedDevice,
}

impl EntityType {
    /// The 3-character prefix for this entity type.
    pub const fn prefix(self) -> &'static str {
        match self {
            EntityType::Client => "clt",
            EntityType::Principal => "prn",
            EntityType::Application => "app",
            EntityType::ServiceAccount => "sac",
            EntityType::Role => "rol",
            EntityType::Permission => "prm",
            EntityType::OAuthClient => "oac",
            EntityType::AuthCode => "acd",
            EntityType::LoginAttempt => "lat",
            EntityType::ClientAuthConfig => "cac",
            EntityType::AppClientConfig => "apc",
            EntityType::IdpRoleMapping => "irm",
            EntityType::CorsOrigin => "cor",
            EntityType::AnchorDomain => "anc",
            EntityType::IdentityProvider => "idp",
            EntityType::EmailDomainMapping => "edm",
            EntityType::ClientAccessGrant => "gnt",
            EntityType::EventType => "evt",
            EntityType::Event => "evn",
            EntityType::EventRead => "evr",
            EntityType::Connection => "con",
            EntityType::Subscription => "sub",
            EntityType::DispatchPool => "dpl",
            EntityType::DispatchJob => "djb",
            EntityType::DispatchJobRead => "djr",
            EntityType::Schema => "sch",
            EntityType::AuditLog => "aud",
            EntityType::PlatformConfig => "pcf",
            EntityType::ConfigAccess => "cfa",
            EntityType::PasswordResetToken => "prt",
            EntityType::WebauthnCredential => "pkc",
            EntityType::ScheduledJob => "sjb",
            EntityType::ScheduledJobInstance => "sji",
            EntityType::ScheduledJobInstanceLog => "sjl",
            EntityType::ApplicationOpenApiSpec => "oas",
            EntityType::Process => "prc",
            EntityType::Function => "fnc",
            EntityType::FunctionVersion => "fnv",
            EntityType::FunctionDomain => "fnd",
            EntityType::FunctionRoute => "fnr",
            EntityType::PortalIdentity => "ptu",
            EntityType::PortalApp => "pta",
            EntityType::MfaMethod => "mfm",
            EntityType::MfaRecoveryCode => "mrc",
            EntityType::MfaEmailPin => "mep",
            EntityType::MfaTrustedDevice => "mtd",
        }
    }
}

/// A typed id with a platform entity prefix: `{prefix}_{tsid}`.
pub fn generate(entity_type: EntityType) -> String {
    generate_with_prefix(entity_type.prefix())
}

/// A typed id with an application-chosen prefix: `{prefix}_{tsid}`.
pub fn generate_with_prefix(prefix: &str) -> String {
    format!("{prefix}_{}", Tsid::generate())
}

/// A raw 13-character id (event ids, outbox row ids, execution ids).
pub fn generate_untyped() -> String {
    Tsid::generate().to_string()
}

/// The numeric value of a raw (`0HZXEQ5Y8JY5Z`) or typed (`clt_0HZXEQ5Y8JY5Z`)
/// id; `None` when the TSID part is malformed. A typed id's TSID is the part
/// after its last `_`, so a prefix may itself contain underscores.
pub fn to_long(id: &str) -> Option<i64> {
    let raw = match id.rsplit_once('_') {
        Some((_, raw)) => raw,
        None => id,
    };
    decode_crockford(raw).map(|v| v as i64)
}

/// The raw string form of a numeric TSID (no prefix).
pub fn from_long(value: i64) -> String {
    encode_crockford(value as u64)
}

/// The next `(millisecond, sequence)` pair: a new millisecond starts at a
/// random sequence; within one the sequence increments; an exhausted sequence
/// borrows the next millisecond. The state only moves forward, so a wall
/// clock stepping back cannot repeat an id.
fn next_ms_seq() -> (u64, u64) {
    loop {
        let now = now_millis();
        let old = STATE.load(Ordering::SeqCst);
        let last_ms = old >> 12;
        let last_seq = old & 0xFFF;
        let (ms, seq) = if now > last_ms {
            (now, u64::from(random_u16() & 0xFFF))
        } else if last_seq < 0xFFF {
            (last_ms, last_seq + 1)
        } else {
            (last_ms + 1, u64::from(random_u16() & 0xFFF))
        };
        if STATE
            .compare_exchange(old, (ms << 12) | seq, Ordering::SeqCst, Ordering::SeqCst)
            .is_ok()
        {
            return (ms, seq);
        }
    }
}

/// Milliseconds since the epoch; a clock before 1970 counts as 0, which the
/// forward-only state then ignores.
fn now_millis() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| u64::try_from(d.as_millis()).unwrap_or(u64::MAX))
        .unwrap_or(0)
}

/// 16 random bits from the operating system (Go uses `crypto/rand`). Should the
/// OS source fail, the nanosecond clock is used, as Go's fallback does.
fn random_u16() -> u16 {
    let mut buf = [0u8; 2];
    match getrandom::fill(&mut buf) {
        Ok(()) => u16::from_be_bytes(buf),
        Err(_) => SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map(|d| d.subsec_nanos() as u16)
            .unwrap_or(0),
    }
}

fn encode_crockford(mut value: u64) -> String {
    let mut out = [b'0'; 13];
    for slot in out.iter_mut().rev() {
        *slot = ALPHABET[(value & 0x1F) as usize];
        value >>= 5;
    }
    out.iter().map(|&b| char::from(b)).collect()
}

/// 13 characters carry 65 bits; the first may only use the low 4 (`0`..`F`),
/// or the value would not fit in a `u64`.
fn decode_crockford(s: &str) -> Option<u64> {
    if s.len() != 13 {
        return None;
    }
    let mut value: u64 = 0;
    for (i, c) in s.bytes().enumerate() {
        let digit = match c.to_ascii_uppercase() {
            c @ b'0'..=b'9' => c - b'0',
            c @ b'A'..=b'H' => c - b'A' + 10,
            c @ b'J'..=b'K' => c - b'J' + 18,
            c @ b'M'..=b'N' => c - b'M' + 20,
            c @ b'P'..=b'T' => c - b'P' + 22,
            c @ b'V'..=b'Z' => c - b'V' + 27,
            _ => return None,
        };
        if i == 0 && digit > 0xF {
            return None;
        }
        value = (value << 5) | u64::from(digit);
    }
    Some(value)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashSet;
    use std::thread;

    #[test]
    fn ids_are_strictly_increasing_within_a_process() {
        let ids: Vec<String> = (0..20_000).map(|_| generate_untyped()).collect();
        for pair in ids.windows(2) {
            assert!(pair[0] < pair[1], "{} !< {}", pair[0], pair[1]);
        }
    }

    #[test]
    fn no_duplicates_past_the_sequence_capacity() {
        let mut seen = HashSet::new();
        for _ in 0..100_000 {
            assert!(seen.insert(Tsid::generate()));
        }
    }

    #[test]
    fn no_duplicates_across_threads() {
        let handles: Vec<_> = (0..8)
            .map(|_| thread::spawn(|| (0..10_000).map(|_| Tsid::generate()).collect::<Vec<_>>()))
            .collect();
        let mut seen = HashSet::new();
        for h in handles {
            for id in h.join().unwrap() {
                assert!(seen.insert(id));
            }
        }
    }

    #[test]
    fn layout_puts_the_clock_in_the_top_42_bits() {
        let before = now_millis();
        let id = Tsid::generate();
        let after = now_millis();
        // Borrowed milliseconds can run slightly ahead of the clock.
        assert!(id.timestamp_millis() >= before && id.timestamp_millis() <= after + 1000);
    }

    /// A fixed value encodes to the same string the Go SDK produces
    /// (`tsid.FromLong`), and decodes back (`tsid.ToLong`).
    #[test]
    fn encoding_matches_the_go_sdk() {
        // Go: tsid.FromLong(0) == "0000000000000",
        //     tsid.FromLong(1) == "0000000000001",
        //     tsid.FromLong(math.MaxInt64) == "7ZZZZZZZZZZZZ".
        assert_eq!(from_long(0), "0000000000000");
        assert_eq!(from_long(1), "0000000000001");
        assert_eq!(from_long(i64::MAX), "7ZZZZZZZZZZZZ");
        assert_eq!(Tsid::from_u64(u64::MAX).to_string(), "FZZZZZZZZZZZZ");
        assert_eq!(to_long("7ZZZZZZZZZZZZ"), Some(i64::MAX));
        assert_eq!(to_long("clt_0000000000001"), Some(1));
        // Crockford is case-insensitive on the way in.
        assert_eq!(to_long("7zzzzzzzzzzzz"), Some(i64::MAX));
    }

    #[test]
    fn decoding_refuses_malformed_ids() {
        assert!("ABC".parse::<Tsid>().is_err());
        assert!("IIIIIIIIIIIII".parse::<Tsid>().is_err());
        assert!("UUUUUUUUUUUUU".parse::<Tsid>().is_err());
        // 65 bits: would overflow u64 (Go and fc-sdk silently wrapped).
        assert!("GZZZZZZZZZZZZ".parse::<Tsid>().is_err());
        assert_eq!(to_long("not-an-id"), None);
    }

    #[test]
    fn typed_ids_use_the_platform_prefixes() {
        let id = generate(EntityType::Client);
        assert_eq!(id.len(), 17);
        assert!(id.starts_with("clt_"));
        assert_eq!(EntityType::PortalIdentity.prefix(), "ptu");
        assert_eq!(EntityType::PortalApp.prefix(), "pta");
        assert_eq!(EntityType::DispatchJob.prefix(), "djb");
    }

    #[test]
    fn a_prefix_may_contain_underscores() {
        let id = generate_with_prefix("my_ord");
        let raw = id.rsplit_once('_').unwrap().1;
        assert_eq!(to_long(&id), to_long(raw));
        assert!(to_long(&id).is_some());
    }

    #[test]
    fn round_trips_through_display_and_parse() {
        let id = Tsid::generate();
        assert_eq!(id.to_string().parse::<Tsid>().unwrap(), id);
        assert_eq!(to_long(&id.to_string()), Some(id.as_u64() as i64));
    }
}
