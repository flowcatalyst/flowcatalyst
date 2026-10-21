export default {
  async fetch(req, env) {
    const u = new URL(req.url);
    const mode = u.searchParams.get("mode");
    if (mode === "spin") { for (;;) {} }
    if (mode === "heap") {
      const keep = []; let n = 0;
      try { for (;;) { const chunk = []; for (let i = 0; i < 20000; i++) chunk.push({ a: i, s: "x".repeat(50) + i }); keep.push(chunk); n += 20000; if (n >= 60000000) break; } }
      catch (e) { return new Response("caught after " + n + " objects: " + e); }
      return new Response("allocated " + n + " objects without error");
    }
    if (mode === "alloc") {
      const keep = []; let mb = 0;
      try { for (;;) { keep.push(new Uint8Array(8 << 20).fill(1)); mb += 8; if (mb >= 6000) break; } }
      catch (e) { return new Response("caught after " + mb + " MB: " + e); }
      return new Response("allocated " + mb + " MB without error");
    }
    if (mode === "bridge") {
      const n = +(u.searchParams.get("n") || 1);
      let sum = 0;
      for (let i = 0; i < n; i++) {
        for (let k = 0; k < 20; k++) {
          const r = await env.RUNNER.fetch("http://runner/call?op=" + (k % 2 + 1), { method: "POST", body: '{"level":"info"}' });
          sum += (await r.text()).length;
        }
      }
      return new Response(String(sum));
    }
    if (mode === "ver") return new Response("v2");
    const w = u.searchParams.get("w") || "1";
    const n = +(u.searchParams.get("n") || 1);
    const body = new Uint8Array(await req.arrayBuffer());
    const meta = JSON.stringify({ w: +w });
    let r;
    for (let i = 0; i < n; i++) r = await globalThis.__fc.handle(meta, body);
    return new Response(r.body);
  },
};
