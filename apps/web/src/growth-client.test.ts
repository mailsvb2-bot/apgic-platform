import assert from "node:assert/strict";
import test from "node:test";

import { growthTarget, growthURL } from "./growth-client.ts";

test("only canonical public demand topics can prefill the form", () => {
  for (const topic of ["anxiety", "sleep", "relationships", "career"]) {
    assert.equal(growthTarget(topic), "/?topic=" + topic + "#start");
  }
  for (const invalid of ["../wrong", "SLEEP", "__proto__", "", "sleep?redirect=evil"]) {
    assert.equal(growthTarget(invalid), "/#start");
  }
});

test("sharing only supports signed first-party opaque paths", () => {
  assert.equal(growthURL("/g/g1.abc.def", "https://apgic.ru"), "https://apgic.ru/g/g1.abc.def");
  assert.equal(growthURL("/g/g1.abc.def", "http://localhost:3000"), "http://localhost:3000/g/g1.abc.def");
  assert.equal(growthURL("/g/g1.abc.def", "http://127.0.0.1:18000"), "http://127.0.0.1:18000/g/g1.abc.def");
  for (const path of [
    "https://evil.example/", "//evil.example", "/g/evil", "/g/g1.a.b/other",
    "/g/g1.a.b?redirect=https://evil.example", "/g/g1.a.b#fragment",
  ]) {
    assert.throws(() => growthURL(path, "https://apgic.ru"));
  }
  assert.throws(() => growthURL("/g/g1.abc.def", "http://evil.example"));
});
