import assert from "node:assert/strict";
import {test} from "node:test";
import {CLOUD_IMPORT_BATCH,importVerifiedMediaInBatches,type CloudImportItem} from "../app/academy/cloud-media-import.ts";
const items=(n:number):CloudImportItem[]=>Array.from({length:n},(_,i)=>({provider:"YANDEX_DISK",provider_file_ref:`disk:/v/${i}.mp4`,title:`${i}`}));
test("bulk Yandex import reports progress per verified batch",async()=>{
 const progress:Array<[number,number]>=[];
 const result=await importVerifiedMediaInBatches(items(60),async batch=>batch.map(item=>({id:item.provider_file_ref})),
  (done,total)=>progress.push([done,total]));
 assert.equal(result.failure,undefined);assert.equal(result.imported.length,60);
 assert.deepEqual(progress,[[0,60],[25,60],[50,60],[60,60]]);
 assert.ok(CLOUD_IMPORT_BATCH<=100);
});
test("partial failure keeps verified files without resending them",async()=>{
 let calls=0;
 const result=await importVerifiedMediaInBatches(items(60),async batch=>{
  calls++;if(calls===2)throw new Error("Яндекс Диск не отвечает");
  return batch.map(item=>({id:item.provider_file_ref}));
 });
 assert.equal(result.imported.length,25);
 assert.deepEqual(result.importedRefs,items(25).map(item=>item.provider_file_ref));
 assert.match(result.failure||"",/не отвечает/);
});
