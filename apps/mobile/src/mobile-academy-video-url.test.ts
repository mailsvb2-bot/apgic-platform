import assert from "node:assert/strict";
import test from "node:test";
import {nativeAcademyVideoPath,nativeAcademyValidatedVideos} from "./mobile-academy-video-url.ts";

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

test("native manifest rejects UUID case aliases of the same media",()=>{
 const upper="AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA";
 const lower=upper.toLowerCase();
 assert.equal(nativeAcademyValidatedVideos({videos:[
  {id:upper,title:"First",ordinal:1},
  {id:lower,title:"Second",ordinal:2},
 ]},e,l),null);
});

test("native manifest rejects duplicate ids and malformed or unbounded video metadata",()=>{
 const good={videos:[{id:m,title:"Lesson",ordinal:1,provider_url:"https://untrusted.invalid/private"}]};
 assert.deepEqual(nativeAcademyValidatedVideos(good,e,l),[{id:m,title:"Lesson",ordinal:1}]);
 for(const bad of [null,{}, {videos:[]}, {videos:[null]},
   {videos:[good.videos[0],good.videos[0]]},
   {videos:[{id:"../token",title:"Oops",ordinal:1}]},
   {videos:[{id:m,title:"Oops",ordinal:-1}]},
   {videos:[{id:m,title:"x".repeat(301),ordinal:1}]},
   {videos:Array(101).fill(good.videos[0])}
 ])assert.equal(nativeAcademyValidatedVideos(bad,e,l),null);
});
