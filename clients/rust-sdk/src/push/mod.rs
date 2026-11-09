//! Push gateway declarations: topic rules and ticket keys.
//!
//! What an application declares about its push topics, synced to the push
//! gateway (docs/push-gateway-spec.md section 8) as a declarative full replace:
//! the payload IS the set, so both halves are sent every time. Add it to a
//! [`DefinitionSet`](crate::sync::DefinitionSet) with `with_push`, or call
//! [`client.push().sync(..)`](crate::client::FlowCatalystClient::push).
//!
//! Tickets (short-lived grants to listen on topics, minted by the application
//! after its own access check) are in [`tickets`] (feature `push-tickets`).

use serde::{Deserialize, Serialize};

#[cfg(feature = "push-tickets")]
pub mod tickets;

#[cfg(feature = "push-tickets")]
pub use tickets::{MintTicket, TicketError, TicketKey, TicketMinter};

/// A topic rule (push contract section 6.3). A pattern's segments are literals
/// or placeholders: `{principal}` must equal the subscriber's principal id,
/// `{client}` the stream's client, any other `{name}` matches one segment. The
/// first segment must be the application's code.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize, Default)]
pub struct PushRule {
    /// `{application}:{segment}(:{segment})*`
    pub pattern: String,
    /// Permissions the subscriber must all hold.
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub requires: Vec<String>,
    /// A deliberately open topic: anyone may listen, with or without
    /// credentials. Required for a rule with neither `{principal}` nor
    /// `requires`; a public rule cannot also require permissions.
    #[serde(default, skip_serializing_if = "is_false")]
    pub public: bool,
}

fn is_false(b: &bool) -> bool {
    !*b
}

impl PushRule {
    /// A rule for `pattern` that requires nothing yet (set `requires`,
    /// `{principal}` in the pattern, or [`public`](Self::public)).
    pub fn new(pattern: impl Into<String>) -> Self {
        Self {
            pattern: pattern.into(),
            ..Self::default()
        }
    }

    pub fn requires(mut self, permissions: impl IntoIterator<Item = impl Into<String>>) -> Self {
        self.requires = permissions.into_iter().map(Into::into).collect();
        self
    }

    pub fn public(mut self) -> Self {
        self.public = true;
        self
    }
}

/// An Ed25519 public key as a JWK: the shape the gateway stores and
/// `/push/keys` serves.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct TicketPublicJwk {
    pub kty: String,
    pub crv: String,
    pub x: String,
    pub kid: String,
    #[serde(rename = "use")]
    pub use_: String,
    pub alg: String,
}

/// A JWKS of ticket keys: what sync sends and `GET /push/keys` answers (at most 4).
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize, Default)]
pub struct TicketJwks {
    pub keys: Vec<TicketPublicJwk>,
}

/// The application's push declaration: rules and ticket keys.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct PushDefinition {
    pub rules: Vec<PushRule>,
    /// The ticket verification keys: a minter's JWKS, or a JWKS of Ed25519
    /// public keys (at most 4).
    pub keys: TicketJwks,
}

impl PushDefinition {
    pub fn new(rules: Vec<PushRule>, keys: TicketJwks) -> Self {
        Self { rules, keys }
    }
}
