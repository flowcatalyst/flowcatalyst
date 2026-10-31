import { test } from "node:test";
import assert from "node:assert/strict";
import { Principal, callerFromMeta } from "./caller";

function principal(permissions: string[]): Principal {
  return new Principal({ permissions });
}

test("hasPermission: exact match", () => {
  assert.equal(principal(["platform:function:view"]).hasPermission("platform:function:view"), true);
});

test("hasPermission: different segment, no match", () => {
  assert.equal(principal(["platform:function:view"]).hasPermission("platform:function:manage"), false);
});

test("hasPermission: trailing wildcard", () => {
  assert.equal(principal(["platform:messaging:*:*"]).hasPermission("platform:messaging:queue:read"), true);
});

test("hasPermission: super-admin wildcard", () => {
  assert.equal(principal(["platform:*:*:*"]).hasPermission("platform:messaging:queue:read"), true);
});

test("hasPermission: requires the same segment count (fewer)", () => {
  assert.equal(principal(["platform:*"]).hasPermission("platform:messaging:queue:read"), false);
});

test("hasPermission: requires the same segment count (more)", () => {
  assert.equal(principal(["platform:*:*:*:*"]).hasPermission("platform:messaging:queue"), false);
});

test("hasPermission: middle wildcard", () => {
  assert.equal(principal(["platform:*:queue:read"]).hasPermission("platform:messaging:queue:read"), true);
  assert.equal(principal(["platform:*:queue:read"]).hasPermission("platform:messaging:topic:read"), false);
});

test("hasPermission: empty permissions", () => {
  assert.equal(principal([]).hasPermission("platform:function:view"), false);
});

test("hasPermission: one of several held permissions", () => {
  assert.equal(principal(["a:b", "platform:function:*"]).hasPermission("platform:function:view"), true);
});

test("hasRole is exact only, never wildcarded", () => {
  const p = new Principal({ roles: ["operant:admin", "billing:viewer"] });
  assert.equal(p.hasRole("operant:admin"), true);
  assert.equal(p.hasRole("operant:*"), false);
});

test("canAccessClient: an anchor principal always passes", () => {
  const p = new Principal({ tier: "ANCHOR" });
  assert.equal(p.canAccessClient("any-client"), true);
});

test("canAccessClientCode: anchor, exact match only", () => {
  assert.equal(new Principal({ tier: "ANCHOR" }).canAccessClientCode("bevco"), true);
  const p = new Principal({ tier: "CLIENT", clientCodes: ["bevco"] });
  assert.equal(p.canAccessClientCode("bevco"), true);
  assert.equal(p.canAccessClientCode("bev"), false);
  assert.equal(new Principal({ tier: "CLIENT" }).canAccessClientCode("bevco"), false);
});

test("canAccessClient: scoped to the explicit clients list", () => {
  const p = new Principal({ tier: "CLIENT", clients: ["clt_a"] });
  assert.equal(p.canAccessClient("clt_a"), true);
  assert.equal(p.canAccessClient("clt_z"), false);
});

test("canAccessApplication: allApplications short-circuits", () => {
  const all = new Principal({ allApplications: true });
  assert.equal(all.canAccessApplication("app_1"), true);

  const scoped = new Principal({ applications: ["app_1"] });
  assert.equal(scoped.canAccessApplication("app_1"), true);
  assert.equal(scoped.canAccessApplication("app_2"), false);
});

test("callerFromMeta decodes a principal caller with every field", () => {
  const caller = callerFromMeta({
    kind: "principal",
    id: "prn_1",
    type: "USER",
    tier: "CLIENT",
    clients: ["clt_1"],
    roles: ["operant:admin"],
    applications: ["app_1"],
    allApplications: false,
    permissions: ["platform:function:view"],
  });
  assert.equal(caller.kind, "principal");
  assert.ok(caller.principal);
  assert.equal(caller.principal?.id, "prn_1");
  assert.equal(caller.principal?.tier, "CLIENT");
  assert.equal(caller.principal?.hasRole("operant:admin"), true);
  assert.equal(caller.principal?.hasPermission("platform:function:view"), true);
});

test("callerFromMeta: webhook caller has no principal", () => {
  const caller = callerFromMeta({ kind: "webhook" });
  assert.equal(caller.kind, "webhook");
  assert.equal(caller.principal, undefined);
});

test("callerFromMeta: anonymous caller has no principal", () => {
  const caller = callerFromMeta({ kind: "anonymous" });
  assert.equal(caller.principal, undefined);
});

test("callerFromMeta: missing/null caller defaults to anonymous", () => {
  assert.equal(callerFromMeta(undefined).kind, "anonymous");
  assert.equal(callerFromMeta(null).kind, "anonymous");
});

const withApps = (applications: string[]) => new Principal({ applications });

test("canAccessApplication: matches the id half of an id:code pair", () => {
  const p = withApps(["app_1:orders", "app_2:billing"]);
  assert.equal(p.canAccessApplication("app_1"), true);
  assert.equal(p.canAccessApplication("app_2"), true);
  assert.equal(p.canAccessApplication("app_3"), false);
});

test("canAccessApplication: never matches the code half", () => {
  assert.equal(withApps(["app_1:orders"]).canAccessApplication("orders"), false);
});

test("canAccessApplication: splits on the first colon only", () => {
  const p = withApps(["app_1:co:de"]);
  assert.equal(p.canAccessApplication("app_1"), true);
  assert.equal(p.canAccessApplication("app_1:co"), false);
});

test("canAccessApplication: a * entry means every application", () => {
  assert.equal(withApps(["*"]).canAccessApplication("anything"), true);
});

test("canAccessApplication: no entry, no access", () => {
  assert.equal(withApps([]).canAccessApplication("app_1"), false);
  assert.equal(withApps([":code"]).canAccessApplication(""), false);
});

test("canAccessApplicationCode: code half, bare code, star, allApplications", () => {
  const p = withApps(["app_1:bevco-integration", "orders"]);
  assert.equal(p.canAccessApplicationCode("bevco-integration"), true);
  assert.equal(p.canAccessApplicationCode("orders"), true);
  assert.equal(p.canAccessApplicationCode("app_1"), false);
  assert.equal(p.canAccessApplicationCode("merensky-integration"), false);
  assert.equal(withApps(["*"]).canAccessApplicationCode("x"), true);
  assert.equal(new Principal({ allApplications: true }).canAccessApplicationCode("x"), true);
});
