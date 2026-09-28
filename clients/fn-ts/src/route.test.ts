import { test } from "node:test";
import assert from "node:assert/strict";
import { Router, compileSegments, matchSegments, splitPath, splitPattern, type RouteEntry } from "./route";

test("splitPattern parses method and path", () => {
  assert.deepEqual(splitPattern("POST /events/order-created"), { method: "POST", path: "/events/order-created" });
});

test("splitPattern requires an explicit method", () => {
  assert.throws(() => splitPattern("/healthz"), /METHOD \/path/);
});

test("compileSegments compiles literal, param and rest", () => {
  assert.deepEqual(compileSegments("/widgets/{id}/files/{rest...}"), [
    { kind: "literal", value: "widgets" },
    { kind: "param", value: "id" },
    { kind: "literal", value: "files" },
    { kind: "rest", value: "rest" },
  ]);
});

test("compileSegments rejects a rest segment that isn't last", () => {
  assert.throws(() => compileSegments("/{rest...}/more"));
});

test("matchSegments extracts a param", () => {
  const segs = compileSegments("/hello/{name}");
  assert.deepEqual(matchSegments(segs, splitPath("/hello/world")), { name: "world" });
  assert.equal(matchSegments(segs, splitPath("/hello/world/extra")), null);
});

test("matchSegments rest consumes the remainder including zero segments", () => {
  const segs = compileSegments("/files/{rest...}");
  assert.deepEqual(matchSegments(segs, splitPath("/files/a/b/c")), { rest: "a/b/c" });
  assert.deepEqual(matchSegments(segs, splitPath("/files")), { rest: "" });
});

function entry(index: number, method: string, path: string): RouteEntry {
  return { index, method, path, segs: compileSegments(path) };
}

test("404 when nothing matches the path", () => {
  const r = new Router([entry(0, "GET", "/healthz")]);
  const result = r.match("GET", "/nope");
  assert.equal(result.kind, "no-match");
});

test("405 with an Allow header when the path matches but not the method", () => {
  const r = new Router([entry(0, "POST", "/only-post"), entry(1, "GET", "/only-post")]);
  const result = r.match("DELETE", "/only-post");
  assert.equal(result.kind, "method-not-allowed");
  if (result.kind === "method-not-allowed") {
    assert.deepEqual(result.allow, ["GET", "HEAD", "POST"]);
  }
});

test("GET registrations also answer HEAD, with the body suppressed by the dispatcher", () => {
  const r = new Router([entry(0, "GET", "/healthz")]);
  const result = r.match("HEAD", "/healthz");
  assert.equal(result.kind, "found");
  if (result.kind === "found") {
    assert.equal(result.index, 0);
    assert.equal(result.headFallback, true);
  }
});

test("an explicit HEAD registration is used over the GET fallback", () => {
  const r = new Router([entry(0, "GET", "/healthz"), entry(1, "HEAD", "/healthz")]);
  const result = r.match("HEAD", "/healthz");
  assert.equal(result.kind, "found");
  if (result.kind === "found") {
    assert.equal(result.index, 1);
    assert.equal(result.headFallback, false);
  }
});

test("precedence: a literal segment beats a param at the same position", () => {
  const r = new Router([entry(0, "GET", "/widgets/{id}"), entry(1, "GET", "/widgets/known")]);
  const result = r.match("GET", "/widgets/known");
  assert.equal(result.kind, "found");
  if (result.kind === "found") assert.equal(result.index, 1);
});

test("precedence: a param beats a trailing rest wildcard", () => {
  const r = new Router([entry(0, "GET", "/files/{rest...}"), entry(1, "GET", "/files/{name}")]);
  const result = r.match("GET", "/files/report");
  assert.equal(result.kind, "found");
  if (result.kind === "found") assert.equal(result.index, 1);
});

test("precedence: a longer literal match beats a shorter rest match", () => {
  const r = new Router([entry(0, "GET", "/files/{rest...}"), entry(1, "GET", "/files/a/b")]);
  const result = r.match("GET", "/files/a/b");
  assert.equal(result.kind, "found");
  if (result.kind === "found") assert.equal(result.index, 1);
});

test("root path matches an empty segment list", () => {
  const r = new Router([entry(0, "GET", "/")]);
  const result = r.match("GET", "/");
  assert.equal(result.kind, "found");
});

test("multiple params in one pattern", () => {
  const segs = compileSegments("/widgets/{id}/parts/{part}");
  const params = matchSegments(segs, splitPath("/widgets/42/parts/7"));
  assert.deepEqual(params, { id: "42", part: "7" });
});
