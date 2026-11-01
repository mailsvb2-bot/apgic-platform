"use client";

import {useEffect,useState} from "react";

type LessonVideo={id:string;title:string;ordinal:number};

// Render only APGIC video IDs. Provider source URLs and OAuth credentials
// are deliberately absent from this component and its API responses.
export default function AcademyLessonVideos({enrollmentId,lessonId,available}:{
 enrollmentId:string;lessonId:string;available:boolean
}){
 const [videos,setVideos]=useState<LessonVideo[]>([]);
 const [error,setError]=useState("");
 useEffect(()=>{
  if(!available){setVideos([]);return}
  const controller=new AbortController();
  void (async()=>{
   try{
    const r=await fetch(`/v1/academy/enrollments/${enrollmentId}/lessons/${lessonId}/videos`,{
     credentials:"same-origin",cache:"no-store",signal:controller.signal
    });
    if(!r.ok)throw new Error("Видеоматериалы пока недоступны");
    const body=await r.json() as {videos:LessonVideo[]};
    if(!controller.signal.aborted){setVideos(body.videos);setError("")}
   }catch(e){if(!controller.signal.aborted){setError(String(e));setVideos([])}}
  })();
  return ()=>controller.abort();
 },[available,enrollmentId,lessonId]);
 if(!available)return <p>Доступ к видео сейчас закрыт.</p>;
 return <section aria-label="Видеоматериалы урока">
  {error&&<p role="status">{error}</p>}
  {videos.map(v=><div key={v.id}>
   <h4>{v.ordinal}. {v.title}</h4>
   <video controls playsInline preload="metadata" controlsList="nodownload" style={{width:"100%",maxWidth:900}}
    src={`/v1/academy/enrollments/${enrollmentId}/lessons/${lessonId}/videos/${v.id}/stream`}>
    Ваш браузер не поддерживает воспроизведение HTML5-видео.
   </video>
   <p>APGIC Academy · материалы автора курса</p>
  </div>)}
 </section>
}
