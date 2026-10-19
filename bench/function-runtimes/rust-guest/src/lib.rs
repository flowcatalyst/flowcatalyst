//! Benchmark function, compiled Rust, raw ABI v1 (no SDK). Same workloads and
//! byte-identical outputs as js/workloads.ts. Request meta is `{"w":N}`.
use serde::{Deserialize, Serialize};
use std::cell::RefCell;

#[cfg(target_arch = "wasm32")]
#[link(wasm_import_module = "fc")]
extern "C" {
    fn call(op: u32, ptr: u32, len: u32) -> u64;
    fn take(dst: u32);
}
#[cfg(not(target_arch = "wasm32"))]
unsafe fn call(_: u32, _: u32, _: u32) -> u64 { 0 }
#[cfg(not(target_arch = "wasm32"))]
unsafe fn take(_: u32) {}

thread_local! { static OUT: RefCell<Vec<u8>> = const { RefCell::new(Vec::new()) }; }

fn frame(meta: &[u8], body: &[u8]) -> Vec<u8> {
    let mut f = Vec::with_capacity(4 + meta.len() + body.len());
    f.extend_from_slice(&(meta.len() as u32).to_le_bytes());
    f.extend_from_slice(meta);
    f.extend_from_slice(body);
    f
}

fn out(v: Vec<u8>) -> u64 {
    OUT.with(|o| {
        *o.borrow_mut() = v;
        let o = o.borrow();
        ((o.as_ptr() as u64) << 32) | o.len() as u64
    })
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
pub extern "C" fn fc_describe() -> u64 { out(br#"{"abi":1}"#.to_vec()) }

#[derive(Deserialize)]
struct Cust<'a> { code: &'a str }
#[derive(Deserialize)]
struct InLine<'a> { sku: &'a str, qty: i64, #[serde(rename = "unitCents")] unit_cents: i64 }
#[derive(Deserialize)]
struct Order<'a> {
    #[serde(rename = "orderId")] order_id: &'a str,
    #[serde(borrow)] customer: Cust<'a>,
    currency: &'a str,
    #[serde(borrow)] lines: Vec<InLine<'a>>,
}
#[derive(Serialize)]
struct OutLine<'a> { sku: &'a str, #[serde(rename = "amountCents")] amount_cents: i64 }
#[derive(Serialize)]
struct OutOrder<'a> {
    id: &'a str, customer: &'a str, currency: &'a str,
    #[serde(rename = "lineCount")] line_count: usize,
    #[serde(rename = "totalCents")] total_cents: i64,
    lines: Vec<OutLine<'a>>,
}

fn w2(body: &[u8]) -> Vec<u8> {
    let o: Order = serde_json::from_slice(body).unwrap();
    let mut total = 0;
    let lines: Vec<OutLine> = o.lines.iter().map(|l| { let a = l.qty * l.unit_cents; total += a; OutLine { sku: l.sku, amount_cents: a } }).collect();
    serde_json::to_vec(&OutOrder { id: o.order_id, customer: o.customer.code, currency: o.currency, line_count: lines.len(), total_cents: total, lines }).unwrap()
}

#[derive(Serialize)] struct Veh<'a> { registration: &'a str, #[serde(rename = "fleetNumber")] fleet_number: &'a str }
#[derive(Serialize)] struct Loc<'a> { code: &'a str, name: &'a str }
#[derive(Serialize)]
struct TLine<'a> { #[serde(rename = "lineNumber")] line_number: &'a str, sku: &'a str, description: &'a str, quantity: i64, uom: &'a str }
#[derive(Serialize)]
struct Trip<'a> {
    #[serde(rename = "tripId")] trip_id: &'a str, status: &'a str, vehicle: Veh<'a>, driver: Loc<'a>, origin: Loc<'a>, destination: Loc<'a>,
    #[serde(rename = "plannedStart")] planned_start: &'a str, #[serde(rename = "plannedEnd")] planned_end: &'a str, #[serde(rename = "actualStart")] actual_start: &'a str,
    lines: Vec<TLine<'a>>,
}

