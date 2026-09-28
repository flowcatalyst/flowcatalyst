//! Raw ABI v1 guest used by the engine tests. Behaviour is chosen by the
//! request path; see the match in `fc_handle`.
use serde::Deserialize;
use std::collections::BTreeMap;

#[link(wasm_import_module = "fc")]
extern "C" {
    fn call(op: u32, ptr: u32, len: u32) -> u64;
    fn take(dst: u32);
}

#[derive(Deserialize)]
struct Req {
    path: String,
    #[serde(rename = "rawQuery", default)]
    raw_query: String,
}

static mut OUT: Vec<u8> = Vec::new();
static mut COUNTER: u64 = 0;
static mut HOLD: Vec<Vec<u8>> = Vec::new();

fn frame(meta: &str, body: &[u8]) -> Vec<u8> {
    let mut f = Vec::with_capacity(4 + meta.len() + body.len());
    f.extend_from_slice(&(meta.len() as u32).to_le_bytes());
    f.extend_from_slice(meta.as_bytes());
    f.extend_from_slice(body);
    f
}

fn split(f: &[u8]) -> (&[u8], &[u8]) {
    let n = u32::from_le_bytes([f[0], f[1], f[2], f[3]]) as usize;
    (&f[4..4 + n], &f[4 + n..])
}

unsafe fn respond(status: u16, headers: &BTreeMap<&str, String>, body: &[u8]) -> u64 {
    let mut meta = format!("{{\"status\":{status},\"headers\":{{");
    for (i, (k, v)) in headers.iter().enumerate() {
        if i > 0 {
            meta.push(',');
        }
        meta.push_str(&format!("{}:[{}]", serde_json::to_string(k).unwrap(), serde_json::to_string(v).unwrap()));
    }
    meta.push_str("}}");
    OUT = frame(&meta, body);
    let o = &*std::ptr::addr_of!(OUT);
    ((o.as_ptr() as u64) << 32) | o.len() as u64
}

fn query(q: &str, key: &str) -> Option<String> {
    q.split('&').find_map(|kv| kv.strip_prefix(&format!("{key}=")).map(|v| v.to_string()))
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

#[no_mangle]
pub unsafe extern "C" fn fc_describe() -> u64 {
    static DESCRIBE: &str = r#"{"abi":1,"endpoints":[{"path":"/{rest...}","auth":"none"}],"config":["GREETING"]}"#;
    ((DESCRIBE.as_ptr() as u64) << 32) | DESCRIBE.len() as u64
}

#[no_mangle]
pub unsafe extern "C" fn fc_handle(ptr: *mut u8, len: u32) -> u64 {
    let input = Vec::from_raw_parts(ptr, len as usize, len as usize);
    let (meta, body) = split(&input);
    let req: Req = serde_json::from_slice(meta).unwrap();
    let mut h = BTreeMap::new();
    match req.path.as_str() {
        "/echo" => {
            h.insert("x-path", req.path.clone());
            respond(200, &h, body)
        }
        "/count" => {
            COUNTER += 1;
            respond(200, &h, COUNTER.to_string().as_bytes())
        }
        "/spin" => loop {
            COUNTER = COUNTER.wrapping_add(1);
            std::hint::black_box(COUNTER);
        },
        "/grow" => {
            // Allocate and touch `mb` MiB, keeping it alive across calls.
            let mb: usize = query(&req.raw_query, "mb").and_then(|v| v.parse().ok()).unwrap_or(1);
            let v = vec![1u8; mb << 20];
            let sum: usize = v.iter().step_by(4096).map(|b| *b as usize).sum();
            HOLD.push(v);
            respond(200, &h, sum.to_string().as_bytes())
        }
        "/trap" => panic!("fixture trap"),
        "/malformed" => ((u32::MAX as u64) << 32) | 16,
        "/call" => {
            // The request body is a host-call frame; op comes from ?op=.
            let op: u32 = query(&req.raw_query, "op").and_then(|v| v.parse().ok()).unwrap_or(0);
            let r = call(op, body.as_ptr() as u32, body.len() as u32);
            let (code, n) = ((r >> 32) as u32, r as u32);
            let mut res = vec![0u8; n as usize];
            if n > 0 {
                take(res.as_mut_ptr() as u32);
            }
            let (m, b) = if n > 0 { split(&res) } else { (&b"{}"[..], &b""[..]) };
            h.insert("x-code", code.to_string());
            h.insert("x-meta", String::from_utf8_lossy(m).into_owned());
            respond(200, &h, b)
        }
        _ => respond(404, &h, b"no such fixture path"),
    }
}
