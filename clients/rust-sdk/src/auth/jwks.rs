//! JWKS Cache and JWT Token Validation
//!
//! Fetches FlowCatalyst's public keys via OIDC discovery and validates
//! JWT access tokens using RS256 signature verification.

use chrono::{DateTime, Utc};
use jsonwebtoken::{Algorithm, DecodingKey, Validation, decode};
use serde::Deserialize;
use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use tokio::sync::RwLock;

use super::AuthError;
use super::claims::{AccessTokenClaims, AuthContext};
use jsonwebtoken::errors::ErrorKind;
use std::time::Duration;

/// JWKS response from the provider.
#[derive(Debug, Clone, Deserialize)]
pub struct Jwks {
    pub keys: Vec<JwkKey>,
}

/// Individual JWK key.
#[derive(Debug, Clone, Deserialize)]
pub struct JwkKey {
    /// Key type (e.g., "RSA")
    pub kty: String,
    /// Key usage (e.g., "sig")
    #[serde(rename = "use")]
    pub key_use: Option<String>,
    /// Key ID
    pub kid: Option<String>,
    /// Algorithm (e.g., "RS256")
    pub alg: Option<String>,
    /// RSA modulus (base64url)
    pub n: Option<String>,
    /// RSA exponent (base64url)
    pub e: Option<String>,
}

/// Partial OIDC discovery document.
#[derive(Debug, Deserialize)]
struct DiscoveryDoc {
    jwks_uri: String,
    #[serde(default)]
    #[allow(dead_code)]
    issuer: Option<String>,
}

/// Cached JWKS entry.
#[derive(Debug)]
struct CachedJwks {
    jwks: Jwks,
    fetched_at: DateTime<Utc>,
}

/// JWKS cache with per-issuer TTL.
///
/// Automatically discovers and caches FlowCatalyst's public keys
/// via the `.well-known/openid-configuration` endpoint.
pub struct JwksCache {
    cache: Arc<RwLock<HashMap<String, CachedJwks>>>,
    http_client: reqwest::Client,
    ttl_secs: i64,
    /// Unix seconds of the last forced refetch (unknown `kid`) per issuer.
    forced_at: Mutex<HashMap<String, i64>>,
    /// Minimum seconds between forced refetches per issuer.
    forced_interval_secs: i64,
    /// Clock in unix seconds; injectable so tests need no sleeps.
    clock: Arc<dyn Fn() -> i64 + Send + Sync>,
}

impl std::fmt::Debug for JwksCache {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("JwksCache")
            .field("ttl_secs", &self.ttl_secs)
            .field("forced_interval_secs", &self.forced_interval_secs)
            .finish_non_exhaustive()
    }
}

/// Default minimum gap between forced JWKS refetches for one issuer.
const FORCED_REFETCH_INTERVAL_SECS: i64 = 60;

impl JwksCache {
    /// Create a new JWKS cache with the given TTL (in seconds).
    pub fn new(ttl_secs: i64) -> Self {
        Self {
            cache: Arc::new(RwLock::new(HashMap::new())),
            http_client: reqwest::Client::builder()
                .timeout(Duration::from_secs(15))
                .build()
                .unwrap_or_default(),
            ttl_secs,
            forced_at: Mutex::new(HashMap::new()),
            forced_interval_secs: FORCED_REFETCH_INTERVAL_SECS,
            clock: Arc::new(|| Utc::now().timestamp()),
        }
    }

    /// Set the minimum seconds between forced refetches (unknown `kid`) for
    /// one issuer. Default 60.
    pub fn with_forced_refetch_interval(mut self, secs: i64) -> Self {
        self.forced_interval_secs = secs;
        self
    }

    /// Get the JWKS for a token whose `kid` is not in the cached set.
    ///
    /// At most one forced refetch per issuer per forced-refetch interval (the
    /// attempt is stamped whether or not it succeeds). Inside the window the
    /// cached set is returned with no network call; a failed refetch keeps
    /// the previously cached set until its normal expiry.
    pub async fn get_jwks_for_unknown_kid(&self, issuer_url: &str) -> Result<Jwks, AuthError> {
        let now = (self.clock)();
        let allowed = {
            let mut forced = self.forced_at.lock().unwrap_or_else(|e| e.into_inner());
            match forced.get(issuer_url) {
                Some(last) if now - *last < self.forced_interval_secs => false,
                _ => {
                    forced.insert(issuer_url.to_string(), now);
                    true
                }
            }
        };
        if !allowed {
            return self.get_jwks(issuer_url).await;
        }

        match self.fetch_jwks(issuer_url).await {
            Ok(jwks) => {
                let mut cache = self.cache.write().await;
                cache.insert(
                    issuer_url.to_string(),
                    CachedJwks {
                        jwks: jwks.clone(),
                        fetched_at: Utc::now(),
                    },
                );
                Ok(jwks)
            }
            Err(err) => {
                // Keep the previously cached set (not dropped) on failure.
                let cache = self.cache.read().await;
                match cache.get(issuer_url) {
                    Some(entry) => Ok(entry.jwks.clone()),
                    None => Err(err),
                }
            }
        }
    }

