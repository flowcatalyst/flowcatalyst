
use std::time::{Duration, Instant};
use wasmtime::*;
use wasmtime_wasi::p1::{self, WasiP1Ctx};
use wasmtime_wasi::WasiCtxBuilder;

const ENGINE_WASM: &str = "/Users/andrewgraaff/Developer/flowcatalyst-go/internal/functions/jsengine/engine.wasm";
const BUNDLE: &str = "/Users/andrewgraaff/Developer/flowcatalyst-go/bench/function-runtimes/js/dist/function.js";
const FIX: &str = "/Users/andrewgraaff/Developer/flowcatalyst-go/bench/function-runtimes/fixtures/";

fn engine(epoch: bool) -> Result<Engine> {
    let mut c = Config::new();
    c.epoch_interruption(epoch);
    Engine::new(&c)
}

fn start_ticker(e: &Engine) {
    let e = e.clone();
    std::thread::spawn(move || loop {
        std::thread::sleep(Duration::from_millis(1));
        e.increment_epoch();
    });
}

fn frame(meta: &[u8], body: &[u8]) -> Vec<u8> {
    let mut f = (meta.len() as u32).to_le_bytes().to_vec();
    f.extend_from_slice(meta);
    f.extend_from_slice(body);
    f
}

struct Guest {
    store: Store<WasiP1Ctx>,
    mem: Memory,
    alloc: TypedFunc<u32, u32>,
    handle: TypedFunc<(u32, u32), u64>,
    deadline: TypedFunc<u64, ()>,
}

fn instantiate(engine: &Engine, module: &Module, epoch: bool) -> Result<Guest> {
    let mut linker: Linker<WasiP1Ctx> = Linker::new(engine);
    p1::add_to_linker_sync(&mut linker, |c| c)?;
    linker.func_wrap("fc", "call", |_op: u32, _ptr: u32, _len: u32| -> u64 { 0 })?;
    linker.func_wrap("fc", "take", |_dst: u32| {})?;
    let wasi = WasiCtxBuilder::new().build_p1();
    let mut store = Store::new(engine, wasi);
    if epoch {
        store.set_epoch_deadline(u64::MAX / 2);
    }
    let inst = linker.instantiate(&mut store, module)?;
    let mem = inst.get_memory(&mut store, "memory").unwrap();
    let alloc = inst.get_typed_func::<u32, u32>(&mut store, "fc_alloc")?;
    let handle = inst.get_typed_func::<(u32, u32), u64>(&mut store, "fc_handle")?;
    let deadline = inst.get_typed_func::<u64, ()>(&mut store, "fc_js_deadline")?;
    let load = inst.get_typed_func::<(u32, u32), u32>(&mut store, "fc_js_load")?;
    let src = std::fs::read(BUNDLE)?;
    let p = alloc.call(&mut store, src.len() as u32)?;
    mem.write(&mut store, p as usize, &src)?;
    assert_eq!(load.call(&mut store, (p, src.len() as u32))?, 0, "script load failed");
    Ok(Guest { store, mem, alloc, handle, deadline })
}

impl Guest {
    fn call(&mut self, meta: &str, body: &[u8], ms: u64) -> Result<Vec<u8>> {
        self.deadline.call(&mut self.store, ms)?;
        let f = frame(meta.as_bytes(), body);
        let p = self.alloc.call(&mut self.store, f.len() as u32)?;
        self.mem.write(&mut self.store, p as usize, &f)?;
        let r = self.handle.call(&mut self.store, (p, f.len() as u32))?;
        let (ptr, len) = ((r >> 32) as usize, (r & 0xffff_ffff) as usize);
        let mut out = vec![0u8; len];
        self.mem.read(&self.store, ptr, &mut out)?;
        Ok(out)
    }
}

fn p50(mut v: Vec<f64>) -> f64 {
    v.sort_by(|a, b| a.partial_cmp(b).unwrap());
    v[v.len() / 2]
}

fn bench(label: &str, g: &mut Guest, meta: &str, body: &[u8], n: usize, ms: u64) -> Result<()> {
    for _ in 0..(n / 5).max(1) {
        g.call(meta, body, ms)?;
    }
    let mut t = vec![];
    for _ in 0..n {
        let s = Instant::now();
        g.call(meta, body, ms)?;
        t.push(s.elapsed().as_secs_f64() * 1e6);
    }
    println!("  {label}: p50 {:.1} us", p50(t));
    Ok(())
}

fn main() -> Result<()> {
    let which = std::env::args().nth(1).unwrap_or_default();
    let json = std::fs::read(format!("{FIX}input2k.json"))?;
    let xml = std::fs::read(format!("{FIX}trip15k.xml"))?;
    if which == "verify" {
        let e = engine(false)?;
        let m = Module::from_file(&e, ENGINE_WASM)?;
        let mut g = instantiate(&e, &m, false)?;
        for (w, body) in [(2, &json[..]), (3, &xml[..]), (4, &b""[..])] {
            let out = g.call(&format!("{{\"w\":{w}}}"), body, 30_000)?;
            let n = u32::from_le_bytes([out[0], out[1], out[2], out[3]]) as usize;
            let got = &out[4 + n..];
            let want = std::fs::read(format!("{FIX}expected-w{w}.txt"))?;
            println!("  W{w}: {}", if got == &want[..] { "byte-identical to reference" } else { "DIFFERS" });
        }
    } else if which == "engine" {
        for epoch in [false, true] {
            let e = engine(epoch)?;
            if epoch { start_ticker(&e); }
            let m = Module::from_file(&e, ENGINE_WASM)?;
            let mut g = instantiate(&e, &m, epoch)?;
            println!("QuickJS engine on wasmtime, epoch interruption {}", if epoch { "ON (1 ms ticker)" } else { "OFF" });
            bench("W2 json 2KB ", &mut g, "{\"w\":2}", &json, 400, 30_000)?;
            bench("W3 xml 15KB ", &mut g, "{\"w\":3}", &xml, 40, 30_000)?;
            bench("W4 loop     ", &mut g, "{\"w\":4}", b"", 3, 30_000)?;
        }
    } else if which == "runaway" {
        // A Rust-style guest in an unconditional loop, stopped by the epoch deadline.
        let wat = r#"(module (func (export "spin") (local i32) (loop $l (local.set 0 (i32.add (local.get 0) (i32.const 1))) (br $l))) (func (export "ok") (result i32) (i32.const 7)))"#;
        let e = engine(true)?;
        start_ticker(&e);
        let m = Module::new(&e, wat)?;
        for round in 1..=3 {
            let mut store = Store::new(&e, ());
            store.set_epoch_deadline(250); // 250 ticks = 250 ms
            let inst = Instance::new(&mut store, &m, &[])?;
            let spin = inst.get_typed_func::<(), ()>(&mut store, "spin")?;
            let s = Instant::now();
            let r = spin.call(&mut store, ());
            let took = s.elapsed();
            let trap = r.err().and_then(|e| e.downcast::<Trap>().ok());
            println!("  round {round}: spin stopped after {:?}, trap = {:?}", took, trap);
            let mut s2 = Store::new(&e, ());
            s2.set_epoch_deadline(250);
            let i2 = Instance::new(&mut s2, &m, &[])?;
            let ok = i2.get_typed_func::<(), i32>(&mut s2, "ok")?;
            println!("           next call on a fresh instance returns {}", ok.call(&mut s2, ())?);
        }
        let s = Instant::now();
        while s.elapsed() < Duration::from_secs(2) { std::thread::sleep(Duration::from_millis(100)); }
        println!("  (process idle 2 s after: check its CPU externally)");
    }
    Ok(())
}
