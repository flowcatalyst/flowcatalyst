//! The `fc` host import boundary (plan §5.3). `host_call` is the one seam
//! every capability wrapper in `context.rs` goes through; its wasm32 half
//! is the real ABI, its non-wasm32 half is a settable fake used by
//! `cargo test` on the host target.

#[cfg(target_arch = "wasm32")]
use crate::frame::{decode_frame, encode_frame};

/// Error codes shared across host ops (plan §5.3).
pub mod error_code {
    pub const NOT_DECLARED: &str = "NOT_DECLARED";
    pub const NOT_ALLOWED: &str = "NOT_ALLOWED";
    pub const DEADLINE: &str = "DEADLINE";
    pub const TOO_LARGE: &str = "TOO_LARGE";
    pub const UNAVAILABLE: &str = "UNAVAILABLE";
    pub const BAD_REQUEST: &str = "BAD_REQUEST";
    pub const CAPABILITY_UNAVAILABLE: &str = "CAPABILITY_UNAVAILABLE";
    pub const DB_CONSTRAINT: &str = "DB_CONSTRAINT";
    pub const DB_SYNTAX: &str = "DB_SYNTAX";
    pub const DB_TIMEOUT: &str = "DB_TIMEOUT";
    pub const DB_UNAVAILABLE: &str = "DB_UNAVAILABLE";
    pub const DB_ERROR: &str = "DB_ERROR";
    pub const DB_TX_UNKNOWN: &str = "DB_TX_UNKNOWN";
}

/// A host capability failure (plan §5.3: the error frame is `{code,
/// message}`).
#[derive(Debug, Clone, serde::Deserialize)]
pub struct HostError {
    pub code: String,
    pub message: String,
}

impl core::fmt::Display for HostError {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        write!(f, "{}: {}", self.code, self.message)
    }
}

impl HostError {
    /// True when this failure is UNAVAILABLE ("retryable" per plan §5.3).
    pub fn is_retryable(&self) -> bool {
        self.code == error_code::UNAVAILABLE
    }
}

pub(crate) mod op {
    pub const LOG: i32 = 1;
    pub const CONFIG_GET: i32 = 2;
    pub const SECRET_GET: i32 = 3;
    pub const HTTP_FETCH: i32 = 4;
    pub const EVENT_EMIT: i32 = 5;
    pub const DB_QUERY: i32 = 6;
    pub const DB_EXEC: i32 = 7;
    pub const DB_BEGIN: i32 = 8;
    pub const DB_COMMIT: i32 = 9;
    pub const DB_ROLLBACK: i32 = 10;
}

/// A local/transport failure distinct from a host-reported HostError (e.g.
/// a malformed result frame). These should not occur against a correct
/// host; they exist so `host_call`'s signature never silently drops data.
#[derive(Debug)]
pub struct TransportError(pub String);

impl core::fmt::Display for TransportError {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        write!(f, "fn: transport: {}", self.0)
    }
}

pub type HostResult = Result<(Vec<u8>, Vec<u8>), HostOutcome>;

#[derive(Debug)]
pub enum HostOutcome {
    Host(HostError),
    Transport(TransportError),
}

impl From<HostError> for HostOutcome {
    fn from(e: HostError) -> Self {
        HostOutcome::Host(e)
    }
}

// --- wasm32: the real ABI ---------------------------------------------------

#[cfg(target_arch = "wasm32")]
mod wasm_abi {
    #[link(wasm_import_module = "fc")]
    extern "C" {
        #[link_name = "call"]
        pub fn fc_call(op: i32, ptr: i32, len: i32) -> i64;
        #[link_name = "take"]
        pub fn fc_take(dst: i32);
    }
}

#[cfg(target_arch = "wasm32")]
pub(crate) fn host_call(op: i32, meta: &[u8], body: &[u8]) -> HostResult {
    let frame = encode_frame(meta, body);
    let ptr = if frame.is_empty() {
        0
    } else {
        frame.as_ptr() as i32
    };
    let packed = unsafe { wasm_abi::fc_call(op, ptr, frame.len() as i32) };
    let code = (packed >> 32) as i32;
    let result_len = (packed & 0xffff_ffff) as u32;

    if result_len == 0 {
        if code != 0 {
            return Err(HostOutcome::Transport(TransportError(
                "host call failed with no result frame".to_string(),
            )));
        }
        return Ok((Vec::new(), Vec::new()));
    }

    let mut result = vec![0u8; result_len as usize];
    unsafe { wasm_abi::fc_take(result.as_mut_ptr() as i32) };

    let (rm, rb) = match decode_frame(&result) {
        Ok(v) => v,
        Err(e) => return Err(HostOutcome::Transport(TransportError(e.to_string()))),
    };
    if code != 0 {
        return match serde_json::from_slice::<HostError>(rm) {
            Ok(e) => Err(HostOutcome::Host(e)),
            Err(_) => Err(HostOutcome::Transport(TransportError(
                String::from_utf8_lossy(rm).to_string(),
            ))),
        };
    }
    Ok((rm.to_vec(), rb.to_vec()))
}

// --- non-wasm32: a settable fake, for tests --------------------------------

#[cfg(not(target_arch = "wasm32"))]
pub type FakeHostFn = dyn Fn(i32, &[u8], &[u8]) -> HostResult;

/// One recorded call made against the fake host: (op, meta, body).
#[cfg(not(target_arch = "wasm32"))]
pub type FakeCall = (i32, Vec<u8>, Vec<u8>);

#[cfg(not(target_arch = "wasm32"))]
thread_local! {
    static FAKE_HOST: std::cell::RefCell<Box<FakeHostFn>> =
        std::cell::RefCell::new(Box::new(|_op, _meta, _body| Ok((Vec::new(), Vec::new()))));
    static FAKE_CALLS: std::cell::RefCell<Vec<FakeCall>> =
        const { std::cell::RefCell::new(Vec::new()) };
}

/// Installs `f` as the active fake host for the current thread (tests run
/// single-threaded per test function, so this is test-local in practice).
#[cfg(not(target_arch = "wasm32"))]
pub fn set_fake_host<F>(f: F)
where
    F: Fn(i32, &[u8], &[u8]) -> HostResult + 'static,
{
    FAKE_HOST.with(|h| *h.borrow_mut() = Box::new(f));
    FAKE_CALLS.with(|c| c.borrow_mut().clear());
}

/// Returns every call made to the fake host since the last `set_fake_host`.
#[cfg(not(target_arch = "wasm32"))]
pub fn fake_host_calls() -> Vec<FakeCall> {
    FAKE_CALLS.with(|c| c.borrow().clone())
}

#[cfg(not(target_arch = "wasm32"))]
pub(crate) fn host_call(op: i32, meta: &[u8], body: &[u8]) -> HostResult {
    FAKE_CALLS.with(|c| c.borrow_mut().push((op, meta.to_vec(), body.to_vec())));
    FAKE_HOST.with(|h| (h.borrow())(op, meta, body))
}
