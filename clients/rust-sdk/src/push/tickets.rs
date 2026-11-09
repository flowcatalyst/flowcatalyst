//! Push tickets: short-lived grants to listen on topics, minted by the
//! application after its own access check (push contract section 6.4;
//! docs/push-gateway-spec.md section 9).
//!
//! A ticket is a JWS compact token, `alg` `EdDSA` (Ed25519), header `kid`. The
//! gateway verifies it with the public key the application synced
//! ([`TicketMinter::jwks`] through [`PushDefinition`](super::PushDefinition)).
//!
//! ```
//! use flowcatalyst_sdk::push::{MintTicket, TicketMinter};
//!
//! let seed = "5BgJptHbkz4uIeRqgLz3Ws_38Ovwi1uCpSdyW45yMdc";
//! let minter = TicketMinter::from_config("shop", &format!("k1:{seed}"), &[])?;
//! let ticket = minter.mint(&MintTicket::new("acme", ["shop:orders:{id}"]).principal("prn_1"))?;
//! assert_eq!(ticket.split('.').count(), 3);
//! # Ok::<(), flowcatalyst_sdk::push::TicketError>(())
//! ```

use base64::Engine as _;
use base64::engine::general_purpose::URL_SAFE_NO_PAD;
use ed25519_dalek::{Signer, SigningKey};

use super::{TicketJwks, TicketPublicJwk};

/// The audience every ticket carries.
pub const TICKET_AUDIENCE: &str = "fw-push";
/// A ticket's `exp` is at most this many seconds after its `iat`.
pub const MAX_TICKET_TTL_SECONDS: u32 = 300;
/// The default lifetime of a minted ticket.
pub const DEFAULT_TICKET_TTL_SECONDS: u32 = 60;
/// A ticket names at most this many topic patterns.
pub const MAX_TICKET_TOPICS: usize = 100;

/// A ticket the helper refuses to configure or mint.
#[derive(Debug, Clone, PartialEq, Eq, thiserror::Error)]
#[error("ticket: {0}")]
pub struct TicketError(pub String);

fn err<T>(m: impl Into<String>) -> Result<T, TicketError> {
    Err(TicketError(m.into()))
}

/// An Ed25519 ticket signing key: `kid` plus the 32-byte seed. Configure it as
/// `kid:base64url-seed` ([`TicketKey::parse`]); the seed is a secret, the
/// derived public JWK is what the gateway needs.
#[derive(Clone)]
pub struct TicketKey {
    kid: String,
    key: SigningKey,
}

impl std::fmt::Debug for TicketKey {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("TicketKey")
            .field("kid", &self.kid)
            .finish_non_exhaustive()
    }
}

impl TicketKey {
    /// From a `kid:base64url-seed` string (the seed is the 32 raw bytes,
    /// base64url without padding).
    pub fn parse(config: &str) -> Result<Self, TicketError> {
        match config.find(':') {
            Some(i) if i > 0 && i + 1 < config.len() => {
                Self::from_seed(config[..i].trim(), config[i + 1..].trim())
            }
            _ => err("a ticket key is kid:base64url-seed"),
        }
    }

    pub fn from_seed(kid: &str, seed_base64url: &str) -> Result<Self, TicketError> {
        if kid.is_empty() || kid.len() > 64 {
            return err("a ticket key id is 1 to 64 characters");
        }
        let seed = URL_SAFE_NO_PAD
            .decode(seed_base64url.trim_end_matches('='))
            .ok()
            .and_then(|b| <[u8; 32]>::try_from(b).ok());
        match seed {
            Some(seed) => Ok(Self {
                kid: kid.to_owned(),
                key: SigningKey::from_bytes(&seed),
            }),
            None => err("a ticket key seed is 32 bytes, base64url"),
        }
    }

    pub fn kid(&self) -> &str {
        &self.kid
    }

