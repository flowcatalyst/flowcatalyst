import { readFileSync } from "node:fs";
import { XMLParser } from "fast-xml-parser";

const parser = new XMLParser({
  ignoreAttributes: false,
  attributeNamePrefix: "@_",
  removeNSPrefix: true,
  parseTagValue: false,
  parseAttributeValue: false,
  trimValues: true,
  isArray: (name: string) => name === "Line",
});

function w3(xml: string): string {
  const t = parser.parse(xml).YardTrip;
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

const xml = readFileSync(process.argv[2], "utf8");
console.log(w3(xml));
