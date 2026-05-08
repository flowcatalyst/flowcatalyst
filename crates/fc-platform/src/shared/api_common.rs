//! Common API types and utilities

use utoipa::{ToSchema, IntoParams};
use serde::{Deserialize, Serialize};

mod string_or_number {
    use serde::{Deserialize, Deserializer, de};

    pub fn deserialize_u32_opt<'de, D>(deserializer: D) -> Result<Option<u32>, D::Error>
    where
        D: Deserializer<'de>,
    {
        #[derive(Deserialize)]
        #[serde(untagged)]
        enum StringOrNum {
            Num(u32),
            Str(String),
        }

        match Option::<StringOrNum>::deserialize(deserializer)? {
            Some(StringOrNum::Num(n)) => Ok(Some(n)),
            Some(StringOrNum::Str(s)) => s.parse().map(Some).map_err(de::Error::custom),
            None => Ok(None),
        }
    }
}

/// Standard API error response
#[derive(Debug, Serialize, ToSchema)]
pub struct ApiError {
    pub error: String,
    pub message: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub details: Option<serde_json::Value>,
}

/// Pagination parameters.
///
/// Accepts any of `size`, `pageSize`, or `limit` on the wire so Rust matches
/// both the Java-era and TS-era callers. Serializes back as camelCase.
#[derive(Debug, Deserialize, ToSchema, IntoParams)]
#[serde(rename_all = "camelCase")]
#[into_params(parameter_in = Query)]
pub struct PaginationParams {
    #[serde(default, deserialize_with = "string_or_number::deserialize_u32_opt")]
    page: Option<u32>,
    #[serde(
        default,
        alias = "limit",
        alias = "pageSize",
        alias = "page_size",
        deserialize_with = "string_or_number::deserialize_u32_opt"
    )]
    size: Option<u32>,
}

impl PaginationParams {
    pub fn page(&self) -> u32 {
        self.page.unwrap_or(0)
    }

    pub fn size(&self) -> u32 {
        self.size.unwrap_or(20)
    }
}

impl Default for PaginationParams {
    fn default() -> Self {
        Self {
            page: Some(0),
            size: Some(20),
        }
    }
}

impl PaginationParams {
    pub fn offset(&self) -> u64 {
        (self.page() as u64) * (self.size() as u64)
    }

    pub fn limit(&self) -> i64 {
        self.size() as i64
    }
}

/// Paginated response wrapper
#[derive(Debug, Serialize, ToSchema)]
pub struct PaginatedResponse<T> {
    pub data: Vec<T>,
    pub page: u32,
    pub size: u32,
    pub total: u64,
    pub total_pages: u32,
}

// ─── Cursor pagination ────────────────────────────────────────────────────
//
// Used by high-volume tables (msg_events, msg_dispatch_jobs, aud_logs,
// iam_login_attempts) where `SELECT COUNT(*)` is prohibitive. Keyset on
// `(created_at DESC, id DESC)` — the cursor encodes the last row of the
// page; the next request asks for rows strictly older than that key.
//
// Wire format is opaque base64 of "{created_at_micros}:{id}". Callers pass
// it back verbatim; the API never promises stability across major versions.

/// Query parameters for cursor-paginated list endpoints.
#[derive(Debug, Deserialize, ToSchema, IntoParams, Default)]
#[serde(rename_all = "camelCase")]
#[into_params(parameter_in = Query)]
pub struct CursorParams {
    /// Opaque cursor returned by a previous response's `nextCursor`.
    /// Omit for the first page.
    #[serde(default)]
    pub after: Option<String>,
    /// Page size. Defaults to 50, capped at 200.
    #[serde(
        default,
        alias = "limit",
        alias = "pageSize",
        alias = "page_size",
        deserialize_with = "string_or_number::deserialize_u32_opt"
    )]
    size: Option<u32>,
}

impl CursorParams {
    pub fn size(&self) -> u32 {
        self.size.unwrap_or(50).clamp(1, 200)
    }
    /// One past the page size — fetched so the repo can detect `hasMore`
    /// without a count.
    pub fn fetch_limit(&self) -> i64 {
        self.size() as i64 + 1
    }
}

/// Decoded cursor key. `created_at` is in microseconds since the Unix epoch
/// to keep the wire format compact and unambiguous across timezones.
#[derive(Debug, Clone, Copy)]
pub struct CursorKey {
    pub created_at_micros: i64,
    pub id_tail: u64,
}

