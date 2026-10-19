//! JS on V8 via deno_core, one isolate per function (the Rust repo's
//! spikes/fnhost-density/v8 model). Runs the same bundle as QuickJS-in-wazero:
//! the three natives (__fc_call, __fc_utf8_encode, __fc_utf8_decode) are
//! deno_core ops. Handlers are async (as the fn-ts SDK's are); the host drains
//! microtasks and reads the settled promise.
//!
//!   v8bench lat  --script f.js [--runs 5] [--tput 1,4,14]
//!   v8bench cold --script f.js
//!   v8bench mem  --script f.js

#[path = "../common.rs"]
mod common;
use common::*;

use deno_core::{op2, v8, Extension, JsRuntime, JsRuntimeForSnapshot, RuntimeOptions};
use deno_core::serde_v8::ToJsBuffer;
use serde::Serialize;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};

#[derive(Serialize)]
struct CallResult {
    code: u32,
    meta: String,
    body: ToJsBuffer,
}

#[op2]
#[serde]
fn op_fc_call(#[smi] op: u32, #[string] meta: String, #[buffer] body: &[u8]) -> CallResult {
    if std::env::var_os("SMOKE").is_some() {
        println!("host op={op} meta={meta} body={}B", body.len());
    }
    let (m, b) = host_op(op);
    CallResult { code: 0, meta: String::from_utf8_lossy(m).into_owned(), body: b.to_vec().into() }
}

#[op2]
#[buffer]
fn op_utf8_encode(#[string] s: String) -> Vec<u8> {
    s.into_bytes()
}

#[op2]
#[string]
fn op_utf8_decode(#[buffer] b: &[u8]) -> String {
    String::from_utf8_lossy(b).into_owned()
}

const BOOT: &str = r#"
const EMPTY = new Uint8Array(0);
globalThis.__fc_call = (op, meta, body) => Deno.core.ops.op_fc_call(op, meta, body ?? EMPTY);
globalThis.__fc_utf8_encode = (s) => Deno.core.ops.op_utf8_encode(s);
globalThis.__fc_utf8_decode = (b) => Deno.core.ops.op_utf8_decode(b);
"#;

fn ext() -> Extension {
    Extension {
        name: "fcbench",
        ops: std::borrow::Cow::Owned(vec![op_fc_call(), op_utf8_encode(), op_utf8_decode()]),
        ..Default::default()
    }
}

struct Func {
    rt: JsRuntime,
    handle: v8::Global<v8::Function>,
}

fn get_handle(rt: &mut JsRuntime) -> v8::Global<v8::Function> {
    let g = rt.execute_script("[get]", "globalThis.__fc.handle".to_string()).expect("globalThis.__fc.handle");
    deno_core::scope!(scope, rt);
    let v = v8::Local::new(scope, g);
    v8::Global::new(scope, v8::Local::<v8::Function>::try_from(v).expect("handle is a function"))
}

fn load(script: &str, snap: Option<&'static [u8]>) -> Func {
    let mut rt = JsRuntime::new(RuntimeOptions { extensions: vec![ext()], startup_snapshot: snap, ..Default::default() });
    if snap.is_none() {
        rt.execute_script("[boot]", BOOT.to_string()).unwrap();
        rt.execute_script("[function]", script.to_string()).unwrap();
    }
    let handle = get_handle(&mut rt);
    Func { rt, handle }
}

fn snapshot(script: &str) -> &'static [u8] {
    let mut rt = JsRuntimeForSnapshot::new(RuntimeOptions { extensions: vec![ext()], ..Default::default() });
    rt.execute_script("[boot]", BOOT.to_string()).unwrap();
    rt.execute_script("[function]", script.to_string()).unwrap();
    Box::leak(rt.snapshot())
}