    /// Get JWKS for an issuer, fetching from network if not cached or expired.
    pub async fn get_jwks(&self, issuer_url: &str) -> Result<Jwks, AuthError> {
        // Check cache
        {
            let cache = self.cache.read().await;
            if let Some(entry) = cache.get(issuer_url) {
                let age = (Utc::now() - entry.fetched_at).num_seconds();
                if age < self.ttl_secs {
                    return Ok(entry.jwks.clone());
                }
            }
        }

        // Fetch fresh
        let jwks = self.fetch_jwks(issuer_url).await?;

        // Store
        {
            let mut cache = self.cache.write().await;
            cache.insert(
                issuer_url.to_string(),
                CachedJwks {
                    jwks: jwks.clone(),
                    fetched_at: Utc::now(),
                },
            );
        }

        Ok(jwks)
    }

    /// Fetch JWKS via OIDC discovery.
    async fn fetch_jwks(&self, issuer_url: &str) -> Result<Jwks, AuthError> {
        let base = issuer_url.trim_end_matches('/');
        let discovery_url = format!("{}/.well-known/openid-configuration", base);

        let discovery: DiscoveryDoc = self
            .http_client
            .get(&discovery_url)
            .send()
            .await
            .map_err(|e| {
                AuthError::Discovery(format!(
                    "Failed to fetch discovery from {}: {}",
                    discovery_url, e
                ))
            })?
            .json()
            .await
            .map_err(|e| AuthError::Discovery(format!("Failed to parse discovery: {}", e)))?;

        let jwks: Jwks = self
            .http_client
            .get(&discovery.jwks_uri)
            .send()
            .await
            .map_err(|e| {
                AuthError::Discovery(format!(
                    "Failed to fetch JWKS from {}: {}",
                    discovery.jwks_uri, e
                ))
            })?
            .json()
            .await
            .map_err(|e| AuthError::Discovery(format!("Failed to parse JWKS: {}", e)))?;

        if jwks.keys.is_empty() {
            return Err(AuthError::Discovery(format!(
                "the JWKS of {issuer_url} contains no keys"
            )));
        }
        Ok(jwks)
    }

    /// Invalidate cached JWKS for a specific issuer (forces re-fetch on next use).
    pub async fn invalidate(&self, issuer_url: &str) {
        let mut cache = self.cache.write().await;
        cache.remove(issuer_url);
    }
}

impl Default for JwksCache {
    fn default() -> Self {
        Self::new(3600) // 1 hour
    }
}

/// Validates JWT access tokens issued by a FlowCatalyst OIDC server.
///
/// Uses JWKS auto-discovery to fetch and cache the server's public keys,
/// then validates token signatures, expiry, issuer, and audience.
///
/// # Example
///
/// ```ignore
/// use flowcatalyst_sdk::auth::{TokenValidator, TokenValidatorConfig};
///
/// let validator = TokenValidator::new(TokenValidatorConfig {
///     issuer_url: "https://auth.flowcatalyst.io".to_string(),
///     audience: "my-app".to_string(),
///     ..Default::default()
/// });
///
/// // Validate a Bearer token
/// let auth_ctx = validator.validate("eyJ...").await?;
/// println!("Hello, {}", auth_ctx.name());
///
/// if auth_ctx.has_role("admin") {
///     // Authorized
/// }
/// ```
#[derive(Debug)]
pub struct TokenValidator {
    config: TokenValidatorConfig,
    jwks_cache: JwksCache,
}

/// Configuration for the token validator.
#[derive(Debug, Clone)]
pub struct TokenValidatorConfig {
    /// FlowCatalyst OIDC server URL (e.g., `"https://auth.flowcatalyst.io"`)
    pub issuer_url: String,

    /// Expected audience claim (your application identifier)
    pub audience: String,

