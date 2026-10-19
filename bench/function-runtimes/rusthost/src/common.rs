//! Shared by both hosts: fixtures, the request frames, statistics and the
//! RESULT line format (the same keys wazerobench prints).
use std::time::{Duration, Instant};

pub const WORKLOADS: [u8; 5] = [1, 2, 3, 4, 5];

pub struct Input {
    pub w: u8,
    pub meta: Vec<u8>,
    pub body: Vec<u8>,
    pub expect: Vec<u8>,
}

pub fn fixtures_dir() -> String {
    std::env::var("FIXTURES").unwrap_or_else(|_| "bench/function-runtimes/fixtures".into())
}

pub fn inputs() -> Vec<Input> {
    let d = fixtures_dir();
    let read = |n: &str| std::fs::read(format!("{d}/{n}")).unwrap_or_else(|e| panic!("{d}/{n}: {e}"));
    WORKLOADS
        .iter()
        .map(|&w| Input {
            w,
            meta: format!("{{\"w\":{w}}}").into_bytes(),
            body: match w {
                1 | 2 => read("input2k.json"),
                3 => read("trip15k.xml"),
                _ => Vec::new(),
            },
            expect: read(&format!("expected-w{w}.txt")),
        })
        .collect()
}

/// The host side of the W5 capability calls: op 2 returns a config value.
pub fn host_op(op: u32) -> (&'static [u8], &'static [u8]) {
    if op == 2 {
        (br#"{"found":true}"#, b"hello-config-value")
    } else {
        (b"", b"")
    }
}

pub fn pct(v: &mut [Duration], p: f64) -> f64 {
    v.sort_unstable();
    v[((v.len() as f64 - 1.0) * p) as usize].as_secs_f64() * 1e6
}

pub fn median(v: &[f64]) -> f64 {
    let mut s = v.to_vec();
    s.sort_by(|a, b| a.partial_cmp(b).unwrap());
    s[s.len() / 2]
}

pub fn minmax(v: &[f64]) -> (f64, f64) {
    (v.iter().cloned().fold(f64::MAX, f64::min), v.iter().cloned().fold(f64::MIN, f64::max))
}

/// Warm up, then `runs` timed runs of `dur`; prints the RESULT lat line.
pub fn measure_latency(label: &str, w: u8, runs: usize, warm: Duration, dur: Duration, mut call: impl FnMut()) {
    let t0 = Instant::now();
    while t0.elapsed() < warm {
        call();
    }
    let (mut p50s, mut p99s, mut means) = (vec![], vec![], vec![]);
    let mut total = 0;
    for _ in 0..runs {
        let mut ds = Vec::new();
        let t0 = Instant::now();
        while t0.elapsed() < dur {
            let s = Instant::now();
            call();
            ds.push(s.elapsed());
        }
        let sum: Duration = ds.iter().sum();
        means.push(sum.as_secs_f64() * 1e6 / ds.len() as f64);
        total += ds.len();
        p99s.push(pct(&mut ds, 0.99));
        p50s.push(pct(&mut ds, 0.5));
    }
    let (a, b) = minmax(&p50s);
    let (c, d) = minmax(&p99s);
    println!(
        "RESULT rt={label} w={w} metric=lat p50_us={:.2} p50_min={a:.2} p50_max={b:.2} p99_us={:.2} p99_min={c:.2} p99_max={d:.2} mean_us={:.2} calls={total} runs={runs}",
        median(&p50s), median(&p99s), median(&means)
    );
}

pub fn rss_mb() -> f64 {
    unsafe {
        let mut info: libc::rusage_info_v2 = std::mem::zeroed();
        libc::proc_pid_rusage(libc::getpid(), libc::RUSAGE_INFO_V2, &mut info as *mut _ as *mut libc::rusage_info_t);
        info.ri_resident_size as f64 / 1048576.0
    }
}

pub fn arg(name: &str) -> Option<String> {
    let a: Vec<String> = std::env::args().collect();
    a.iter().position(|x| x == name).map(|i| a[i + 1].clone())
}