/// One handle() call: request frame in, (meta, body) out.
fn call(f: &mut Func, i: &Input) -> (String, Vec<u8>) {
    let handle = &f.handle;
    deno_core::scope!(scope, &mut f.rt);
    let func = v8::Local::new(scope, handle);
    let meta = v8::String::new(scope, std::str::from_utf8(&i.meta).unwrap()).unwrap();
    let store = v8::ArrayBuffer::new_backing_store_from_vec(i.body.clone()).make_shared();
    let ab = v8::ArrayBuffer::with_backing_store(scope, &store);
    let body = v8::Uint8Array::new(scope, ab, 0, i.body.len()).unwrap();
    let undef = v8::undefined(scope);
    let mut r = func.call(scope, undef.into(), &[meta.into(), body.into()]).expect("handle threw");
    if r.is_promise() {
        scope.perform_microtask_checkpoint();
        let p = v8::Local::<v8::Promise>::try_from(r).unwrap();
        assert_eq!(p.state(), v8::PromiseState::Fulfilled, "promise not fulfilled");
        r = p.result(scope);
    }
    let o = v8::Local::<v8::Object>::try_from(r).unwrap();
    let k = v8::String::new(scope, "meta").unwrap();
    let m = o.get(scope, k.into()).unwrap().to_rust_string_lossy(scope);
    let k = v8::String::new(scope, "body").unwrap();
    let b = o.get(scope, k.into()).unwrap();
    let bytes = if b.is_string() {
        b.to_rust_string_lossy(scope).into_bytes()
    } else {
        let u = v8::Local::<v8::Uint8Array>::try_from(b).unwrap();
        let mut v = vec![0u8; u.byte_length()];
        u.copy_contents(&mut v);
        v
    };
    (m, bytes)
}

fn check(f: &mut Func, i: &Input) {
    let (_, b) = call(f, i);
    assert_eq!(b, i.expect, "w{}: output mismatch: got {:.100}", i.w, String::from_utf8_lossy(&b));
}

fn tput(script: &str, snap: Option<&'static [u8]>, i: &Input, c: usize, secs: f64) -> f64 {
    let stop = Arc::new(AtomicBool::new(false));
    let n = Arc::new(AtomicU64::new(0));
    let ready = Arc::new(std::sync::Barrier::new(c + 1));
    let hs: Vec<_> = (0..c)
        .map(|_| {
            let (stop, n, ready, script) = (stop.clone(), n.clone(), ready.clone(), script.to_string());
            let inp = Input { w: i.w, meta: i.meta.clone(), body: i.body.clone(), expect: vec![] };
            std::thread::spawn(move || {
                let trt = tokio::runtime::Builder::new_current_thread().enable_all().build().unwrap();
                let _g = trt.enter();
                let mut f = load(&script, snap);
                let t0 = Instant::now();
                while t0.elapsed() < Duration::from_millis(300) {
                    call(&mut f, &inp);
                }
                ready.wait();
                let mut local = 0;
                while !stop.load(Ordering::Relaxed) {
                    call(&mut f, &inp);
                    local += 1;
                }
                n.fetch_add(local, Ordering::Relaxed);
                std::mem::forget(f);
                std::mem::forget(trt);
            })
        })
        .collect();
    ready.wait();
    let t0 = Instant::now();
    std::thread::sleep(Duration::from_secs_f64(secs));
    stop.store(true, Ordering::Relaxed);
    for h in hs {
        h.join().unwrap();
    }
    n.load(Ordering::Relaxed) as f64 / t0.elapsed().as_secs_f64()
}