fn txt<'a>(n: roxmltree::Node<'a, 'a>, name: &str) -> &'a str {
    n.children().find(|c| c.has_tag_name(name)).and_then(|c| c.text()).unwrap_or("").trim()
}
fn el<'a>(n: roxmltree::Node<'a, 'a>, name: &str) -> roxmltree::Node<'a, 'a> {
    n.children().find(|c| c.has_tag_name(name)).unwrap()
}
fn loc<'a>(n: roxmltree::Node<'a, 'a>) -> Loc<'a> { Loc { code: txt(n, "Code"), name: txt(n, "Name") } }

fn w3(body: &[u8]) -> Vec<u8> {
    let s = std::str::from_utf8(body).unwrap();
    let doc = roxmltree::Document::parse(s).unwrap();
    let t = doc.root_element();
    let veh = el(t, "Vehicle");
    let lines = el(t, "Lines").children().filter(|c| c.has_tag_name("Line")).map(|l| {
        let q = el(l, "Quantity");
        TLine { line_number: l.attribute("number").unwrap_or(""), sku: txt(l, "Sku"), description: txt(l, "Description"),
                quantity: q.text().unwrap_or("").trim().parse().unwrap_or(0), uom: q.attribute("uom").unwrap_or("") }
    }).collect();
    serde_json::to_vec(&Trip {
        trip_id: txt(t, "TripNumber"), status: txt(t, "Status"),
        vehicle: Veh { registration: txt(veh, "Registration"), fleet_number: txt(veh, "FleetNumber") },
        driver: loc(el(t, "Driver")), origin: loc(el(t, "Origin")), destination: loc(el(t, "Destination")),
        planned_start: txt(t, "PlannedStart"), planned_end: txt(t, "PlannedEnd"), actual_start: txt(t, "ActualStart"), lines,
    }).unwrap()
}

fn w4() -> Vec<u8> {
    let mut x: u32 = 2463534242;
    let mut acc: u32 = 0;
    for _ in 0..5_000_000u32 {
        x ^= x << 13;
        x ^= x >> 17;
        x ^= x << 5;
        acc = acc.wrapping_add(std::hint::black_box(x));
    }
    acc.to_string().into_bytes()
}

fn host_call(op: u32, meta: &[u8]) -> (u32, usize) {
    let f = frame(meta, b"");
    let r = unsafe { call(op, f.as_ptr() as u32, f.len() as u32) };
    let (code, n) = ((r >> 32) as u32, r as u32 as usize);
    let mut res = vec![0u8; n];
    if n > 0 { unsafe { take(res.as_mut_ptr() as u32) } }
    let ml = if n >= 4 { u32::from_le_bytes([res[0], res[1], res[2], res[3]]) as usize } else { 0 };
    let _meta = if n >= 4 { String::from_utf8_lossy(&res[4..4 + ml]).into_owned() } else { String::new() };
    (code, n.saturating_sub(4 + ml))
}

fn w5() -> Vec<u8> {
    let mut sum = 0usize;
    for _ in 0..10 {
        let (c, b) = host_call(1, br#"{"level":"info","msg":"bench"}"#);
        sum += c as usize + b;
        let (c, b) = host_call(2, br#"{"key":"BENCH"}"#);
        sum += c as usize + b;
    }
    sum.to_string().into_bytes()
}

/// # Safety
/// `ptr`/`len` must be a buffer `fc_alloc` returned.
#[no_mangle]
pub unsafe extern "C" fn fc_handle(ptr: *mut u8, len: u32) -> u64 {
    let input = Vec::from_raw_parts(ptr, len as usize, len as usize);
    let ml = u32::from_le_bytes([input[0], input[1], input[2], input[3]]) as usize;
    let meta = &input[4..4 + ml];
    let body = &input[4 + ml..];
    let w = meta[5] - b'0';
    let resp = match w {
        1 => body.to_vec(),
        2 => w2(body),
        3 => w3(body),
        4 => w4(),
        5 => w5(),
        _ => Vec::new(),
    };
    out(frame(br#"{"status":200}"#, &resp))
}
