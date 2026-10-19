// The benchmark function, as a JS function: ABI-v1 `__fc` contract of the
// shared QuickJS engine (clients/fn-js-engine/src/lib.rs), raw (no SDK) so the
// numbers are the engine's, not the SDK router's. The same bundle runs on
// QuickJS-in-wazero and on V8 (deno_core); the host supplies the three natives.
//
// Request meta is exactly `{"w":N}`; N selects the workload.
import { XMLParser } from "fast-xml-parser";

declare const __fc_call: (op: number, meta: string, body?: Uint8Array) => { code: number; meta: string; body: Uint8Array };
declare const __fc_utf8_encode: (s: string) => Uint8Array;
declare const __fc_utf8_decode: (b: Uint8Array) => string;

const parser = new XMLParser({
  ignoreAttributes: false,
  attributeNamePrefix: "@_",
  removeNSPrefix: true,
  parseTagValue: false,
  parseAttributeValue: false,
  trimValues: true,
  isArray: (name) => name === "Line",
});

const LOG_META = '{"level":"info","msg":"bench"}';
const CFG_META = '{"key":"BENCH"}';

function w2(body: Uint8Array): string {
  const o = JSON.parse(__fc_utf8_decode(body));
  let total = 0;
  const lines = o.lines.map((l: any) => {
    const amountCents = l.qty * l.unitCents;
    total += amountCents;
    return { sku: l.sku, amountCents };
  });
  return JSON.stringify({ id: o.orderId, customer: o.customer.code, currency: o.currency, lineCount: lines.length, totalCents: total, lines });
}

function w3(body: Uint8Array): string {
  const t = parser.parse(__fc_utf8_decode(body)).YardTrip;
  const loc = (x: any) => ({ code: x.Code, name: x.Name });
  return JSON.stringify({
    tripId: t.TripNumber,
    status: t.Status,
    vehicle: { registration: t.Vehicle.Registration, fleetNumber: t.Vehicle.FleetNumber },
    driver: loc(t.Driver),
    origin: loc(t.Origin),
    destination: loc(t.Destination),
    plannedStart: t.PlannedStart,
    plannedEnd: t.PlannedEnd,
    actualStart: t.ActualStart,
    lines: t.Lines.Line.map((l: any) => ({
      lineNumber: l["@_number"],
      sku: l.Sku,
      description: l.Description,
      quantity: parseInt(l.Quantity["#text"], 10),
      uom: l.Quantity["@_uom"],
    })),
  });
}

function w4(): string {
  let x = 2463534242 | 0;
  let acc = 0;
  for (let i = 0; i < 5_000_000; i++) {
    x ^= x << 13;
    x ^= x >>> 17;
    x ^= x << 5;
    acc = (acc + (x >>> 0)) >>> 0;
  }
  return String(acc);
}

function w5(): string {
  let sum = 0;
  for (let i = 0; i < 10; i++) {
    const a = __fc_call(1, LOG_META, undefined);
    sum += a.code + a.body.length;
    const b = __fc_call(2, CFG_META, undefined);
    sum += b.code + b.body.length;
  }
  return String(sum);
}

async function handle(meta: string, body: Uint8Array): Promise<{ meta: string; body: Uint8Array | string }> {
  const w = meta.charCodeAt(5) - 48;
  const m = '{"status":200}';
  switch (w) {
    case 1: return { meta: m, body };
    case 2: return { meta: m, body: w2(body) };
    case 3: return { meta: m, body: w3(body) };
    case 4: return { meta: m, body: w4() };
    case 5: return { meta: m, body: w5() };
  }
  throw new Error("unknown workload");
}

(globalThis as any).__fc = { describe: () => '{"abi":1}', handle };
