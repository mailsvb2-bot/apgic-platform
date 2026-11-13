import assert from "node:assert/strict";
import test from "node:test";
import {AcademyAccessError,academyOptional,academySessionRejected} from "./mobile-academy-session.ts";

test("401 is a non-optional Academy identity boundary",async()=>{
 const failure=new AcademyAccessError(401,"UNAUTHORIZED");
 assert.equal(academySessionRejected(failure),true);
 await assert.rejects(academyOptional(Promise.reject(failure),{items:[]}),error=>error===failure);
 assert.equal(academySessionRejected(new AcademyAccessError(403,"ACCESS_DENIED")),false);
 assert.equal(academySessionRejected(new Error("network offline")),false);
});
test("non-auth optional Academy metadata errors preserve normal fallback",async()=>{
 const value={items:[] as string[]};
 assert.deepEqual(await academyOptional(Promise.reject(new Error("offline")),value),value);
 assert.deepEqual(await academyOptional(Promise.resolve({items:["cohort"]}),value),{items:["cohort"]});
});
