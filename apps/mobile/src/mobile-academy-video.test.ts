import assert from "node:assert/strict";
import test from "node:test";
import {academyVerifiedVideoEntries,academyVideoStreamPath} from "./mobile-academy-video.ts";
const a="11111111-1111-4111-8111-111111111111",b="22222222-2222-4222-8222-222222222222",c="33333333-3333-4333-8333-333333333333";
test("video URL stays on canonical APGIC gateway",()=>{
 assert.equal(academyVideoStreamPath(a,b,c),"/v1/academy/enrollments/"+a+"/lessons/"+b+"/videos/"+c+"/stream");
 for(const id of ["https://disk.yandex.ru/file","../cloud","","not-a-uuid"])assert.throws(()=>academyVideoStreamPath(a,b,id),/ACADEMY_INVALID_VIDEO_ID/);
});
test("manifest strips provider URL and rejects duplicate or invalid ids",()=>{
 assert.deepEqual(academyVerifiedVideoEntries({videos:[{id:c,title:"Lecture",ordinal:1,url:"https://attacker.invalid"}]}),[{id:c,title:"Lecture",ordinal:1}]);
 for(const value of [null,{videos:null},{videos:[{id:c,title:"A",ordinal:1},{id:c,title:"B",ordinal:2}]},{videos:[{id:"/foo",title:"A",ordinal:1}]}])assert.throws(()=>academyVerifiedVideoEntries(value),/ACADEMY_INVALID_MANIFEST/);
});