    /// The public key as a JWK (no private member).
    pub fn to_public_jwk(&self) -> TicketPublicJwk {
        TicketPublicJwk {
            kty: "OKP".into(),
            crv: "Ed25519".into(),
            x: URL_SAFE_NO_PAD.encode(self.key.verifying_key().to_bytes()),
            kid: self.kid.clone(),
            use_: "sig".into(),
            alg: "EdDSA".into(),
        }
    }
}

/// What a ticket grants.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MintTicket {
    client: String,
    topics: Vec<String>,
    sub: Option<String>,
    jkt: Option<String>,
    ttl_seconds: Option<u32>,
    stream_exp: Option<i64>,
    now: Option<i64>,
}

impl MintTicket {
    /// A ticket for the stream's `client` (the platform client's value,
    /// `_platform`, for platform-wide) on `topics`: patterns of the application
    /// (placeholders `{x}` match one segment), at most 100.
    pub fn new(
        client: impl Into<String>,
        topics: impl IntoIterator<Item = impl Into<String>>,
    ) -> Self {
        Self {
            client: client.into(),
            topics: topics.into_iter().map(Into::into).collect(),
            sub: None,
            jkt: None,
            ttl_seconds: None,
            stream_exp: None,
            now: None,
        }
    }

    /// The principal, recorded in the stream token for logs.
    pub fn principal(mut self, sub: impl Into<String>) -> Self {
        self.sub = Some(sub.into());
        self
    }

    /// Bind the ticket to a DPoP key: the subscribe must prove it (`cnf.jkt`).
    pub fn jkt(mut self, jkt: impl Into<String>) -> Self {
        self.jkt = Some(jkt.into());
        self
    }

    /// Lifetime in seconds: 1 to 300 (default 60). The time to subscribe with it.
    pub fn ttl_seconds(mut self, ttl: u32) -> Self {
        self.ttl_seconds = Some(ttl);
        self
    }

    /// The latest the stream may live, in epoch seconds.
    pub fn stream_exp(mut self, epoch_seconds: i64) -> Self {
        self.stream_exp = Some(epoch_seconds);
        self
    }

    /// Issued-at in epoch seconds (default now). For tests.
    pub fn now(mut self, epoch_seconds: i64) -> Self {
        self.now = Some(epoch_seconds);
        self
    }
}

/// Mints tickets for one application with its current key, and exports the
/// JWKS (the current key then the previous ones during a rotation) that sync
/// sends to the gateway.
#[derive(Debug, Clone)]
pub struct TicketMinter {
    application_code: String,
    current: TicketKey,
    previous: Vec<TicketPublicJwk>,
}

fn is_application(s: &str) -> bool {
    let mut b = s.bytes();
    b.next().is_some_and(|c| c.is_ascii_lowercase())
        && b.all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == b'-')
}

impl TicketMinter {
    /// `application_code` is the `iss` of every ticket; each topic's first
    /// segment must equal it. `previous` are the keys of a rotation in
    /// progress: published in the JWKS so tickets signed with them keep
    /// verifying, never used to sign (a public JWK is enough).
    pub fn new(
        application_code: impl Into<String>,
        current: TicketKey,
        previous: Vec<TicketPublicJwk>,
    ) -> Result<Self, TicketError> {
        let application_code = application_code.into();
        if !is_application(&application_code) {
            return err(format!("not an application code: {application_code:?}"));
        }
        if previous.len() > 3 {
            return err("at most 4 ticket keys (the current and 3 previous)");
        }
        Ok(Self {
            application_code,
            current,
            previous,
        })
    }

    /// From configuration: the current key as `kid:base64url-seed`, and the
    /// previous ones (during a rotation) as `kid:base64url-seed` strings;
    /// blank entries are ignored.
    pub fn from_config(
        application_code: &str,
        current: &str,
        previous: &[&str],
    ) -> Result<Self, TicketError> {
        let previous = previous
            .iter()
            .filter(|p| !p.trim().is_empty())
            .map(|p| TicketKey::parse(p).map(|k| k.to_public_jwk()))
            .collect::<Result<Vec<_>, _>>()?;
        Self::new(application_code, TicketKey::parse(current)?, previous)
    }

