//! A platform token verifies only in the spelling it was issued in
//! (docs/wire-contract.md, Authentication, "Token spelling"). The Rust SDK's
//! validators were reported strict already (jsonwebtoken on base64's
//! `URL_SAFE_NO_PAD`, which refuses padding, characters outside the URL-safe
//! alphabet and nonzero trailing bits): these tests hold them to it with the
//! same cases as the other SDKs. Every re-spelling of the issued token, and
//! every change in its content, is refused, and the issued token still
//! verifies afterwards.
#![cfg(feature = "auth")]
#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    reason = "test helpers"
)]

use base64::Engine;
use base64::engine::general_purpose::URL_SAFE_NO_PAD;
use flowcatalyst_sdk::auth::{HmacTokenValidator, TokenValidator, TokenValidatorConfig};
use jsonwebtoken::{Algorithm, EncodingKey, Header, encode};
use tokio::io::{AsyncReadExt, AsyncWriteExt};

const ALPHABET: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";

/// The field fixtures' PUBLIC, TEST-ONLY RSA-2048 key (tools/field-fixtures);
/// it protects nothing.
const FIXTURE_RSA_KEY: &str = "-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQCwjyq4QciYbnQD
QZOSsL6iwzyTMwAANR/QLqrW1IQb2cRNEN/J+W45cy5I8KkqA/d3IYSvU77VRLwK
wiswzbu5YXI927bMAzuyztDoSL+rP5+O1OXgoTmdTju6ayOD3ka+RVfSVL/J6k77
WITqnT18cVbIJq8hqJjbWXcQR+Uv57IW8fcFGiPveJqHUDn8e5L5yEKObCy7w7Cp
+ZU7TepPHl3lbhfncA5nx0x2YCrtUhGyooCISecb49zaujoC778Lgd94yWt2IJzH
6KcSfRlG3EWGHt6vIXkosQcpjHjxE7CEehDwKspnMz2rYnEQQf82ldFXF9Y/blqZ
N1YMFOSvAgMBAAECggEAFNjtcruUblM/GudSVSl4yQqD4N75FWKFyB91MqVTQrVF
lX1JlLVWti0104z/HHlCw2v6GSL/+c9XjhmmiekGTSRcHeu55ir3kJcVBGhv9qOD
jpTwM8Q1uQT0zn/kvfxanrrcsiF+/TN6cfkzcJRydHrZhAv1So8ixWZcmwKHkE6j
1flDhPpRzVdf7/4LAg6xn5mnCD25iMoHlO/3QcrefEOYgg2oD6NqHOLxy5Ky2C9i
ZqzBAONs0TOVt+AcLuAlYmOHkCY+rzqwAJjl1t/JLUJp5PlCq6Ah1663Scic7pQB
XASrrMOLD0uc2uHoOERCr9SX1hm2x1KIszIdMYpGwQKBgQDtzIAs/igTct5kQWHs
uuDBN6Evw8mAMTMO8Nf9oAZqtPmUqq/apVsRYw9H9M3FWHxYaSjEeNIOq0kG3XBm
gKv5P315NhXv9SuKkeOmagCyAXozRRncFCgWSnbfakDCHo/IxrCGXEDT+OPo4wkQ
UJpFTxLXLAn08d/FJBXiXVfFIQKBgQC+ErhQmNIad3gEmteV3jmfsg357aqWiSVV
l2cJOrg4zVdMgrw6gKhSb/gNjbJXGze3KvUspodZQG9hN3D4q+d3EOWaBcUypwLs
w4PKqiysiEcMmFyzT7rjxd8PLDz9syfRYIn6kXg7en6zIJY8CGimuYpVHCCFM4h5
Ux4QjA2fzwKBgANMW9bD67BhXn5Lfd03tlhXui+capYRmKCzbHuDFbOzMOcD+w5w
TrQOCK850QTUtlzPkJYxCtDTRV8IAUAatsftPoNI4cMDRohUf7DWNY5CgPxnD/XC
fc/ZVo7AvYczv1DMmLQ0YV3PTPH0/DIyvAEMGLnNTZINLMpiNr9+BfKhAoGABVed
WpaRATFubCK2AH5YfEaS4f1uGAEOzZMN0FKYLvCGjZ33EGXhVT/UZv0ekBgxvrI0
ZebgUv/eoYrfX/3k6ZHOAkA7RKsxB9qO77lMvCSwaog1wsxw4koDdPe3VyIb8mSW
PDaufG169inqTxGlb1ozqyS5PUUvhslxROHK2D0CgYEA2IDwFBAhO41i+LCiQmBu
uCH7s8GgHkWJsC6ys2c4kKkGolMubozsdeYe5i0ce6/iMEcGR9nXHm9MApdM8gbl
tPxoa5Ze6qll9lNONCbe5wDwNVLS+Pf4v1qmBTM1uwsXHpzhQ1eBDlYxI6kymp/Y
cdH5S85GpJToxgXdNYF7Jy8=
-----END PRIVATE KEY-----
";
const FIXTURE_N: &str = "sI8quEHImG50A0GTkrC-osM8kzMAADUf0C6q1tSEG9nETRDfyfluOXMuSPCpKgP3dyGEr1O-1US8CsIrMM27uWFyPdu2zAM7ss7Q6Ei_qz-fjtTl4KE5nU47umsjg95GvkVX0lS_yepO-1iE6p09fHFWyCavIaiY21l3EEflL-eyFvH3BRoj73iah1A5_HuS-chCjmwsu8OwqfmVO03qTx5d5W4X53AOZ8dMdmAq7VIRsqKAiEnnG-Pc2ro6Au-_C4HfeMlrdiCcx-inEn0ZRtxFhh7eryF5KLEHKYx48ROwhHoQ8CrKZzM9q2JxEEH_NpXRVxfWP25amTdWDBTkrw";
const FIXTURE_E: &str = "AQAB";

