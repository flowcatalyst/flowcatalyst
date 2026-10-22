import fs from "node:fs";
import { execSync, spawn } from "node:child_process";
const B = "http://127.0.0.1:18787";
const sleep = ms => new Promise(r => setTimeout(r, ms));
const post = (p, body) => fetch(B + p, { method: "POST", body });
const rss = pid => { try { return +fs.readFileSync(`/proc/${pid}/status`,"utf8").match(/VmRSS:\s+(\d+)/)[1]; } catch { return 0; } };
const startWd = () => spawn("node_modules/.bin/workerd", ["serve", "--experimental", "config2.capnp"], { stdio: "inherit" });
let wd = startWd(); 
for (let i = 0; i < 100; i++) { try { await fetch(B + "/?v=x"); break; } catch { await sleep(100); } }
const code = fs.readFileSync("worker2.mjs", "utf8");
await post("/load?v=2", code);
const inputs = { 2: "input2k.json", 3: "trip15k.xml", 4: "input2k.json" };
for (const w of [2, 3, 4]) {
  const r = await post(`/?v=2&w=${w}`, fs.readFileSync(inputs[w]));
  const got = Buffer.from(await r.arrayBuffer());
  console.log(`W${w}`, got.equals(fs.readFileSync(`expected-w${w}.txt`)) ? "identical" : "DIFFERS");
}
// speed: n calls in one request
for (const [w, n] of [[2, 20000], [3, 500], [4, 20]]) {
  const body = fs.readFileSync(inputs[w]);
  await post(`/?v=2&w=${w}&n=${Math.min(n, 50)}`, body);
  const t = performance.now();
  await post(`/?v=2&w=${w}&n=${n}`, body);
  console.log(`W${w} per call: ${((performance.now() - t) / n * 1000).toFixed(1)} us`);
}
console.log("RSS idle MB", (rss(wd.pid) / 1024).toFixed(0));
// concurrency on W3
for (const c of [1, 4, 8]) {
  const body = fs.readFileSync("trip15k.xml"); const N = 2000; let done = 0; const t = performance.now();
  await Promise.all(Array.from({ length: c }, async () => { while (done < N) { done++; await (await post("/?v=2&w=3", body)).arrayBuffer(); } }));
  console.log(`W3 c=${c}: ${(N / ((performance.now() - t) / 1000)).toFixed(0)} req/s`);
}
// runaway: spin, client deadline, kill, restart, reload
const ctl = new AbortController(); setTimeout(() => ctl.abort(), 500);
try { await fetch(B + "/?v=2&mode=spin", { signal: ctl.signal }); } catch {}
await sleep(1500);
console.log("after spin, other request in 1s?", await Promise.race([post("/?v=2&mode=ver").then(r => r.text()), sleep(1000).then(() => "NO (stuck)")]));
const tk = performance.now(); wd.kill("SIGKILL"); await sleep(50);
wd = startWd();
for (;;) { try { await post("/load?v=2", code); break; } catch { await sleep(5); } }
const r = await post("/?v=2&w=2", fs.readFileSync("input2k.json")); await r.arrayBuffer();
console.log(`kill -> first good answer: ${(performance.now() - tk).toFixed(0)} ms (includes 50 ms wait)`);
wd.kill("SIGKILL"); process.exit(0);
