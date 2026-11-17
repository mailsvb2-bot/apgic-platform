// Each batch is independently verified by the canonical Academy API.
export type CloudImportItem={provider:string;provider_file_ref:string;title:string};
export type CloudImportBatch<T>=(items:CloudImportItem[])=>Promise<T[]>;
export type CloudImportResult<T>={imported:T[];importedRefs:string[];failure?:string};
export const CLOUD_IMPORT_BATCH=25;
export async function importVerifiedMediaInBatches<T>(
 items:CloudImportItem[],importBatch:CloudImportBatch<T>,onProgress?:(done:number,total:number)=>void
):Promise<CloudImportResult<T>>{
 const imported:T[]=[];const importedRefs:string[]=[];
 onProgress?.(0,items.length);
 for(let i=0;i<items.length;i+=CLOUD_IMPORT_BATCH){
  const batch=items.slice(i,i+CLOUD_IMPORT_BATCH);
  try{
   const result=await importBatch(batch);
   if(result.length!==batch.length)return {imported,importedRefs,failure:"Импорт выполнен не полностью. Обновите медиатеку перед назначением."};
   imported.push(...result);importedRefs.push(...batch.map(item=>item.provider_file_ref));
  }catch(error){return {imported,importedRefs,failure:error instanceof Error?error.message:"Операция не выполнена"};}
  onProgress?.(Math.min(i+batch.length,items.length),items.length);
 }
 return {imported,importedRefs};
}
