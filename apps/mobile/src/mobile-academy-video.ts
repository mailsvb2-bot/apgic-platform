export type AcademyVideoEntry={id:string;title:string;ordinal:number};
const uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
// First-party gateway only. A manifest is not a media grant.
export function academyVideoStreamPath(enrollmentId:string,lessonId:string,mediaId:string):string{
 if(![enrollmentId,lessonId,mediaId].every(value=>uuid.test(value)))throw new Error("ACADEMY_INVALID_VIDEO_ID");
 return "/v1/academy/enrollments/"+enrollmentId+"/lessons/"+lessonId+"/videos/"+mediaId+"/stream";
}
export function academyVerifiedVideoEntries(value:unknown):AcademyVideoEntry[]{
 if(!value||typeof value!=="object"||!Array.isArray((value as {videos?:unknown}).videos))throw new Error("ACADEMY_INVALID_MANIFEST");
 const videos=(value as {videos:unknown[]}).videos;
 if(videos.length>100)throw new Error("ACADEMY_INVALID_MANIFEST");
 const seen=new Set<string>();
 return videos.map(entry=>{
  if(!entry||typeof entry!=="object")throw new Error("ACADEMY_INVALID_MANIFEST");
  const video=entry as Partial<AcademyVideoEntry>;
  if(typeof video.id!=="string"||!uuid.test(video.id)||seen.has(video.id)||
   typeof video.title!=="string"||video.title.length>300||
   typeof video.ordinal!=="number"||!Number.isSafeInteger(video.ordinal)||video.ordinal<0)
   throw new Error("ACADEMY_INVALID_MANIFEST");
  seen.add(video.id);
  return {id:video.id,title:video.title,ordinal:video.ordinal};
 });
}
