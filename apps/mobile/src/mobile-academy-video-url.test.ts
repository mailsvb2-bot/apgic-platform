import assert from "node:assert/strict";
import test from "node:test";
import {nativeAcademyManifestPath,nativeAcademyVideoPath,nativeAcademyValidatedVideos} from "./mobile-academy-video-url.ts";

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
 const good={videos:[{id:m,title:"Lesson",ordinal:1}]};
 assert.deepEqual(nativeAcademyValidatedVideos(good,e,l),good.videos);
 for(const key of ["provider_url","cloud_url","download_url","downloadUrl","stream_uri","href","access_token","refresh_token","authorization","cookie","credentials","secret"]){
  assert.equal(nativeAcademyValidatedVideos({videos:[{...good.videos[0],[key]:"secret"}]},e,l),null);
 }
 for(const bad of [null,{}, {videos:[]}, {videos:[null]},
   {videos:[good.videos[0],good.videos[0]]},
   {videos:[{id:"../token",title:"Oops",ordinal:1}]},
   {videos:[{id:m,title:"Oops",ordinal:-1}]},
   {videos:[{id:m,title:"x".repeat(301),ordinal:1}]},
   {videos:Array(101).fill(good.videos[0])}
 ])assert.equal(nativeAcademyValidatedVideos(bad,e,l),null);
});

test("native manifest rejects ambiguous ordering and unsafe display titles",()=>{
 const other="55555555-5555-4555-8555-555555555555";
 assert.equal(nativeAcademyValidatedVideos({videos:[
  {id:m,title:"First",ordinal:1},{id:other,title:"Second",ordinal:1}
 ]},e,l),null);
 for(const title of ["Video\nInjected", "Video\u0000Hidden", "Video\u007fHidden"]){
  assert.equal(nativeAcademyValidatedVideos({videos:[{id:m,title,ordinal:1}]},e,l),null);
 }
 assert.deepEqual(nativeAcademyValidatedVideos({videos:[
  {id:m,title:"Урок 1",ordinal:1},{id:other,title:"Урок 2",ordinal:2}
 ]},e,l),[
  {id:m,title:"Урок 1",ordinal:1},{id:other,title:"Урок 2",ordinal:2}
 ]);
});


test("native manifest rejects invisible and bidi-spoofed lesson titles",()=>{
 for(const title of ["", "   ", "\u202eVideo", "Video\u202c", "\u2066Lesson\u2069"]){
  assert.equal(nativeAcademyValidatedVideos({videos:[{id:m,title,ordinal:1}]},e,l),null);
 }
 assert.deepEqual(nativeAcademyValidatedVideos({videos:[{id:m,title:"Урок — введение",ordinal:1}]},e,l),[
  {id:m,title:"Урок — введение",ordinal:1}
 ]);
});


test("native manifest requests reject malformed navigation identities before signed fetch",()=>{
 assert.equal(nativeAcademyManifestPath(e,l),`/v1/academy/enrollments/${e}/lessons/${l}/videos`);
 for(const invalid of ["", "../admin", "https://disk.yandex.ru/private", "?token=secret", e+"/../other", "00000000-0000-0000-0000-00000000000x"]){
  assert.equal(nativeAcademyManifestPath(invalid,l),null);
  assert.equal(nativeAcademyManifestPath(e,invalid),null);
 }
});


test("native manifest rejects provider credentials and cloud URLs at envelope level",()=>{
 const safe={videos:[{id:m,title:"Урок",ordinal:1}]};
 for(const key of ["provider_url","downloadUrl","stream_uri","authorization","access_token","refreshToken","cookie","credentials","secret"]){
  assert.equal(nativeAcademyValidatedVideos({...safe,[key]:"https://disk.yandex.ru/private?token=secret"},e,l),null);
 }
 assert.equal(nativeAcademyValidatedVideos([safe],e,l),null);
 assert.deepEqual(nativeAcademyValidatedVideos({...safe,course_id:e},e,l),safe.videos);
});


test("native manifest refuses nested provider secrets without rejecting safe metadata",()=>{
 const video={id:m,title:"Урок",ordinal:1};
 for(const nested of [{provider:{accessToken:"secret"}},{metadata:{source:{downloadUrl:"https://disk.yandex.ru/private"}}},{audit:[{cookie:"secret"}]}]){
  assert.equal(nativeAcademyValidatedVideos({videos:[video],...nested},e,l),null);
  assert.equal(nativeAcademyValidatedVideos({videos:[{...video,details:nested}]},e,l),null);
 }
 assert.deepEqual(nativeAcademyValidatedVideos({videos:[video],metadata:{revision:2,flags:["ready"]}},e,l),[video]);
 let deep:unknown={accessToken:"secret"};
 for(let i=0;i<12;i++)deep={child:deep};
 assert.equal(nativeAcademyValidatedVideos({videos:[video],metadata:deep},e,l),null);
});

test("author preview routes bind owned version and never use enrollment paths",()=>{
 assert.equal(nativeAcademyManifestPath(e,l,"author"),`/v1/academy/author/versions/${e}/lessons/${l}/videos`);
 assert.equal(nativeAcademyVideoPath(e,l,m,"author"),`/v1/academy/author/versions/${e}/lessons/${l}/videos/${m}/stream`);
 assert.equal(nativeAcademyVideoPath("../other",l,m,"author"),null);
 assert.equal(nativeAcademyVideoPath(e,l,"https://disk.yandex.ru","author"),null);
 assert.notEqual(nativeAcademyVideoPath(e,l,m,"author"),nativeAcademyVideoPath(e,l,m));
});