fn spare_bits(n: usize) -> u32 {
    match n % 4 {
        2 => 4,
        3 => 2,
        _ => 0,
    }
}

/// Every other last character of `seg` with the same significant bits.
fn non_canonical_spellings(seg: &str) -> Vec<String> {
    let mask = (1usize << spare_bits(seg.len())) - 1;
    let last = ALPHABET
        .iter()
        .position(|&c| c == *seg.as_bytes().last().unwrap())
        .unwrap();
    (1..=mask)
        .map(|low| {
            format!(
                "{}{}",
                &seg[..seg.len() - 1],
                ALPHABET[(last & !mask) | low] as char
            )
        })
        .collect()
}

fn lenient(s: &str) -> Vec<u8> {
    let cleaned: String = s.trim_end_matches('=').chars().collect();
    base64::engine::GeneralPurpose::new(
        &base64::alphabet::URL_SAFE,
        base64::engine::GeneralPurposeConfig::new()
            .with_decode_allow_trailing_bits(true)
            .with_decode_padding_mode(base64::engine::DecodePaddingMode::Indifferent),
    )
    .decode(cleaned)
    .unwrap()
}

/// The issued token re-spelled: every other last character of the signature,
/// '==' and (where two belong) '=' padding, the standard alphabet, LF / CR /
/// CRLF inside the signature, a space before or after it, CRLF after it.
fn respellings(issued: &str) -> Vec<(String, String)> {
    let parts: Vec<&str> = issued.split('.').collect();
    let (head, sig) = (format!("{}.{}.", parts[0], parts[1]), parts[2]);
    let bytes = lenient(sig);
    let mut out = Vec::new();
    for v in non_canonical_spellings(sig) {
        assert_eq!(lenient(&v), bytes, "test bug: {v} is not another spelling");
        out.push((
            format!("signature ends in {}", v.chars().last().unwrap()),
            format!("{head}{v}"),
        ));
    }
    let pad = "=".repeat((4 - sig.len() % 4) % 4);
    if !pad.is_empty() {
        out.push(("signature padded".into(), format!("{head}{sig}{pad}")));
    }
    if pad.len() == 2 {
        out.push(("signature padded once".into(), format!("{head}{sig}=")));
    }
    let std = sig.replace('-', "+").replace('_', "/");
    if std != sig {
        out.push((
            "signature in the standard alphabet".into(),
            format!("{head}{std}"),
        ));
    }
    let mid = sig.len() / 2;
    for (name, brk) in [
        ("line feed", "\n"),
        ("carriage return", "\r"),
        ("CRLF", "\r\n"),
    ] {
        out.push((
            format!("{name} inside the signature"),
            format!("{head}{}{brk}{}", &sig[..mid], &sig[mid..]),
        ));
    }
    out.push(("leading space".into(), format!(" {issued}")));
    out.push(("trailing space".into(), format!("{issued} ")));
    out.push(("CRLF after it".into(), format!("{issued}\r\n")));
    out
}

/// The issued token changed in content.
fn tampered(issued: &str, claim: &str, other: &str) -> Vec<(String, String)> {
    let p: Vec<&str> = issued.split('.').collect();
    let sig = lenient(p[2]);
    let payload = String::from_utf8(lenient(p[1])).unwrap();
    assert!(
        payload.contains(claim) && claim != other,
        "test bug: {claim} must occur in the payload"
    );
    let mut first = sig.clone();
    first[0] ^= 0x80;
    let mut last = sig.clone();
    *last.last_mut().unwrap() ^= 0x01;
    let swapped = if p[1].starts_with('A') { 'B' } else { 'A' };
    vec![
        (
            "first signature bit flipped".into(),
            format!("{}.{}.{}", p[0], p[1], URL_SAFE_NO_PAD.encode(&first)),
        ),
        (
            "last signature bit flipped".into(),
            format!("{}.{}.{}", p[0], p[1], URL_SAFE_NO_PAD.encode(&last)),
        ),
        (
            "payload claim changed".into(),
            format!(
                "{}.{}.{}",
                p[0],
                URL_SAFE_NO_PAD.encode(payload.replace(claim, other)),
                p[2]
            ),
        ),
        (
            "payload character changed".into(),
            format!("{}.{swapped}{}.{}", p[0], &p[1][1..], p[2]),
        ),
        (
            "signature truncated by one byte".into(),
            format!(
                "{}.{}.{}",
                p[0],
                p[1],
                URL_SAFE_NO_PAD.encode(&sig[..sig.len() - 1])
            ),
        ),
        (
            "truncated by one character".into(),
            issued[..issued.len() - 1].to_string(),
        ),
        ("signature removed".into(), format!("{}.{}.", p[0], p[1])),
        (
            "signature segment removed".into(),
            format!("{}.{}", p[0], p[1]),
        ),
    ]
}

