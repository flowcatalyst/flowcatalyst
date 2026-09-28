//! Turns a request frame into a `Request`/`Context`, runs it through
//! `App::route`, and turns the handler's `Result<Response, Error>` back
//! into a response frame. Used by the `export!` macro's generated
//! `#[no_mangle]` functions; not part of the public API surface directly.
//!
//! Unlike the Go SDK, this module does not recover a handler panic into a
//! soft 500: the plan's Rust section only specifies "Handler Err -> 500
//! with fixed body, logged", and a `wasm32-unknown-unknown` guest built
//! with `panic = "abort"` (the norm for wasm, and what
//! `examples/hello` uses) traps the instance on an actual Rust panic
//! rather than unwinding — which is exactly the runner's own "Trap ...
//! -> 500 FUNCTION_FAILED" outcome (plan §6.2), handled uniformly by the
//! host for any guest language. A malformed request frame is treated the
//! same way: it indicates a host/guest ABI mismatch that should never
//! happen, so this module panics rather than manufacturing a response.

use crate::app::{App, RouteMatch};
use crate::context::Context;
use crate::frame::{decode_frame, encode_frame};
use crate::types::{Invocation, Request, RequestMeta, Response, ResponseMeta, HANDLER_ERROR_BODY};
use std::cell::RefCell;
use std::collections::BTreeMap;

thread_local! {
    static ALLOC_BUF: RefCell<Vec<u8>> = const { RefCell::new(Vec::new()) };
    static OUT_BUF: RefCell<Vec<u8>> = const { RefCell::new(Vec::new()) };
}

/// Implements the `fc_alloc` export: allocates `size` bytes for the host to
/// write a request frame into, and returns their address.
pub fn alloc(size: u32) -> u32 {
    ALLOC_BUF.with(|b| {
        let mut buf = b.borrow_mut();
        *buf = vec![0u8; size as usize];
        if buf.is_empty() {
            0
        } else {
            buf.as_mut_ptr() as u32
        }
    })
}

/// Implements the `fc_describe` export.
pub fn describe(app: App) -> u64 {
    pack(app.describe_json())
}

/// Implements the `fc_handle` export: `ptr`/`len` name a frame the host
/// already wrote into a buffer this module handed out via `alloc`.
///
/// # Safety
/// `ptr`/`len` must describe a valid, live memory range (guaranteed by the
/// ABI contract: the host only ever calls this with a range it obtained
/// from this module's own `fc_alloc`).
pub unsafe fn handle(app: App, ptr: u32, len: u32) -> u64 {
    let req_bytes: &[u8] = if len == 0 {
        &[]
    } else {
        std::slice::from_raw_parts(ptr as *const u8, len as usize)
    };
    pack(handle_frame(&app, req_bytes))
}

fn pack(bytes: Vec<u8>) -> u64 {
    OUT_BUF.with(|b| {
        *b.borrow_mut() = bytes;
        let buf = b.borrow();
        if buf.is_empty() {
            0
        } else {
            ((buf.as_ptr() as u64) << 32) | (buf.len() as u64 & 0xffff_ffff)
        }
    })
}

fn handle_frame(app: &App, frame: &[u8]) -> Vec<u8> {
    let (meta, body) =
        decode_frame(frame).unwrap_or_else(|e| panic!("fn: malformed request frame: {e}"));
    let rm: RequestMeta = serde_json::from_slice(meta)
        .unwrap_or_else(|e| panic!("fn: malformed request meta: {e}"));

    let (ep, path_params) = match app.route(&rm.method, &rm.path) {
        RouteMatch::Found(ep, params) => (ep, params),
        RouteMatch::MethodMismatch => return status_frame(405, &[]),
        RouteMatch::NoMatch => return status_frame(404, &[]),
    };

    let req = Request {
        id: rm.id.clone(),
        address: rm.address.clone(),
        version: rm.version,
        method: rm.method.clone(),
        path: rm.path.clone(),
        raw_query: rm.raw_query.clone(),
        headers: rm.headers.clone(),
        route: ep.path.clone(),
        path_params,
        caller: rm.caller.clone(),
        deadline_unix_ms: rm.deadline_unix_ms,
        body: body.to_vec(),
    };
    let ctx = Context {
        caller: rm.caller,
        invocation: Invocation {
            id: rm.id,
            address: rm.address,
            version: rm.version,
            deadline_unix_ms: rm.deadline_unix_ms,
        },
    };

    match (ep.handler)(&req, &ctx) {
        Ok(resp) => response_frame(resp),
        Err(e) => {
            ctx.log_error(&format!("handler error: {e}"));
            status_frame(500, HANDLER_ERROR_BODY.as_bytes())
        }
    }
}

fn response_frame(resp: Response) -> Vec<u8> {
    let meta = ResponseMeta {
        status: resp.status,
        headers: resp.headers,
    };
    let mb = serde_json::to_vec(&meta).unwrap_or_else(|e| panic!("fn: encode response meta: {e}"));
    encode_frame(&mb, &resp.body)
}