    /// JWKS cache TTL in seconds (default: 3600 = 1 hour)
    pub jwks_ttl_secs: i64,

    /// Allowed clock skew in seconds for exp/nbf validation (default: 60)
    pub clock_skew_secs: u64,
}

impl Default for TokenValidatorConfig {
    fn default() -> Self {
        Self {
            issuer_url: String::new(),
            audience: "flowcatalyst".to_string(),
            jwks_ttl_secs: 3600,
            clock_skew_secs: 60,
        }
    }
}

impl TokenValidator {
    /// Create a new token validator.
    pub fn new(config: TokenValidatorConfig) -> Self {
        let jwks_cache = JwksCache::new(config.jwks_ttl_secs);
        Self { config, jwks_cache }
    }

    /// Validate a JWT access token and return an [`AuthContext`].
    ///
    /// Performs:
    /// 1. Decode JWT header to find the key ID (`kid`)
    /// 2. Fetch JWKS from the issuer (cached)
    /// 3. Find matching RSA public key
    /// 4. Verify RS256 signature
    /// 5. Validate claims: `iss`, `aud`, `exp`, `nbf`
    /// 6. Return parsed [`AuthContext`] with claims
    pub async fn validate(&self, token: &str) -> Result<AuthContext, AuthError> {
        // Decode header to get kid
        let header = jsonwebtoken::decode_header(token)
            .map_err(|e| AuthError::InvalidToken(format!("Invalid JWT header: {}", e)))?;

        let kid = header.kid.as_deref();

        // Fetch JWKS (cached). An unknown kid is a key rotation the cache has
        // not seen yet: refetch (at most once per issuer per minute, keeping the
        // cached set if the refetch fails) before refusing the token.
        let mut jwks = self.jwks_cache.get_jwks(&self.config.issuer_url).await?;
        if self.find_key(&jwks, kid).is_err() && kid.is_some() {
            jwks = self
                .jwks_cache
                .get_jwks_for_unknown_kid(&self.config.issuer_url)
                .await?;
        }
        let key = self.find_key(&jwks, kid)?;

        // Build decoding key from RSA components
        let n = key
            .n
            .as_deref()
            .ok_or_else(|| AuthError::InvalidToken("JWK missing RSA modulus (n)".to_string()))?;
        let e = key
            .e
            .as_deref()
            .ok_or_else(|| AuthError::InvalidToken("JWK missing RSA exponent (e)".to_string()))?;

        let decoding_key = DecodingKey::from_rsa_components(n, e)
            .map_err(|e| AuthError::InvalidToken(format!("Invalid RSA components: {}", e)))?;

        // Validate
        let mut validation = Validation::new(Algorithm::RS256);
        validation.set_issuer(&[&self.config.issuer_url]);
        validation.set_audience(&[&self.config.audience]);
        validation.leeway = self.config.clock_skew_secs;

        let token_data =
            decode::<AccessTokenClaims>(token, &decoding_key, &validation).map_err(|e| match e
                .kind()
            {
                ErrorKind::ExpiredSignature => AuthError::TokenExpired,
                ErrorKind::InvalidAudience => {
                    AuthError::InvalidToken(format!("Invalid audience: {}", e))
                }
                ErrorKind::InvalidIssuer => {
                    AuthError::InvalidToken(format!("Invalid issuer: {}", e))
                }
                _ => AuthError::InvalidToken(format!("{}", e)),
            })?;

        Ok(AuthContext::new(token_data.claims, token.to_string()))
    }

    /// Validate a token from an `Authorization: Bearer <token>` header value.
    pub async fn validate_bearer(&self, auth_header: &str) -> Result<AuthContext, AuthError> {
        let token = auth_header
            .strip_prefix("Bearer ")
            .ok_or_else(|| AuthError::InvalidToken("Missing 'Bearer ' prefix".to_string()))?;

        self.validate(token).await
    }

    /// The key named by `kid` (no fallback to another key), or, for a token
    /// without a `kid`, the first RSA signing key.
    fn find_key<'a>(&self, jwks: &'a Jwks, kid: Option<&str>) -> Result<&'a JwkKey, AuthError> {
        // If kid is specified, find exact match
        if let Some(kid) = kid {
            if let Some(key) = jwks.keys.iter().find(|k| k.kid.as_deref() == Some(kid)) {
                return Ok(key);
            }
            return Err(AuthError::InvalidToken(format!(
                "no JWK with kid {kid:?} in the issuer's key set"
            )));
        }

        // Find first RSA signing key
        jwks.keys
            .iter()
            .find(|k| {
                k.kty == "RSA"
                    && k.key_use.as_deref() != Some("enc") // not encryption-only
                    && k.n.is_some()
                    && k.e.is_some()
            })
            .ok_or_else(|| AuthError::InvalidToken("No suitable RSA key found in JWKS".to_string()))
    }

    /// Force refresh of cached JWKS (e.g., after key rotation).
    pub async fn refresh_jwks(&self) {
        self.jwks_cache.invalidate(&self.config.issuer_url).await;
    }
}

