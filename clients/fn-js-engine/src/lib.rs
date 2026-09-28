//! The function runner's shared JavaScript engine (docs/function-runner-plan.md
//! §2.2, phase 3): QuickJS compiled once per runner, speaking ABI v1. A JS
//! function's artifact is its bundled script; the runner instantiates this
//! engine and hands the script to `fc_js_load` before the first call.
//!
//! The script must define `globalThis.__fc`:
//!
//! ```js
//! globalThis.__fc = {
//!   describe(): string,                                   // the describe JSON
//!   handle(meta: string, body: Uint8Array):               // request frame
//!     { meta: string, body: Uint8Array | string }         // response frame
//!     | Promise<{ meta: string, body: Uint8Array | string }>,
//! };
//! ```
//!
//! and may call the natives `__fc_call(op, meta, body)` (→ `{code, meta, body}`),
//! `__fc_utf8_encode(string)` and `__fc_utf8_decode(bytes)`. The
//! `@flowcatalyst/fn` SDK wraps all of this.
use rquickjs::{Context, Ctx, Function, Object, Promise, Runtime, TypedArray, Value};
use std::cell::RefCell;
use std::io::Write;

#[link(wasm_import_module = "fc")]
extern "C" {
    fn call(op: u32, ptr: u32, len: u32) -> u64;
    fn take(dst: u32);
}

thread_local! {
    static ENGINE: RefCell<Option<(Runtime, Context)>> = const { RefCell::new(None) };
    static OUT: RefCell<Vec<u8>> = const { RefCell::new(Vec::new()) };
}

const MAX_STACK: usize = 512 << 10;
const FAILED: &[u8] = br#"{"error":"FUNCTION_FAILED"}"#;

fn frame(meta: &[u8], body: &[u8]) -> Vec<u8> {
    let mut f = Vec::with_capacity(4 + meta.len() + body.len());
    f.extend_from_slice(&(meta.len() as u32).to_le_bytes());
    f.extend_from_slice(meta);
    f.extend_from_slice(body);
    f
}

fn split(f: &[u8]) -> Option<(&[u8], &[u8])> {
    if f.len() < 4 {
        return None;
    }
    let n = u32::from_le_bytes([f[0], f[1], f[2], f[3]]) as usize;
    if n > f.len() - 4 {
        return None;
    }
    Some((&f[4..4 + n], &f[4 + n..]))
}

fn out(v: Vec<u8>) -> u64 {
    OUT.with(|o| {
        *o.borrow_mut() = v;
        let o = o.borrow();
        ((o.as_ptr() as u64) << 32) | o.len() as u64
    })
}

fn stderr(msg: &str) {
    let _ = writeln!(std::io::stderr(), "{msg}");
}

/// Describes a JS exception with its stack, for the runner's log.
fn exception(ctx: &Ctx<'_>, err: rquickjs::Error) -> String {
    if let rquickjs::Error::Exception = err {
        let v = ctx.catch();
        if let Some(ex) = v.as_exception() {
            return format!("{}\n{}", ex.message().unwrap_or_default(), ex.stack().unwrap_or_default());
        }
        if let Ok(s) = v.get::<rquickjs::Coerced<String>>() {
            return s.0;
        }
    }
    err.to_string()
}

#[no_mangle]
pub extern "C" fn fc_abi_v1() {}

#[no_mangle]
pub extern "C" fn fc_alloc(n: u32) -> *mut u8 {
    let mut v = Vec::<u8>::with_capacity(n as usize);
    let p = v.as_mut_ptr();
    std::mem::forget(v);
    p
}

fn host_call<'js>(ctx: Ctx<'js>, op: u32, meta: String, body: Option<TypedArray<'js, u8>>) -> rquickjs::Result<Object<'js>> {
    let body = body.as_ref().and_then(|b| b.as_bytes()).unwrap_or(&[]);
    let f = frame(meta.as_bytes(), body);
    let r = unsafe { call(op, f.as_ptr() as u32, f.len() as u32) };
    let (code, n) = ((r >> 32) as u32, r as u32);
    let mut res = vec![0u8; n as usize];
    if n > 0 {
        unsafe { take(res.as_mut_ptr() as u32) }
    }
    let o = Object::new(ctx.clone())?;
    o.set("code", code)?;
    let (m, b) = split(&res).unwrap_or((b"{}", b""));
    o.set("meta", String::from_utf8_lossy(m).into_owned())?;
    o.set("body", TypedArray::<u8>::new(ctx, b.to_vec())?)?;
    Ok(o)
}

fn utf8_encode<'js>(ctx: Ctx<'js>, s: String) -> rquickjs::Result<TypedArray<'js, u8>> {
    TypedArray::new(ctx, s.into_bytes())
}

