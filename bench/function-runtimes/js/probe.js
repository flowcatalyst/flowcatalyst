// Prints which globals / language features the engine offers (describe() returns the report).
// Run through `wazerobench -mode smoke` (QuickJS) and `v8bench smoke` (V8).
globalThis.__fc = {
  handle() { return { meta: "{}", body: "" }; },
  describe() {
    const t = (n) => { try { return typeof globalThis[n]; } catch (e) { return "throws"; } };
    const f = (fn) => { try { return String(fn()); } catch (e) { return "throws:" + e.name; } };
    const names = ["console", "setTimeout", "setInterval", "queueMicrotask", "structuredClone", "TextEncoder", "TextDecoder", "URL", "URLSearchParams",
      "atob", "btoa", "crypto", "fetch", "Request", "Response", "Headers", "AbortController", "Buffer", "process", "performance", "Intl", "WebAssembly",
      "SharedArrayBuffer", "Atomics", "WeakRef", "FinalizationRegistry", "BigInt", "Proxy", "Reflect", "Temporal", "Deno", "require", "module", "__fc_call", "__fc_utf8_encode", "__fc_utf8_decode"];
    const o = { globals: {} };
    for (const n of names) o.globals[n] = t(n);
    o.features = {
      toLocaleString: f(() => (1234567.891).toLocaleString()),
      dateToString: f(() => new Date(0).toString()),
      tzOffset: f(() => new Date().getTimezoneOffset()),
      dateToLocale: f(() => new Date(0).toLocaleDateString("en-ZA")),
      intlNumberFormat: f(() => new Intl.NumberFormat("en-ZA", { style: "currency", currency: "ZAR" }).format(1234.5)),
      replaceAll: f(() => "a-b".replaceAll("-", "+")),
      arrayAt: f(() => [1, 2, 3].at(-1)),
      toSorted: f(() => [3, 1, 2].toSorted().join()),
      objectGroupBy: f(() => typeof Object.groupBy),
      promiseWithResolvers: f(() => typeof Promise.withResolvers),
      regexUnicodeProp: f(() => /\p{L}+/u.test("é")),
      regexLookbehind: f(() => /(?<=a)b/.test("ab")),
      regexNamedGroups: f(() => "2026-10-19".match(/(?<y>\d+)-/).groups.y),
      stackFormat: f(() => { try { null.x; } catch (e) { return e.stack.split("\n")[0] + " | " + e.message; } }),
      errorMessageUndefinedProp: f(() => { try { undefined.foo; } catch (e) { return e.message; } }),
      numToFixed: f(() => (1.005).toFixed(2) + "," + (0.000001234).toString() + "," + (1e21).toString()),
      jsonDate: f(() => JSON.stringify(new Date(0))),
      sortStable: f(() => [{ k: 1, i: 0 }, { k: 1, i: 1 }, { k: 0, i: 2 }].sort((a, b) => a.k - b.k).map((x) => x.i).join()),
      stringNormalize: f(() => "é".normalize("NFD").length),
      localeCompare: f(() => ["b", "a", "ä"].sort((a, b) => a.localeCompare(b)).join()),
      toUpperCaseSpecial: f(() => "ß".toUpperCase()),
      mathRandomType: f(() => typeof Math.random()),
    };
    return JSON.stringify(o, null, 1);
  },
};