fn main() {
    let cmd = std::env::args().nth(1).unwrap_or_default();
    let script_path = arg("--script").expect("--script");
    let script = std::fs::read_to_string(&script_path).unwrap();
    let inputs = inputs();
    JsRuntime::init_platform(None);
    let trt = tokio::runtime::Builder::new_current_thread().enable_all().build().unwrap();
    let _g = trt.enter();
    match cmd.as_str() {
        "lat" => {
            let runs: usize = arg("--runs").map(|s| s.parse().unwrap()).unwrap_or(5);
            let conc: Vec<usize> = arg("--tput").unwrap_or("1,4,14".into()).split(',').map(|s| s.parse().unwrap()).collect();
            let only: Vec<u8> = arg("--w").unwrap_or("1,2,3,4,5".into()).split(',').map(|s| s.parse().unwrap()).collect();
            for i in inputs.iter().filter(|i| only.contains(&i.w)) {
                let mut f = load(&script, None);
                check(&mut f, i);
                measure_latency("v8", i.w, runs, Duration::from_millis(500), Duration::from_millis(1500), || {
                    call(&mut f, i);
                });
                std::mem::forget(f);
                for &c in &conc {
                    let mut r: Vec<f64> = (0..3).map(|_| tput(&script, None, i, c, 3.0)).collect();
                    r.sort_by(|a, b| a.partial_cmp(b).unwrap());
                    println!("RESULT rt=v8 w={} metric=tput c={c} rps={:.0} rps_min={:.0} rps_max={:.0}", i.w, r[1], r[0], r[2]);
                }
            }
            std::process::exit(0);
        }
        "smoke" => {
            // A REAL bundle's describe and one request, as wazerobench -mode smoke does.
            let mut f = load(&script, None);
            let d = f.rt.execute_script("[describe]", "globalThis.__fc.describe()".to_string()).unwrap();
            {
                deno_core::scope!(scope, &mut f.rt);
                let v = v8::Local::new(scope, d);
                println!("describe err=<nil>\n{}", v.to_rust_string_lossy(scope));
            }
            let method = arg("--method").unwrap_or("GET".into());
            let path = arg("--path").unwrap_or("/healthz".into());
            let caller = arg("--caller").unwrap_or("anonymous".into());
            let body = arg("--body").map(|p| std::fs::read(p).unwrap()).unwrap_or_default();
            let meta = format!(r#"{{"id":"inv_1","address":"x.y","version":1,"method":"{method}","path":"{path}","rawQuery":"","headers":{{"content-type":["application/xml"]}},"route":"{method} {path}","pathParams":{{}},"caller":{{"kind":"{caller}"}},"deadlineUnixMs":9999999999999}}"#);
            let inp = Input { w: 0, meta: meta.into_bytes(), body, expect: vec![] };
            let (m, b) = call(&mut f, &inp);
            println!("handle err=<nil>\nmeta={m}\nbody={}", String::from_utf8_lossy(&b));
            std::process::exit(0);
        }
        "cold" => {
            // New isolate + evaluate the bundle + first call, with and without a startup snapshot
            // that already holds the evaluated bundle (V8's analogue of a compile cache).
            let w1 = &inputs[0];
            let reps = 15;
            let mut cold = vec![];
            let mut parts = vec![];
            for _ in 0..reps {
                let s = Instant::now();
                let mut f = load(&script, None);
                let t_load = s.elapsed();
                let s2 = Instant::now();
                call(&mut f, w1);
                let t_first = s2.elapsed();
                cold.push((t_load + t_first).as_secs_f64() * 1e3);
                parts.push((t_load.as_secs_f64() * 1e3, t_first.as_secs_f64() * 1e3));
                std::mem::forget(f);
            }
            let t = Instant::now();
            let snap = snapshot(&script);
            let snap_ms = t.elapsed().as_secs_f64() * 1e3;
            let mut sc = vec![];
            let mut sparts = vec![];
            for _ in 0..reps {
                let s = Instant::now();
                let mut f = load(&script, Some(snap));
                let t_load = s.elapsed();
                let s2 = Instant::now();
                call(&mut f, w1);
                let t_first = s2.elapsed();
                sc.push((t_load + t_first).as_secs_f64() * 1e3);
                sparts.push((t_load.as_secs_f64() * 1e3, t_first.as_secs_f64() * 1e3));
                std::mem::forget(f);
            }
            let l: Vec<f64> = parts.iter().map(|p| p.0).collect();
            let fc: Vec<f64> = parts.iter().map(|p| p.1).collect();
            let sl: Vec<f64> = sparts.iter().map(|p| p.0).collect();
            let sf: Vec<f64> = sparts.iter().map(|p| p.1).collect();
            println!("RESULT rt=v8 metric=cold size_bytes={} no_snapshot: create_and_eval_ms={:.2} first_call_ms={:.3} total_ms={:.2} | snapshot ({snap_ms:.1} ms to build, {} KB): create_ms={:.2} first_call_ms={:.3} total_ms={:.2} (medians of {reps})",
                script.len(), median(&l), median(&fc), median(&cold), snap.len() / 1024, median(&sl), median(&sf), median(&sc));
            std::process::exit(0);
        }
        "mem" => {
            let base = rss_mb();
            let engine_probe = { let f = load(&script, None); std::mem::forget(f); rss_mb() };
            // engine_probe includes one loaded isolate; measure incremental from there.
            let n = 40;
            let mut fs = vec![];
            for _ in 0..n {
                let mut f = load(&script, None);
                for i in &inputs[..3] {
                    call(&mut f, i);
                }
                fs.push(f);
            }
            std::thread::sleep(Duration::from_millis(300));
            let after = rss_mb();
            println!("RESULT rt=v8 metric=mem process_base_mb={base:.1} first_isolate_loaded_mb={:.1} per_extra_warm_isolate_mb={:.2} (n={n}, each ran W1-W3)", engine_probe - base, (after - engine_probe) / n as f64);
            std::process::exit(0);
        }
        _ => eprintln!("usage: v8bench lat|cold|mem --script f.js"),
    }
}
