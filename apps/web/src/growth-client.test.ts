import assert from "node:assert/strict";
import test from "node:test";
import {growthTarget,growthURL} from "./growth-client";
test("only known public topics can be prefilled",()=>{
  assert.equal(growthTarget("sleep"),"/?topic=sleep#start");
  assert.equal(growthTarget("../wrong"),"/#start");
});
test("signed sharing stays on first-party origin",()=>{
  assert.equal(growthURL("/g/g1.abc.def","https://apgic.ru"),"https://apgic.ru/g/g1.abc.def");
  assert.throws(()=>growthURL("https://evil.example","https://apgic.ru"));
  assert.throws(()=>growthURL("/g/g1.abc.def","http://evil.example"));
});