/// Validates tokens using a shared HMAC secret (HS256).
///
/// Use this for development or when your app shares a secret with FlowCatalyst
/// instead of using JWKS-based RS256 validation.
///
/// # Example
///
/// ```ignore
/// use flowcatalyst_sdk::auth::HmacTokenValidator;
///
/// let validator = HmacTokenValidator::new(
///     "your-shared-secret",
///     "flowcatalyst",  // issuer
///     "flowcatalyst",  // audience
/// );
///
/// let ctx = validator.validate("eyJ...")?;
/// ```
pub struct HmacTokenValidator {
    decoding_key: DecodingKey,
    issuer: String,
    audience: String,
}

impl std::fmt::Debug for HmacTokenValidator {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("HmacTokenValidator")
            .field("issuer", &self.issuer)
            .field("audience", &self.audience)
            .finish_non_exhaustive()
    }
}

impl HmacTokenValidator {
    /// Create a new HMAC token validator.
    pub fn new(secret: &str, issuer: &str, audience: &str) -> Self {
        Self {
            decoding_key: DecodingKey::from_secret(secret.as_bytes()),
            issuer: issuer.to_string(),
            audience: audience.to_string(),
        }
    }

    /// Validate a JWT token signed with HS256.
    pub fn validate(&self, token: &str) -> Result<AuthContext, AuthError> {
        let mut validation = Validation::new(Algorithm::HS256);
        validation.set_issuer(&[&self.issuer]);
        validation.set_audience(&[&self.audience]);

        let token_data = decode::<AccessTokenClaims>(token, &self.decoding_key, &validation)
            .map_err(|e| match e.kind() {
                ErrorKind::ExpiredSignature => AuthError::TokenExpired,
                _ => AuthError::InvalidToken(format!("{}", e)),
            })?;

        Ok(AuthContext::new(token_data.claims, token.to_string()))
    }