fn claims(iss: &str, aud: &str) -> serde_json::Value {
    let now = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_secs() as i64;
    serde_json::json!({
        "sub": "prn_canonical", "iss": iss, "aud": aud, "exp": now + 3600, "iat": now, "nbf": now,
        "jti": "j1", "type": "SERVICE", "tier": "CLIENT", "scope": "", "name": "Canon",
        "clients": ["clt_1"], "roles": [], "applications": [], "token_use": "api"
    })
}

async fn assert_only_the_issued_spelling<F, Fut>(token: &str, other_last_chars: usize, accepts: F)
where
    F: Fn(String) -> Fut,
    Fut: std::future::Future<Output = bool>,
{
    assert!(
        accepts(token.to_string()).await,
        "the issued token must verify"
    );
    let cases = respellings(token);
    let spare = cases
        .iter()
        .filter(|(n, _)| n.starts_with("signature ends in"))
        .count();
    assert_eq!(
        spare, other_last_chars,
        "other spellings of the signature's last character"
    );
    for (name, bad) in cases {
        assert_ne!(bad, token);
        assert!(!accepts(bad).await, "re-spelled ({name}) must be refused");
    }
    for (name, bad) in tampered(token, "prn_canonical", "prn_somebody") {
        assert_ne!(bad, token);
        assert!(!accepts(bad).await, "tampered ({name}) must be refused");
    }
    assert!(
        accepts(token.to_string()).await,
        "the issued token must still verify"
    );
}

#[tokio::test]
async fn hmac_validator_accepts_only_the_issued_spelling() {
    // HS256: a 32-byte signature, 43 characters, 2 spare bits.
    let secret = "shared-secret-for-testing-12345!";
    let v = HmacTokenValidator::new(secret, "fc", "fc");
    let token = encode(
        &Header::new(Algorithm::HS256),
        &claims("fc", "fc"),
        &EncodingKey::from_secret(secret.as_bytes()),
    )
    .unwrap();
    assert_only_the_issued_spelling(&token, 3, |t| {
        let ok = v.validate(&t).is_ok();
        async move { ok }
    })
    .await;
}

/// Serves OIDC discovery and the fixture key's JWKS on 127.0.0.1 (a port the
/// OS picks).
async fn fake_issuer() -> String {
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let base = format!("http://{}", listener.local_addr().unwrap());
    let b = base.clone();
    tokio::spawn(async move {
        loop {
            let (mut sock, _) = listener.accept().await.unwrap();
            let b = b.clone();
            tokio::spawn(async move {
                let mut req = String::new();
                let mut buf = [0u8; 2048];
                while !req.contains("\r\n\r\n") {
                    let n = sock.read(&mut buf).await.unwrap_or(0);
                    if n == 0 {
                        break;
                    }
                    req.push_str(&String::from_utf8_lossy(&buf[..n]));
                }
                let body = if req.contains("openid-configuration") {
                    format!(r#"{{"issuer":"{b}","jwks_uri":"{b}/jwks"}}"#)
                } else {
                    format!(
                        r#"{{"keys":[{{"kty":"RSA","kid":"fixture-key","use":"sig","alg":"RS256","n":"{FIXTURE_N}","e":"{FIXTURE_E}"}}]}}"#
                    )
                };
                let resp = format!(
                    "HTTP/1.1 200 OK\r\ncontent-type: application/json\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{body}",
                    body.len()
                );
                sock.write_all(resp.as_bytes()).await.ok();
                sock.shutdown().await.ok();
            });
        }
    });
    base
}

#[tokio::test]
async fn token_validator_accepts_only_the_issued_spelling() {
    // RS256 with a 2048-bit key: a 256-byte signature, 342 characters, 4 spare bits.
    let base = fake_issuer().await;
    let v = TokenValidator::new(TokenValidatorConfig {
        issuer_url: base.clone(),
        audience: "my-app".into(),
        ..Default::default()
    });
    let mut header = Header::new(Algorithm::RS256);
    header.kid = Some("fixture-key".into());
    let key = EncodingKey::from_rsa_pem(FIXTURE_RSA_KEY.as_bytes()).unwrap();
    let token = encode(&header, &claims(&base, "my-app"), &key).unwrap();
    let v = &v;
    assert_only_the_issued_spelling(&token, 15, |t| async move { v.validate(&t).await.is_ok() })
        .await;
    assert_only_the_issued_spelling(&token, 15, |t| async move {
        v.validate_bearer(&format!("Bearer {t}")).await.is_ok()
    })
    .await;
}
