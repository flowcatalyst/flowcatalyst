const store = new Map();
export default {
  async fetch(req, env) {
    const u = new URL(req.url);
    if (u.pathname === "/load") {
      store.set(u.searchParams.get("v"), await req.text());
      return new Response("loaded " + u.searchParams.get("v") + "\n");
    }
    const v = u.searchParams.get("v");
    const code = store.get(v);
    if (code === undefined) return new Response("no such version", { status: 404 });
    const w = env.LOADER.get("fn-" + v, async () => ({
      compatibilityDate: "2026-10-01",
      mainModule: "main.js",
      modules: { "main.js": code },
      env: { RUNNER: env.RUNNER },
      globalOutbound: null,
    }));
    return await w.getEntrypoint().fetch(req);
  },
};
