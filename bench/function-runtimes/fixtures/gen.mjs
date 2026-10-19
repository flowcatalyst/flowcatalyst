// Generates the shared workload inputs deterministically:
//   input2k.json  W2 input (~2 KB order)
//   trip15k.xml   W3 input (~15 KB yard-trip message: the real BEVCO yard-trip.xml
//                 fixture's schema with the Lines repeated to reach ~15 KB; the real
//                 15 KB BluJay LeanXML sample is not in the repo)
import { writeFileSync } from "node:fs";
const here = new URL(".", import.meta.url).pathname;

const names = ["Lager 330ml can, 24 pack", "Cider 750ml bottle, 12 pack", "Gin & Tonic 250ml, 24 pack", "Still water 500ml, 24 pack", "Cola 2l PET, 6 pack"];
const lines = [];
for (let i = 0; i < 22; i++) {
  lines.push({ sku: `BEV-${100 + i}-CAN`, desc: names[i % names.length], qty: 3 + (i % 9), unitCents: 12000 + i * 375 });
}
const order = { orderId: "SO-100234", customer: { code: "C0042", name: "Acme Foods (Pty) Ltd" }, currency: "ZAR", lines, notes: "Deliver before 08:00; call the depot on arrival." };
writeFileSync(here + "input2k.json", JSON.stringify(order));

let xml = `<?xml version="1.0" encoding="UTF-8"?>
<YardTrip xmlns="urn:bevco:yard:trip:1" id="YT-20261016-0042">
  <TripNumber>YT-20261016-0042</TripNumber>
  <Status>IN_PROGRESS</Status>
  <Vehicle>
    <Registration>CA 123-456</Registration>
    <FleetNumber>TRK-017</FleetNumber>
  </Vehicle>
  <Driver>
    <Code>D0099</Code>
    <Name>Sipho Dlamini</Name>
  </Driver>
  <Origin>
    <Code>YARD-A</Code>
    <Name>Epping Yard A</Name>
  </Origin>
  <Destination>
    <Code>DEP-12</Code>
    <Name>Montague Gardens Depot</Name>
  </Destination>
  <PlannedStart>2026-10-16T05:30:00+02:00</PlannedStart>
  <PlannedEnd>2026-10-16T08:00:00+02:00</PlannedEnd>
  <ActualStart>2026-10-16T05:42:00+02:00</ActualStart>
  <Lines>
`;
let n = 0;
while (true) {
  n++;
  const d = names[n % names.length].replace("&", "&amp;");
  const line = `    <Line number="${n}">
      <Sku>BEV-${String(n).padStart(3, "0")}-${n % 2 ? "CAN" : "BTL"}</Sku>
      <Description>${d}</Description>
      <Quantity uom="PAL">${1 + (n % 14)}</Quantity>
    </Line>
`;
  if (xml.length + line.length + 20 > 15360) break;
  xml += line;
}
xml += "  </Lines>\n</YardTrip>\n";
writeFileSync(here + "trip15k.xml", xml);
console.log("trip lines", n - 1, "bytes", xml.length);
