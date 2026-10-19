//! The compiled-Rust ABI v1 guest on wasmtime (Cranelift), epoch interruption
//! on (the deadline mechanism, 1 ms tick).
//!
//!   wtbench lat|cold|mem --wasm guest.wasm

#[path = "../common.rs"]
mod common;
use common::*;

use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};
use wasmtime::*;

#[derive(Default)]
struct St {
    pending: Vec<u8>,
}

fn engine() -> Engine {
    let mut c = Config::new();
    c.epoch_interruption(true);
    let e = Engine::new(&c).unwrap();
    let e2 = e.clone();
    std::thread::spawn(move || loop {
        std::thread::sleep(Duration::from_millis(1));
        e2.increment_epoch();
    });
    e
}

fn linker(e: &Engine) -> Linker<St> {
    let mut l = Linker::new(e);
    l.func_wrap("fc", "call", |mut c: Caller<'_, St>, op: u32, _ptr: u32, _len: u32| -> u64 {
        let (m, b) = host_op(op);
        let mut f = Vec::new();
        f.extend_from_slice(&(m.len() as u32).to_le_bytes());
        f.extend_from_slice(m);
        f.extend_from_slice(b);
        let n = f.len() as u64;
        c.data_mut().pending = f;
        n
    })
    .unwrap();
    l.func_wrap("fc", "take", |mut c: Caller<'_, St>, dst: u32| {
        let p = std::mem::take(&mut c.data_mut().pending);
        let mem = c.get_export("memory").unwrap().into_memory().unwrap();
        mem.write(&mut c, dst as usize, &p).unwrap();
    })
    .unwrap();
    l
}

struct Inst {
    store: Store<St>,
    mem: Memory,
    alloc: TypedFunc<u32, u32>,
    handle: TypedFunc<(u32, u32), u64>,
}

fn instantiate(pre: &InstancePre<St>) -> Inst {
    let mut store = Store::new(pre.module().engine(), St::default());
    store.set_epoch_deadline(u64::MAX / 2);
    let i = pre.instantiate(&mut store).unwrap();
    Inst {
        mem: i.get_memory(&mut store, "memory").unwrap(),
        alloc: i.get_typed_func(&mut store, "fc_alloc").unwrap(),
        handle: i.get_typed_func(&mut store, "fc_handle").unwrap(),
        store,
    }
}

fn call(i: &mut Inst, inp: &Input) -> Vec<u8> {
    let mut f = Vec::with_capacity(4 + inp.meta.len() + inp.body.len());
    f.extend_from_slice(&(inp.meta.len() as u32).to_le_bytes());
    f.extend_from_slice(&inp.meta);
    f.extend_from_slice(&inp.body);
    let p = i.alloc.call(&mut i.store, f.len() as u32).unwrap();
    i.mem.write(&mut i.store, p as usize, &f).unwrap();
    let r = i.handle.call(&mut i.store, (p, f.len() as u32)).unwrap();
    let (ptr, n) = ((r >> 32) as usize, r as u32 as usize);
    let out = i.mem.data(&i.store)[ptr..ptr + n].to_vec();
    let ml = u32::from_le_bytes([out[0], out[1], out[2], out[3]]) as usize;
    out[4 + ml..].to_vec()
}

fn tput(pre: &InstancePre<St>, inp: &Input, c: usize, secs: f64) -> f64 {
    let stop = Arc::new(AtomicBool::new(false));
    let n = Arc::new(AtomicU64::new(0));
    let ready = Arc::new(std::sync::Barrier::new(c + 1));
    std::thread::scope(|s| {
        for _ in 0..c {
            let (stop, n, ready) = (stop.clone(), n.clone(), ready.clone());
            s.spawn(move || {
                let mut i = instantiate(pre);
                let t0 = Instant::now();
                while t0.elapsed() < Duration::from_millis(300) {
                    call(&mut i, inp);
                }
                ready.wait();
                let mut local = 0;
                while !stop.load(Ordering::Relaxed) {
                    call(&mut i, inp);
                    local += 1;
                }
                n.fetch_add(local, Ordering::Relaxed);
            });
        }
        ready.wait();
        let t0 = Instant::now();
        std::thread::sleep(Duration::from_secs_f64(secs));
        stop.store(true, Ordering::Relaxed);
        let _ = t0;
    });
    n.load(Ordering::Relaxed) as f64 / secs
}