fn utf8_decode(b: TypedArray<'_, u8>) -> String {
    String::from_utf8_lossy(b.as_bytes().unwrap_or(&[])).into_owned()
}

/// Loads the function's script. Returns 0 on success; 1 when the engine could
/// not start or the script threw (the reason goes to stderr, the runner's log).
///
/// # Safety
/// `ptr`/`len` must be a buffer this module's `fc_alloc` returned.
#[no_mangle]
pub unsafe extern "C" fn fc_js_load(ptr: *mut u8, len: u32) -> u32 {
    let src = Vec::from_raw_parts(ptr, len as usize, len as usize);
    let Ok(rt) = Runtime::new() else { return 1 };
    rt.set_max_stack_size(MAX_STACK);
    let Ok(ctx) = Context::full(&rt) else { return 1 };
    let ok = ctx.with(|ctx| {
        let g = ctx.globals();
        let natives = (|| -> rquickjs::Result<()> {
            g.set("__fc_call", Function::new(ctx.clone(), host_call)?)?;
            g.set("__fc_utf8_encode", Function::new(ctx.clone(), utf8_encode)?)?;
            g.set("__fc_utf8_decode", Function::new(ctx.clone(), utf8_decode)?)?;
            Ok(())
        })();
        if natives.is_err() {
            return false;
        }
        match ctx.eval::<Value, _>(src) {
            Ok(_) => {
                while ctx.execute_pending_job() {}
                if g.get::<_, Object>("__fc").is_err() {
                    stderr("the script did not define globalThis.__fc");
                    return false;
                }
                true
            }
            Err(e) => {
                stderr(&format!("the script failed to load: {}", exception(&ctx, e)));
                false
            }
        }
    });
    ENGINE.with(|e| *e.borrow_mut() = Some((rt, ctx)));
    if ok {
        0
    } else {
        1
    }
}

fn with_fc<R>(f: impl for<'js> FnOnce(Ctx<'js>, Object<'js>) -> R, missing: R) -> R {
    ENGINE.with(|e| {
        let e = e.borrow();
        let Some((_, ctx)) = e.as_ref() else {
            stderr("fc_js_load was not called");
            return missing;
        };
        ctx.with(|ctx| match ctx.globals().get::<_, Object>("__fc") {
            Ok(fc) => f(ctx, fc),
            Err(_) => missing,
        })
    })
}

#[no_mangle]
pub extern "C" fn fc_describe() -> u64 {
    let doc = with_fc(
        |ctx, fc| {
            let r = fc.get::<_, Function>("describe").and_then(|d| d.call::<_, String>(()));
            match r {
                Ok(s) => s.into_bytes(),
                Err(e) => {
                    stderr(&format!("describe failed: {}", exception(&ctx, e)));
                    Vec::new()
                }
            }
        },
        Vec::new(),
    );
    out(doc)
}

fn failed() -> Vec<u8> {
    frame(br#"{"status":500,"headers":{"Content-Type":["application/json"]}}"#, FAILED)
}

/// # Safety
/// `ptr`/`len` must be a buffer this module's `fc_alloc` returned.
#[no_mangle]
pub unsafe extern "C" fn fc_handle(ptr: *mut u8, len: u32) -> u64 {
    let input = Vec::from_raw_parts(ptr, len as usize, len as usize);
    let Some((meta, body)) = split(&input) else { return out(failed()) };
    let res = with_fc(
        |ctx, fc| {
            let run = || -> rquickjs::Result<Vec<u8>> {
                let h: Function = fc.get("handle")?;
                let body = TypedArray::<u8>::new(ctx.clone(), body.to_vec())?;
                let mut r: Value = h.call((String::from_utf8_lossy(meta).into_owned(), body))?;
                if let Some(p) = r.as_promise() {
                    let p: Promise = p.clone();
                    r = p.finish::<Value>()?;
                }
                let r = r.into_object().ok_or_else(|| rquickjs::Error::new_from_js("value", "response object"))?;
                let m: String = r.get("meta")?;
                let b: Value = r.get("body")?;
                let bytes = if let Ok(t) = TypedArray::<u8>::from_value(b.clone()) {
                    t.as_bytes().unwrap_or(&[]).to_vec()
                } else if let Some(s) = b.as_string() {
                    s.to_string()?.into_bytes()
                } else {
                    Vec::new()
                };
                Ok(frame(m.as_bytes(), &bytes))
            };
            match run() {
                Ok(f) => f,
                Err(e) => {
                    stderr(&format!("handle threw: {}", exception(&ctx, e)));
                    failed()
                }
            }
        },
        failed(),
    );
    out(res)
}