fn status_frame(status: u16, body: &[u8]) -> Vec<u8> {
    let meta = ResponseMeta {
        status,
        headers: BTreeMap::new(),
    };
    let mb = serde_json::to_vec(&meta).expect("fn: status-only response meta is always encodable");
    encode_frame(&mb, body)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::types::{Error, Response};

    fn ok(_req: &Request, _ctx: &Context) -> Result<Response, Error> {
        Ok(Response::new(200)
            .with_header("Content-Type", "text/plain")
            .with_body(b"ok".to_vec()))
    }

    fn echo_path_param(req: &Request, _ctx: &Context) -> Result<Response, Error> {
        let name = req.path_param("name").unwrap_or_default();
        Ok(Response::new(200).with_body(format!("hello {name}").into_bytes()))
    }

    fn fails(_req: &Request, _ctx: &Context) -> Result<Response, Error> {
        Err(Error::from("nope"))
    }

    fn build_frame(method: &str, path: &str, body: &[u8]) -> Vec<u8> {
        let meta = serde_json::json!({
            "id": "inv_1", "address": "app.fn", "version": 1,
            "method": method, "path": path, "rawQuery": "",
            "headers": {}, "route": "", "pathParams": {},
            "caller": {"kind": "anonymous"}, "deadlineUnixMs": 0,
        });
        encode_frame(&serde_json::to_vec(&meta).unwrap(), body)
    }

    fn resp_of(frame: Vec<u8>) -> (ResponseMeta, Vec<u8>) {
        let (mb, body) = decode_frame(&frame).unwrap();
        (serde_json::from_slice(mb).unwrap(), body.to_vec())
    }

    #[test]
    fn dispatch_basic_route() {
        let mut app = App::new();
        app.open("GET /healthz", ok);
        let (meta, body) = resp_of(handle_frame(&app, &build_frame("GET", "/healthz", &[])));
        assert_eq!(meta.status, 200);
        assert_eq!(body, b"ok");
        assert_eq!(meta.headers.get("Content-Type").unwrap()[0], "text/plain");
    }

    #[test]
    fn dispatch_path_param() {
        let mut app = App::new();
        app.open("GET /hello/{name}", echo_path_param);
        let (meta, body) = resp_of(handle_frame(&app, &build_frame("GET", "/hello/world", &[])));
        assert_eq!(meta.status, 200);
        assert_eq!(body, b"hello world");
    }

    #[test]
    fn dispatch_404() {
        let mut app = App::new();
        app.open("GET /known", ok);
        let (meta, _) = resp_of(handle_frame(&app, &build_frame("GET", "/unknown", &[])));
        assert_eq!(meta.status, 404);
    }

    #[test]
    fn dispatch_405() {
        let mut app = App::new();
        app.open("POST /only-post", ok);
        let (meta, _) = resp_of(handle_frame(&app, &build_frame("GET", "/only-post", &[])));
        assert_eq!(meta.status, 405);
    }

    #[test]
    fn dispatch_handler_err_is_500_with_fixed_body() {
        let mut app = App::new();
        app.open("GET /fail", fails);
        let (meta, body) = resp_of(handle_frame(&app, &build_frame("GET", "/fail", &[])));
        assert_eq!(meta.status, 500);
        assert_eq!(body, HANDLER_ERROR_BODY.as_bytes());
    }

    #[test]
    #[should_panic(expected = "malformed request frame")]
    fn malformed_frame_panics() {
        let app = App::new();
        handle_frame(&app, &[0x01]); // too short to hold a length prefix
    }

    // Note on the next two tests: `describe`/`pack` return a packed
    // `ptr<<32|len` u64 (plan §5.1) whose "ptr" half is only meaningful
    // inside an actual wasm32 linear memory address space. On the (64-bit)
    // host test target, `buf.as_ptr()` is a real 64-bit address; packing it
    // into the high 32 bits of a u64 and later reinterpreting those bits as
    // a standalone pointer (as a wasm *host* would after `fc_take`, or as
    // this crate's own real ABI glue does via `unsafe impl` on wasm32) is
    // only valid when pointers are natively 32 bits. Doing that cast+deref
    // on the host target segfaults (confirmed while writing this test) --
    // so these tests check the packing arithmetic and the OUT_BUF/ALLOC_BUF
    // contents directly instead of dereferencing the packed value.

    #[test]
    fn describe_export_matches_app_len_and_content() {
        let mut app = App::new();
        app.open("GET /healthz", ok);
        let want = app.describe_json();

        let mut app2 = App::new();
        app2.open("GET /healthz", ok);
        let bytes = describe(app2);
        let len = (bytes & 0xffff_ffff) as usize;
        assert_eq!(len, want.len());
        OUT_BUF.with(|b| {
            assert_eq!(&b.borrow()[..], &want[..]);
        });
    }

    #[test]
    fn alloc_returns_writable_region() {
        let ptr = alloc(4);
        assert_ne!(ptr, 0);
        ALLOC_BUF.with(|b| {
            assert_eq!(b.borrow().len(), 4);
        });
    }

    #[test]
    fn alloc_zero_returns_zero_ptr() {
        assert_eq!(alloc(0), 0);
    }

    #[test]
    fn pack_encodes_len_in_low_32_bits() {
        let bytes = vec![1u8, 2, 3, 4, 5];
        let packed = pack(bytes.clone());
        assert_eq!((packed & 0xffff_ffff) as usize, bytes.len());
        assert_ne!(packed >> 32, 0, "ptr half should be a real (nonzero) address");
    }

    #[test]
    fn pack_empty_returns_zero() {
        assert_eq!(pack(Vec::new()), 0);
    }
}