    pub fn application_code(&self) -> &str {
        &self.application_code
    }

    /// The JWKS to sync: the current key, then the previous ones.
    pub fn jwks(&self) -> Result<TicketJwks, TicketError> {
        let mut keys = vec![self.current.to_public_jwk()];
        keys.extend(self.previous.iter().cloned());
        let mut kids: Vec<&str> = keys.iter().map(|k| k.kid.as_str()).collect();
        kids.sort_unstable();
        kids.dedup();
        if kids.len() != keys.len() {
            return err("duplicate ticket key ids");
        }
        Ok(TicketJwks { keys })
    }

    /// Mint a ticket. Claims, in order: `iss`, `aud`, `client`, `topics`,
    /// `sub`, `cnf`, `iat`, `exp`, `stream-exp`.
    pub fn mint(&self, t: &MintTicket) -> Result<String, TicketError> {
        let valid_client = !t.client.is_empty()
            && t.client.len() <= 100
            && t.client
                .bytes()
                .all(|c| c.is_ascii_alphanumeric() || matches!(c, b'.' | b'_' | b'-'));
        if !valid_client {
            return err("client is [A-Za-z0-9._-]{1,100}");
        }
        if t.topics.is_empty() || t.topics.len() > MAX_TICKET_TOPICS {
            return err(format!("a ticket names 1 to {MAX_TICKET_TOPICS} topics"));
        }
        for topic in &t.topics {
            let first = topic.split(':').next().unwrap_or("");
            if first != self.application_code || !topic.contains(':') {
                return err(format!(
                    "the topic {topic:?} is not a topic of the application {:?}",
                    self.application_code
                ));
            }
        }
        let ttl = t.ttl_seconds.unwrap_or(DEFAULT_TICKET_TTL_SECONDS);
        if !(1..=MAX_TICKET_TTL_SECONDS).contains(&ttl) {
            return err(format!("ttlSeconds is 1 to {MAX_TICKET_TTL_SECONDS}"));
        }
        let iat = match t.now {
            Some(n) => n,
            None => std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .map_or(0, |d| i64::try_from(d.as_secs()).unwrap_or(i64::MAX)),
        };
        let exp = iat + i64::from(ttl);

        let j = |v: &dyn erased::Json| v.json();
        let mut claims = format!(
            "{{\"iss\":{},\"aud\":{},\"client\":{},\"topics\":{}",
            j(&self.application_code.as_str())?,
            j(&TICKET_AUDIENCE)?,
            j(&t.client.as_str())?,
            j(&t.topics)?
        );
        if let Some(sub) = t.sub.as_deref().filter(|s| !s.is_empty()) {
            claims.push_str(&format!(",\"sub\":{}", j(&sub)?));
        }
        if let Some(jkt) = t.jkt.as_deref().filter(|s| !s.is_empty()) {
            claims.push_str(&format!(",\"cnf\":{{\"jkt\":{}}}", j(&jkt)?));
        }
        claims.push_str(&format!(",\"iat\":{iat},\"exp\":{exp}"));
        if let Some(se) = t.stream_exp {
            claims.push_str(&format!(",\"stream-exp\":{se}"));
        }
        claims.push('}');

        let header = format!(
            "{{\"alg\":\"EdDSA\",\"kid\":{}}}",
            j(&self.current.kid.as_str())?
        );
        let signing_input = format!(
            "{}.{}",
            URL_SAFE_NO_PAD.encode(header),
            URL_SAFE_NO_PAD.encode(claims)
        );
        let sig = self.current.key.sign(signing_input.as_bytes());
        Ok(format!(
            "{signing_input}.{}",
            URL_SAFE_NO_PAD.encode(sig.to_bytes())
        ))
    }
}

mod erased {
    use super::TicketError;

    pub trait Json {
        fn json(&self) -> Result<String, TicketError>;
    }

    impl<T: serde::Serialize + ?Sized> Json for T {
        fn json(&self) -> Result<String, TicketError> {
            serde_json::to_string(self).map_err(|e| TicketError(e.to_string()))
        }
    }
}
