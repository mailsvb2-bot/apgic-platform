import type {MetadataRoute} from "next";

export const dynamic="force-dynamic";

// Only canonical pages passing the live, qualification-backed Search Gate
// are submitted to crawlers; no synthetic topic x city combinations.
const slugs=["trevoga","son","otnosheniya","kariera"];
export default async function sitemap():Promise<MetadataRoute.Sitemap>{
 const origin=process.env.APGIC_API_ORIGIN||"http://127.0.0.1:43111";
 const indexable=await Promise.all(slugs.map(async slug=>{
  try{
   const r=await fetch(new URL("/v1/public/help/"+slug,origin),{cache:"no-store"});
   if(!r.ok)return null;
   const p=await r.json() as {indexable?:boolean;topic?:{slug?:string}};
   return p.indexable&&p.topic?.slug===slug?{url:"https://apgic.ru/help/"+slug}:null;
  }catch{return null;}
 }));
 return [{url:"https://apgic.ru"},...indexable.filter((x):x is {url:string}=>x!==null)];
}
