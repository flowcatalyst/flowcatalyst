// Runs the bundle under node with fake natives; writes ../fixtures/expected-wN.txt (the reference outputs every other runtime must match byte for byte).
import { readFileSync, writeFileSync } from "node:fs";
import vm from "node:vm";
const enc = new TextEncoder(), dec = new TextDecoder();
const g = { __fc_utf8_encode: (s) => enc.encode(s), __fc_utf8_decode: (b) => dec.decode(b),
  __fc_call: (op) => ({ code: 0, meta: "{}", body: op === 2 ? enc.encode("hello-config-value") : new Uint8Array(0) }) };
g.globalThis = g;
vm.runInNewContext(readFileSync("dist/function.js", "utf8"), g);
const inputs = { 1: readFileSync("../fixtures/input2k.json"), 2: readFileSync("../fixtures/input2k.json"), 3: readFileSync("../fixtures/trip15k.xml"), 4: new Uint8Array(0), 5: new Uint8Array(0) };
for (const w of [1, 2, 3, 4, 5]) {
  const r = await g.__fc.handle(`{"w":${w}}`, new Uint8Array(inputs[w]));
  const out = typeof r.body === "string" ? r.body : dec.decode(r.body);
  writeFileSync(`../fixtures/expected-w${w}.txt`, out);
  console.log(w, out.length, out.slice(0, 100));
}
