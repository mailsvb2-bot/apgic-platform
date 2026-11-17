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

// Reorder only the selected lesson and keep the canonical assignment snapshot
// of every other lesson byte-for-byte unchanged. Sequential ordinals start at 1.
export function moveLessonMediaAssignment<T extends LessonMediaMapping>(
 current:T[],lessonId:string,mediaId:string,direction:-1|1
):LessonMediaMapping[]{
 const lesson=current.filter(row=>row.lesson_id===lessonId).sort((a,b)=>a.ordinal-b.ordinal);
 const index=lesson.findIndex(row=>row.media_id===mediaId);
 const destination=index+direction;
 if(index<0||destination<0||destination>=lesson.length)return current;
 const ordered=[...lesson];
 [ordered[index],ordered[destination]]=[ordered[destination],ordered[index]];
 const updated=ordered.map((row,i)=>({...row,ordinal:i+1}));
 return [...current.filter(row=>row.lesson_id!==lessonId),...updated];
}