fn main() {
    let cmd = std::env::args().nth(1).unwrap_or_default();
    let path = arg("--wasm").expect("--wasm");
    let label = arg("--label").unwrap_or("wasmtime".into());
    let bytes = std::fs::read(&path).unwrap();
    let inputs = inputs();
    let e = engine();
    match cmd.as_str() {
        "lat" => {
            let m = Module::new(&e, &bytes).unwrap();
            let pre = linker(&e).instantiate_pre(&m).unwrap();
            let runs: usize = arg("--runs").map(|s| s.parse().unwrap()).unwrap_or(5);
            let conc: Vec<usize> = arg("--tput").unwrap_or("1,4,14".into()).split(',').map(|s| s.parse().unwrap()).collect();
            for inp in &inputs {
                let mut i = instantiate(&pre);
                assert_eq!(call(&mut i, inp), inp.expect, "w{} output mismatch", inp.w);
                measure_latency(&label, inp.w, runs, Duration::from_millis(500), Duration::from_millis(1500), || {
                    call(&mut i, inp);
                });
                for &c in &conc {
                    let mut r: Vec<f64> = (0..3).map(|_| tput(&pre, inp, c, 3.0)).collect();
                    r.sort_by(|a, b| a.partial_cmp(b).unwrap());
                    println!("RESULT rt={label} w={} metric=tput c={c} rps={:.0} rps_min={:.0} rps_max={:.0}", inp.w, r[1], r[0], r[2]);
                }
            }
            std::process::exit(0);
        }
        "cold" => {
            let reps = 9;
            let (mut comp, mut des, mut inst, mut first) = (vec![], vec![], vec![], vec![]);
            let ser = Module::new(&e, &bytes).unwrap().serialize().unwrap();
            for _ in 0..reps {
                let t = Instant::now();
                let m = Module::new(&e, &bytes).unwrap();
                comp.push(t.elapsed().as_secs_f64() * 1e3);
                let pre = linker(&e).instantiate_pre(&m).unwrap();
                let t = Instant::now();
                let mut i = instantiate(&pre);
                inst.push(t.elapsed().as_secs_f64() * 1e3);
                let t = Instant::now();
                call(&mut i, &inputs[0]);
                first.push(t.elapsed().as_secs_f64() * 1e3);
                let t = Instant::now();
                let _m2 = unsafe { Module::deserialize(&e, &ser).unwrap() };
                des.push(t.elapsed().as_secs_f64() * 1e3);
            }
            println!("RESULT rt={label} metric=cold size_bytes={} compile_cold_ms={:.1} compile_cached_ms(deserialize)={:.2} inst_ms={:.3} first_call_ms={:.3} serialized_kb={} (medians of {reps})",
                bytes.len(), median(&comp), median(&des), median(&inst), median(&first), ser.len() / 1024);
            std::process::exit(0);
        }
        "mem" => {
            let base = rss_mb();
            let m = Module::new(&e, &bytes).unwrap();
            let pre = linker(&e).instantiate_pre(&m).unwrap();
            let after_compile = rss_mb();
            let mut first = instantiate(&pre);
            for inp in &inputs[..3] {
                call(&mut first, inp);
            }
            let after_one = rss_mb();
            let mut v = vec![first];
            for _ in 1..40 {
                let mut i = instantiate(&pre);
                for inp in &inputs[..3] {
                    call(&mut i, inp);
                }
                v.push(i);
            }
            let after_n = rss_mb();
            println!("RESULT rt={label} metric=mem process_base_mb={base:.1} compiled_function_mb={:.1} first_instance_warm_mb={:.1} per_extra_warm_instance_mb={:.2}", after_compile - base, after_one - after_compile, (after_n - after_one) / 39.0);
            std::process::exit(0);
        }
        _ => eprintln!("usage: wtbench lat|cold|mem --wasm f.wasm"),
    }
}