    /// Validate a token from an `Authorization: Bearer <token>` header value.
    pub fn validate_bearer(&self, auth_header: &str) -> Result<AuthContext, AuthError> {
        let token = auth_header
            .strip_prefix("Bearer ")
            .ok_or_else(|| AuthError::InvalidToken("Missing 'Bearer ' prefix".to_string()))?;

        self.validate(token)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use jsonwebtoken::{EncodingKey, Header, encode};

    // ─── TokenValidatorConfig ───────────────────────────────────────────

    #[test]
    fn token_validator_config_default() {
        let config = TokenValidatorConfig::default();
        assert!(config.issuer_url.is_empty());
        assert_eq!(config.audience, "flowcatalyst");
        assert_eq!(config.jwks_ttl_secs, 3600);
        assert_eq!(config.clock_skew_secs, 60);
    }

    #[test]
    fn token_validator_config_clone() {
        let config = TokenValidatorConfig {
            issuer_url: "https://auth.example.com".to_string(),
            audience: "my-app".to_string(),
            jwks_ttl_secs: 120,
            clock_skew_secs: 30,
        };
        let cloned = config.clone();
        assert_eq!(cloned.issuer_url, "https://auth.example.com");
        assert_eq!(cloned.audience, "my-app");
        assert_eq!(cloned.jwks_ttl_secs, 120);
        assert_eq!(cloned.clock_skew_secs, 30);
    }

    // ─── JwksCache ──────────────────────────────────────────────────────

    #[test]
    fn jwks_cache_default_ttl() {
        let cache = JwksCache::default();
        assert_eq!(cache.ttl_secs, 3600);
    }

    #[test]
    fn jwks_cache_custom_ttl() {
        let cache = JwksCache::new(120);
        assert_eq!(cache.ttl_secs, 120);
    }

    #[tokio::test]
    async fn jwks_cache_invalidate_is_noop_when_empty() {
        let cache = JwksCache::new(60);
        // Should not panic
        cache.invalidate("https://not-cached.example.com").await;
    }

    // ─── L14: unknown-kid refetch is rate-limited ───────────────────────

    use std::sync::atomic::{AtomicBool, AtomicI64, AtomicUsize, Ordering};
    use tokio::io::{AsyncReadExt, AsyncWriteExt};

    /// A tiny HTTP server: discovery + JWKS. Counts discovery fetches, can be
    /// told to fail, and publishes the kids in `kids`.
    struct FakeIssuer {
        base: String,
        discovery_hits: Arc<AtomicUsize>,
        failing: Arc<AtomicBool>,
        kids: Arc<Mutex<Vec<String>>>,
    }

    async fn fake_issuer(initial: &[&str]) -> FakeIssuer {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let base = format!("http://{}", listener.local_addr().unwrap());
        let hits = Arc::new(AtomicUsize::new(0));
        let failing = Arc::new(AtomicBool::new(false));
        let kids = Arc::new(Mutex::new(
            initial.iter().map(|k| k.to_string()).collect::<Vec<_>>(),
        ));
        let (b, h, f, k) = (base.clone(), hits.clone(), failing.clone(), kids.clone());
        tokio::spawn(async move {
            loop {
                let (mut sock, _) = listener.accept().await.unwrap();
                let (b, h, f, k) = (b.clone(), h.clone(), f.clone(), k.clone());
                tokio::spawn(async move {
                    // Read the whole request head so closing never resets the connection.
                    let mut req = String::new();
                    let mut buf = [0u8; 2048];
                    while !req.contains("\r\n\r\n") {
                        let n = sock.read(&mut buf).await.unwrap_or(0);
                        if n == 0 {
                            break;
                        }
                        req.push_str(&String::from_utf8_lossy(&buf[..n]));
                    }
                    let (status, body) = if req.contains("openid-configuration") {
                        h.fetch_add(1, Ordering::SeqCst);
                        if f.load(Ordering::SeqCst) {
                            ("500 Internal Server Error", "boom".to_string())
                        } else {
                            ("200 OK", format!(r#"{{"jwks_uri":"{b}/jwks"}}"#))
                        }
                    } else {
                        let keys: Vec<String> = k
                            .lock()
                            .unwrap()
                            .iter()
                            .map(|kid| {
                                format!(r#"{{"kty":"RSA","kid":"{kid}","n":"abc","e":"AQAB"}}"#)
                            })
                            .collect();
                        ("200 OK", format!(r#"{{"keys":[{}]}}"#, keys.join(",")))
                    };
                    let resp = format!(
                        "HTTP/1.1 {status}\r\ncontent-type: application/json\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{body}",
                        body.len()
                    );
                    let _ = sock.write_all(resp.as_bytes()).await;
                    let _ = sock.shutdown().await;
                });
            }
        });
        FakeIssuer { base, discovery_hits: hits, failing, kids }
    }

    fn cache_with_clock(clock: Arc<AtomicI64>) -> JwksCache {
        let mut c = JwksCache::new(3600);
        c.clock = Arc::new(move || clock.load(Ordering::SeqCst));
        c
    }

    fn has_kid(j: &Jwks, kid: &str) -> bool {
        j.keys.iter().any(|k| k.kid.as_deref() == Some(kid))
    }

    /// A token header naming `kid` (alg RS256), unsigned: `validate` refuses
    /// it at key lookup, which is all these tests exercise.
    fn token_for_kid(kid: &str) -> String {
        use base64::Engine;
        let e = base64::engine::general_purpose::URL_SAFE_NO_PAD;
        let h = e.encode(format!(r#"{{"alg":"RS256","typ":"JWT","kid":"{kid}"}}"#));
        let p = e.encode(r#"{"sub":"x"}"#);
        format!("{h}.{p}.c2ln")
    }

    #[tokio::test]
    async fn unknown_kids_inside_window_cause_exactly_one_refetch() {
        let issuer = fake_issuer(&["k1"]).await;
        let clock = Arc::new(AtomicI64::new(1_000_000));
        let validator = TokenValidator {
            config: TokenValidatorConfig {
                issuer_url: issuer.base.clone(),
                ..Default::default()
            },
            jwks_cache: cache_with_clock(clock.clone()),
        };
        for i in 0..20 {
            let r = validator.validate(&token_for_kid(&format!("rogue{i}"))).await;
            assert!(r.is_err());
        }
        // 1 initial fill + exactly 1 forced refetch for 20 unknown kids.
        assert_eq!(issuer.discovery_hits.load(Ordering::SeqCst), 2);

        // After the window another refetch is allowed.
        clock.fetch_add(61, Ordering::SeqCst);
        let _ = validator.validate(&token_for_kid("rogue-later")).await;
        assert_eq!(issuer.discovery_hits.load(Ordering::SeqCst), 3);
    }

    #[tokio::test]
    async fn refetch_after_window_picks_up_a_newly_published_key() {
        let issuer = fake_issuer(&["k1"]).await;
        let clock = Arc::new(AtomicI64::new(1_000_000));
        let cache = cache_with_clock(clock.clone());
        cache.get_jwks(&issuer.base).await.unwrap();

        let first = cache.get_jwks_for_unknown_kid(&issuer.base).await.unwrap();
        assert!(!has_kid(&first, "k2"));

        issuer.kids.lock().unwrap().push("k2".into());
        clock.fetch_add(30, Ordering::SeqCst);
        let inside = cache.get_jwks_for_unknown_kid(&issuer.base).await.unwrap();
        assert!(!has_kid(&inside, "k2"), "inside the window: no refetch");

        clock.fetch_add(31, Ordering::SeqCst);
        let after = cache.get_jwks_for_unknown_kid(&issuer.base).await.unwrap();
        assert!(has_kid(&after, "k2"));
    }

    #[tokio::test]
    async fn failed_forced_refetch_keeps_the_cached_key_set() {
        let issuer = fake_issuer(&["k1"]).await;
        let clock = Arc::new(AtomicI64::new(1_000_000));
        let cache = cache_with_clock(clock);
        cache.get_jwks(&issuer.base).await.unwrap();

        issuer.failing.store(true, Ordering::SeqCst);
        let jwks = cache.get_jwks_for_unknown_kid(&issuer.base).await.unwrap();
        assert!(has_kid(&jwks, "k1"));
        assert_eq!(issuer.discovery_hits.load(Ordering::SeqCst), 2);
        // The cache still serves k1 afterwards.
        assert!(has_kid(&cache.get_jwks(&issuer.base).await.unwrap(), "k1"));
    }

    // ─── JwkKey deserialization ─────────────────────────────────────────

    #[test]
    fn jwk_key_deserialization() {
        let json = r#"{
            "kty": "RSA",
            "use": "sig",
            "kid": "key-1",
            "alg": "RS256",
            "n": "0vx7agoebGcQSuuPiLJXZptN9nndrQmbXEps2aiAFbWhM",
            "e": "AQAB"
        }"#;
        let key: JwkKey = serde_json::from_str(json).unwrap();
        assert_eq!(key.kty, "RSA");
        assert_eq!(key.key_use.as_deref(), Some("sig"));
        assert_eq!(key.kid.as_deref(), Some("key-1"));
        assert_eq!(key.alg.as_deref(), Some("RS256"));
        assert_eq!(
            key.n.as_deref(),
            Some("0vx7agoebGcQSuuPiLJXZptN9nndrQmbXEps2aiAFbWhM")
        );
        assert_eq!(key.e.as_deref(), Some("AQAB"));
    }

    #[test]
    fn jwk_key_deserialization_minimal() {
        let json = r#"{"kty": "RSA"}"#;
        let key: JwkKey = serde_json::from_str(json).unwrap();
        assert_eq!(key.kty, "RSA");
        assert!(key.key_use.is_none());
        assert!(key.kid.is_none());
        assert!(key.alg.is_none());
        assert!(key.n.is_none());
        assert!(key.e.is_none());
    }

    #[test]
    fn jwks_deserialization() {
        let json = r#"{
            "keys": [
                {"kty": "RSA", "kid": "k1", "n": "abc", "e": "AQAB"},
                {"kty": "RSA", "kid": "k2", "n": "def", "e": "AQAB"}
            ]
        }"#;
        let jwks: Jwks = serde_json::from_str(json).unwrap();
        assert_eq!(jwks.keys.len(), 2);
        assert_eq!(jwks.keys[0].kid.as_deref(), Some("k1"));
        assert_eq!(jwks.keys[1].kid.as_deref(), Some("k2"));
    }

    #[test]
    fn jwks_empty_keys() {
        let json = r#"{"keys": []}"#;
        let jwks: Jwks = serde_json::from_str(json).unwrap();
        assert!(jwks.keys.is_empty());
    }

    // ─── TokenValidator::find_key ───────────────────────────────────────

    fn make_rsa_key(kid: &str) -> JwkKey {
        JwkKey {
            kty: "RSA".to_string(),
            key_use: Some("sig".to_string()),
            kid: Some(kid.to_string()),
            alg: Some("RS256".to_string()),
            n: Some("modulus".to_string()),
            e: Some("AQAB".to_string()),
        }
    }

    fn make_validator() -> TokenValidator {
        TokenValidator::new(TokenValidatorConfig {
            issuer_url: "https://auth.example.com".to_string(),
            audience: "test".to_string(),
            ..Default::default()
        })
    }

    #[test]
    fn find_key_by_kid_match() {
        let validator = make_validator();
        let jwks = Jwks {
            keys: vec![make_rsa_key("k1"), make_rsa_key("k2")],
        };
        let key = validator.find_key(&jwks, Some("k2")).unwrap();
        assert_eq!(key.kid.as_deref(), Some("k2"));
    }

    /// fc-sdk fell back to the first RSA key; a token naming a key the
    /// issuer does not publish is refused instead (after one JWKS refetch in
    /// `validate`), as the Go SDK's jwx key-set lookup does.
    #[test]
    fn find_key_refuses_an_unknown_kid() {
        let validator = make_validator();
        let jwks = Jwks {
            keys: vec![make_rsa_key("k1")],
        };
        let err = validator.find_key(&jwks, Some("k999")).unwrap_err();
        assert!(matches!(err, AuthError::InvalidToken(_)));
    }

    #[test]
    fn find_key_no_kid_returns_first_rsa() {
        let validator = make_validator();
        let jwks = Jwks {
            keys: vec![make_rsa_key("k1")],
        };
        let key = validator.find_key(&jwks, None).unwrap();
        assert_eq!(key.kid.as_deref(), Some("k1"));
    }

    #[test]
    fn find_key_skips_enc_key() {
        let validator = make_validator();
        let mut enc_key = make_rsa_key("enc");
        enc_key.key_use = Some("enc".to_string());

        let jwks = Jwks {
            keys: vec![enc_key, make_rsa_key("sig")],
        };
        let key = validator.find_key(&jwks, None).unwrap();
        assert_eq!(key.kid.as_deref(), Some("sig"));
    }

    #[test]
    fn find_key_skips_key_without_n() {
        let validator = make_validator();
        let mut incomplete = make_rsa_key("inc");
        incomplete.n = None;

        let jwks = Jwks {
            keys: vec![incomplete, make_rsa_key("complete")],
        };
        let key = validator.find_key(&jwks, None).unwrap();
        assert_eq!(key.kid.as_deref(), Some("complete"));
    }

    #[test]
    fn find_key_skips_key_without_e() {
        let validator = make_validator();
        let mut incomplete = make_rsa_key("inc");
        incomplete.e = None;

        let jwks = Jwks {
            keys: vec![incomplete, make_rsa_key("complete")],
        };
        let key = validator.find_key(&jwks, None).unwrap();
        assert_eq!(key.kid.as_deref(), Some("complete"));
    }

    #[test]
    fn find_key_error_when_no_suitable_key() {
        let validator = make_validator();
        let jwks = Jwks { keys: vec![] };
        let err = validator.find_key(&jwks, None).unwrap_err();
        assert!(matches!(err, AuthError::InvalidToken(_)));
    }

    #[test]
    fn find_key_error_when_only_non_rsa_keys() {
        let validator = make_validator();
        let ec_key = JwkKey {
            kty: "EC".to_string(),
            key_use: Some("sig".to_string()),
            kid: Some("ec1".to_string()),
            alg: Some("ES256".to_string()),
            n: None,
            e: None,
        };
        let jwks = Jwks { keys: vec![ec_key] };
        let err = validator.find_key(&jwks, None).unwrap_err();
        assert!(matches!(err, AuthError::InvalidToken(_)));
    }

    // ─── HmacTokenValidator ─────────────────────────────────────────────

    fn make_hs256_token(sub: &str, iss: &str, aud: &str, secret: &str) -> String {
        let now = chrono::Utc::now().timestamp();
        let claims = AccessTokenClaims {
            sub: sub.to_string(),
            iss: iss.to_string(),
            aud: aud.to_string(),
            exp: now + 3600,
            iat: now,
            nbf: now,
            jti: "jti_test".to_string(),
            principal_type: "USER".to_string(),
            tier: "ANCHOR".to_string(),
            scope: String::new(),
            email: Some("test@example.com".to_string()),
            name: "Test".to_string(),
            clients: vec!["*".to_string()],
            roles: vec!["admin".to_string()],
            applications: vec![],
            all_applications: false,
            token_use: None,
            azp: None,
        };

        let header = Header::new(Algorithm::HS256);
        encode(
            &header,
            &claims,
            &EncodingKey::from_secret(secret.as_bytes()),
        )
        .unwrap()
    }

    #[test]
    fn hmac_validator_accepts_valid_token() {
        let secret = "my-test-secret-at-least-32-chars!";
        let token = make_hs256_token("prn_1", "fc", "fc", secret);
        let validator = HmacTokenValidator::new(secret, "fc", "fc");

        let ctx = validator.validate(&token).unwrap();
        assert_eq!(ctx.principal_id(), "prn_1");
        assert!(ctx.is_anchor());
        assert!(ctx.has_role("admin"));
        assert_eq!(ctx.email(), Some("test@example.com"));
    }

    #[test]
    fn hmac_validator_rejects_wrong_secret() {
        let token = make_hs256_token("prn_1", "fc", "fc", "secret-a");
        let validator = HmacTokenValidator::new("secret-b", "fc", "fc");

        let err = validator.validate(&token).unwrap_err();
        assert!(matches!(err, AuthError::InvalidToken(_)));
    }

    #[test]
    fn hmac_validator_rejects_wrong_issuer() {
        let secret = "shared-secret-for-testing-12345!";
        let token = make_hs256_token("prn_1", "wrong-issuer", "fc", secret);
        let validator = HmacTokenValidator::new(secret, "fc", "fc");

        let err = validator.validate(&token).unwrap_err();
        assert!(matches!(err, AuthError::InvalidToken(_)));
    }

    #[test]
    fn hmac_validator_rejects_wrong_audience() {
        let secret = "shared-secret-for-testing-12345!";
        let token = make_hs256_token("prn_1", "fc", "wrong-aud", secret);
        let validator = HmacTokenValidator::new(secret, "fc", "fc");

        let err = validator.validate(&token).unwrap_err();
        assert!(matches!(err, AuthError::InvalidToken(_)));
    }

    #[test]
    fn hmac_validator_rejects_expired_token() {
        let secret = "shared-secret-for-testing-12345!";
        let now = chrono::Utc::now().timestamp();
        let claims = AccessTokenClaims {
            sub: "prn_1".to_string(),
            iss: "fc".to_string(),
            aud: "fc".to_string(),
            exp: now - 100, // expired
            iat: now - 200,
            nbf: now - 200,
            jti: "j".to_string(),
            principal_type: "USER".to_string(),
            tier: "CLIENT".to_string(),
            scope: String::new(),
            email: None,
            name: "t".to_string(),
            clients: vec![],
            roles: vec![],
            applications: vec![],
            all_applications: false,
            token_use: None,
            azp: None,
        };
        let token = encode(
            &Header::new(Algorithm::HS256),
            &claims,
            &EncodingKey::from_secret(secret.as_bytes()),
        )
        .unwrap();

        let validator = HmacTokenValidator::new(secret, "fc", "fc");
        let err = validator.validate(&token).unwrap_err();
        assert!(matches!(err, AuthError::TokenExpired));
    }

    #[test]
    fn hmac_validator_validate_bearer_strips_prefix() {
        let secret = "shared-secret-for-testing-12345!";
        let token = make_hs256_token("prn_1", "fc", "fc", secret);
        let validator = HmacTokenValidator::new(secret, "fc", "fc");

        let ctx = validator
            .validate_bearer(&format!("Bearer {}", token))
            .unwrap();
        assert_eq!(ctx.principal_id(), "prn_1");
    }

    #[test]
    fn hmac_validator_validate_bearer_rejects_missing_prefix() {
        let secret = "shared-secret-for-testing-12345!";
        let token = make_hs256_token("prn_1", "fc", "fc", secret);
        let validator = HmacTokenValidator::new(secret, "fc", "fc");

        let err = validator.validate_bearer(&token).unwrap_err();
        assert!(matches!(err, AuthError::InvalidToken(_)));
    }

    #[test]
    fn hmac_validator_returns_bearer_token_in_context() {
        let secret = "shared-secret-for-testing-12345!";
        let token = make_hs256_token("prn_1", "fc", "fc", secret);
        let validator = HmacTokenValidator::new(secret, "fc", "fc");

        let ctx = validator.validate(&token).unwrap();
        assert_eq!(ctx.bearer_token(), token);
    }
}
