
class El {
  name: string;
  attrs: Map<string, string>;
  kids: El[];
  text: string;
  constructor(name: string) {
    this.name = name;
    this.attrs = new Map<string, string>();
    this.kids = [];
    this.text = "";
  }
  child(name: string): El {
    for (let i = 0; i < this.kids.length; i++) {
      if (this.kids[i].name === name) return this.kids[i];
    }
    throw new Error("missing element " + name);
  }
  childText(name: string): string {
    return this.child(name).text;
  }
}

function unescapeXml(s: string): string {
  if (s.indexOf("&") < 0) return s;
  return s.replace(/&amp;/g, "&").replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&quot;/g, '"').replace(/&apos;/g, "'");
}

function localName(n: string): string {
  const i = n.indexOf(":");
  return i < 0 ? n : n.slice(i + 1);
}

function isSpace(c: number): boolean {
  return c === 32 || c === 9 || c === 10 || c === 13;
}

function parse(xml: string): El {
  const stack: El[] = [];
  let root: El | null = null;
  let i = 0;
  const n = xml.length;
  while (i < n) {
    if (xml.charCodeAt(i) !== 60) {
      const j = xml.indexOf("<", i);
      const end = j < 0 ? n : j;
      if (stack.length > 0) {
        const t = xml.slice(i, end).trim();
        if (t.length > 0) stack[stack.length - 1].text += unescapeXml(t);
      }
      i = end;
      continue;
    }
    if (xml.startsWith("<?", i)) {
      i = xml.indexOf("?>", i) + 2;
      continue;
    }
    if (xml.startsWith("<!--", i)) {
      i = xml.indexOf("-->", i) + 3;
      continue;
    }
    if (xml.charCodeAt(i + 1) === 47) {
      const j = xml.indexOf(">", i);
      stack.pop();
      i = j + 1;
      continue;
    }
    let j = i + 1;
    while (j < n && !isSpace(xml.charCodeAt(j)) && xml.charCodeAt(j) !== 62 && xml.charCodeAt(j) !== 47) j++;
    const el = new El(localName(xml.slice(i + 1, j)));
    let selfClose = false;
    for (;;) {
      while (j < n && isSpace(xml.charCodeAt(j))) j++;
      const c = xml.charCodeAt(j);
      if (c === 62) {
        j++;
        break;
      }
      if (c === 47) {
        selfClose = true;
        j += 2;
        break;
      }
      let k = j;
      while (k < n && xml.charCodeAt(k) !== 61 && !isSpace(xml.charCodeAt(k))) k++;
      const an = xml.slice(j, k);
      while (xml.charCodeAt(k) !== 34 && xml.charCodeAt(k) !== 39) k++;
      const q = xml.charCodeAt(k);
      const e = xml.indexOf(String.fromCharCode(q), k + 1);
      const av = unescapeXml(xml.slice(k + 1, e));
      if (an !== "xmlns" && !an.startsWith("xmlns:")) el.attrs.set(localName(an), av);
      j = e + 1;
    }
    if (stack.length > 0) stack[stack.length - 1].kids.push(el);
    else root = el;
    if (!selfClose) stack.push(el);
    i = j;
  }
  if (root === null) throw new Error("no root");
  return root;
}

interface Loc {
  code: string;
  name: string;
}
interface Vehicle {
  registration: string;
  fleetNumber: string;
}
interface Line {
  lineNumber: string;
  sku: string;
  description: string;
  quantity: number;
  uom: string;
}
interface Trip {
  tripId: string;
  status: string;
  vehicle: Vehicle;
  driver: Loc;
  origin: Loc;
  destination: Loc;
  plannedStart: string;
  plannedEnd: string;
  actualStart: string;
  lines: Line[];
}

function loc(e: El): Loc {
  return { code: e.childText("Code"), name: e.childText("Name") };
}

function w3(xml: string): string {
  const t = parse(xml);
  const lines: Line[] = [];
  const ls = t.child("Lines");
  for (let i = 0; i < ls.kids.length; i++) {
    const l = ls.kids[i];
    if (l.name !== "Line") continue;
    const q = l.child("Quantity");
    lines.push({
      lineNumber: l.attrs.get("number") as string,
      sku: l.childText("Sku"),
      description: l.childText("Description"),
      quantity: parseInt(q.text, 10),
      uom: q.attrs.get("uom") as string,
    });
  }
  const v = t.child("Vehicle");
  const trip: Trip = {
    tripId: t.childText("TripNumber"),
    status: t.childText("Status"),
    vehicle: { registration: v.childText("Registration"), fleetNumber: v.childText("FleetNumber") },
    driver: loc(t.child("Driver")),
    origin: loc(t.child("Origin")),
    destination: loc(t.child("Destination")),
    plannedStart: t.childText("PlannedStart"),
    plannedEnd: t.childText("PlannedEnd"),
    actualStart: t.childText("ActualStart"),
    lines: lines,
  };
  return JSON.stringify(trip);
}


export function w3x(xml: string): string {
  return w3(xml);
}