#[derive(Debug)]
pub struct CursorDecodeError;

impl std::fmt::Display for CursorDecodeError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "invalid cursor")
    }
}

impl std::error::Error for CursorDecodeError {}

/// Encode a `(created_at, id)` pair as an opaque cursor. The `id` is hashed
/// to a `u64` so the cursor doesn't leak the full TSID structure and stays
/// compact; the consuming repository only needs strict ordering for the
/// keyset comparison.
pub fn encode_cursor(created_at: chrono::DateTime<chrono::Utc>, id: &str) -> String {
    use base64::Engine;
    let micros = created_at.timestamp_micros();
    // Use the raw id string in the cursor — it's already a stable, sortable
    // TSID, and including it lets the keyset comparison be exact.
    let raw = format!("{}:{}", micros, id);
    base64::engine::general_purpose::URL_SAFE_NO_PAD.encode(raw.as_bytes())
}

/// Decoded cursor for repositories. Returns the original (created_at, id)
/// pair so the keyset WHERE clause can use them directly.
pub struct DecodedCursor {
    pub created_at: chrono::DateTime<chrono::Utc>,
    pub id: String,
}

pub fn decode_cursor(cursor: &str) -> Result<DecodedCursor, CursorDecodeError> {
    use base64::Engine;
    let bytes = base64::engine::general_purpose::URL_SAFE_NO_PAD
        .decode(cursor.as_bytes())
        .map_err(|_| CursorDecodeError)?;
    let raw = std::str::from_utf8(&bytes).map_err(|_| CursorDecodeError)?;
    let (micros_str, id) = raw.split_once(':').ok_or(CursorDecodeError)?;
    let micros: i64 = micros_str.parse().map_err(|_| CursorDecodeError)?;
    let created_at = chrono::DateTime::<chrono::Utc>::from_timestamp_micros(micros)
        .ok_or(CursorDecodeError)?;
    Ok(DecodedCursor { created_at, id: id.to_string() })
}

/// Cursor-paginated response wrapper used by high-volume list endpoints.
/// No `total` — counting billions of rows isn't free, and consumers don't
/// need it for "older / newer" navigation.
#[derive(Debug, Serialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct CursorPage<T> {
    pub items: Vec<T>,
    /// True when more items exist after the last one in `items`.
    pub has_more: bool,
    /// Pass back as `?after=` to fetch the next page. `None` on the last
    /// page.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub next_cursor: Option<String>,
}

impl<T> CursorPage<T> {
    /// Build a page from a query that fetched `size + 1` rows. The repo
    /// passes a closure that extracts the cursor key from the last
    /// included row so the encoder doesn't need to know the row type.
    pub fn from_overfetch(
        mut rows: Vec<T>,
        page_size: usize,
        cursor_key: impl FnOnce(&T) -> (chrono::DateTime<chrono::Utc>, String),
    ) -> Self {
        let has_more = rows.len() > page_size;
        if has_more {
            rows.truncate(page_size);
        }
        let next_cursor = if has_more {
            rows.last().map(|row| {
                let (at, id) = cursor_key(row);
                encode_cursor(at, &id)
            })
        } else {
            None
        };
        Self { items: rows, has_more, next_cursor }
    }
}

impl<T> PaginatedResponse<T> {
    pub fn new(data: Vec<T>, page: u32, size: u32, total: u64) -> Self {
        let total_pages = ((total as f64) / (size as f64)).ceil() as u32;
        Self {
            data,
            page,
            size,
            total,
            total_pages,
        }
    }
}

/// Success response with optional message
#[derive(Debug, Serialize, ToSchema)]
pub struct SuccessResponse {
    pub success: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub message: Option<String>,
}

impl SuccessResponse {
    pub fn ok() -> Self {
        Self {
            success: true,
            message: None,
        }
    }

    pub fn with_message(message: impl Into<String>) -> Self {
        Self {
            success: true,
            message: Some(message.into()),
        }
    }
}

/// Created response with ID
#[derive(Debug, Serialize, ToSchema)]
pub struct CreatedResponse {
    pub id: String,
}

impl CreatedResponse {
    pub fn new(id: impl Into<String>) -> Self {
        Self { id: id.into() }
    }
}
