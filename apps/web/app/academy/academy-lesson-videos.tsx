"use client";

import {useEffect,useRef,useState} from "react";

type LessonVideo={id:string;title:string;ordinal:number};

export default function AcademyLessonVideos({enrollmentId,lessonId,available}:{
 enrollmentId:string;lessonId:string;available:boolean
}){
 const [videos,setVideos]=useState<LessonVideo[]>([]);
 const [error,setError]=useState("");
 const [playbackError,setPlaybackError]=useState<Record<string,string>>({});
 const refs=useRef<Record<string,HTMLVideoElement|null>>({});
 useEffect(()=>{
  if(!available){setVideos([]);return}
  const controller=new AbortController();
  void (async()=>{
   try{
    const response=await fetch(`/v1/academy/enrollments/${enrollmentId}/lessons/${lessonId}/videos`,{
     credentials:"same-origin",cache:"no-store",signal:controller.signal
    });
    if(!response.ok)throw new Error("Видеоматериалы пока недоступны");
    const data=await response.json() as {videos:LessonVideo[]};
    if(!controller.signal.aborted){setVideos(data.videos);setError("")}
   }catch{if(!controller.signal.aborted){setError("Не удалось получить список видео. Повторите открытие урока.");setVideos([])}}
  })();
  return ()=>{
   controller.abort();
   for(const video of Object.values(refs.current)){if(video){video.pause();video.removeAttribute("src");video.load()}}
  };
 },[available,enrollmentId,lessonId]);
 if(!available)return <p>Доступ к видео сейчас закрыт.</p>;
 return <section aria-label="Видеоматериалы урока">
  {error&&<p role="status">{error}</p>}
  {videos.map(video=><div key={video.id}>
   <h4>{video.ordinal}. {video.title}</h4>
   <div style={{position:"relative",maxWidth:900,background:"#101827",borderRadius:12,overflow:"hidden"}}>
    <video controls playsInline preload="metadata" controlsList="nodownload"
     ref={node=>{refs.current[video.id]=node}}
     onError={()=>setPlaybackError(prev=>({...prev,[video.id]:"Не удалось воспроизвести видео. Проверьте доступ к курсу или попробуйте ещё раз."}))}
     onPlaying={()=>setPlaybackError(prev=>({...prev,[video.id]:""}))}
     aria-label={video.title}
     style={{width:"100%",display:"block"}}
     src={`/v1/academy/enrollments/${enrollmentId}/lessons/${lessonId}/videos/${video.id}/stream`}>
     Ваш браузер не поддерживает HTML5-видео.
    </video>
    <div style={{position:"absolute",top:12,left:12,pointerEvents:"none",color:"white",fontWeight:700,textShadow:"0 1px 4px black"}}>APGIC Academy</div>
   </div>
   {playbackError[video.id]&&<p role="alert">{playbackError[video.id]}</p>}
   <p>APGIC Academy · видеоматериал автора курса</p>
  </div>)}
 </section>
}
