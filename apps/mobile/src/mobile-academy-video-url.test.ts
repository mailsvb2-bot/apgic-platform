import assert from "node:assert/strict";
import test from "node:test";
import {nativeAcademyVideoPath} from "./mobile-academy-video-url.ts";

const e="22222222-2222-4222-8222-222222222222";
const l="33333333-3333-4333-8333-333333333333";
const m="44444444-4444-4444-8444-444444444444";

test("native video uses only the canonical first-party protected media route",()=>{
 assert.equal(nativeAcademyVideoPath(e,l,m),`/v1/academy/enrollments/${e}/lessons/${l}/videos/${m}/stream`);
});
test("reject any injection, foreign cloud origin, malformed path or token",()=>{
 for(const invalid of ["","../admin","disk:/foo","https://downloader.disk.yandex.net","?token=abc",e+"/../../other","00000000-0000-0000-0000-00000000000x"]){
  assert.equal(nativeAcademyVideoPath(invalid,l,m),null);
  assert.equal(nativeAcademyVideoPath(e,invalid,m),null);
  assert.equal(nativeAcademyVideoPath(e,l,invalid),null);
 }
});
