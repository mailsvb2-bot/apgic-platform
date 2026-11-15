// UI-only draft assignment: canonical persistence/authorization remains with Academy API.
export type LessonMediaMapping={lesson_id:string;media_id:string;ordinal:number};

export function appendLessonMediaAssignments<T extends LessonMediaMapping>(
 current:T[],lessonId:string,mediaIds:string[]
):LessonMediaMapping[]{
 if(!lessonId)return current;
 const existing=current.filter(row=>row.lesson_id===lessonId);
 const seen=new Set(existing.map(row=>row.media_id));
 let ordinal=Math.max(0,...existing.map(row=>row.ordinal));
 const additions:LessonMediaMapping[]=[];
 for(const id of mediaIds){
  if(!id||seen.has(id))continue;
  seen.add(id);
  additions.push({lesson_id:lessonId,media_id:id,ordinal:++ordinal});
 }
 return [...current,...additions];
}
