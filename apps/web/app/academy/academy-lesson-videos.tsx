"use client";

import {useEffect,useRef,useState} from "react";

type LessonVideo={id:string;title:string;ordinal:number};

export default function AcademyLessonVideos({enrollmentId,lessonId,available}:{
 enrollmentId:string;lessonId:string;available:boolean
}){
 const [videos,setVideos]=useState<LessonVideo[]>([]);
 const [error,setError]=useState("");
 const [playbackError,setPlaybackError]=useState<Record<string,string>>({});
 const [reloadVersion,setReloadVersion]=useState(0);
 const [openedIds,setOpenedIds]=useState<string[]>([]);
 const refs=useRef<Record<string,HTMLVideoElement|null>>({});
 useEffect(()=>{
  setOpenedIds([]);
  if(!available){setVideos([]);setError("");return}
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
 },[available,enrollmentId,lessonId,reloadVersion]);
 // A previously buffered HTML5 video can continue playing after the server
 // closes its stream. Once a video has started, recheck canonical access even
 // while paused, and discard buffered bytes if access is no longer proven.
 useEffect(()=>{
  if(!available||openedIds.length===0)return;
  let disposed=false;
  let checking=false;
  const controller=new AbortController();
  async function revalidate(){
   if(disposed||checking)return;
   checking=true;
   const timeout=window.setTimeout(()=>controller.abort(),2000);
   try{
    const response=await fetch(`/v1/academy/enrollments/${enrollmentId}/lessons/${lessonId}/videos`,{
     credentials:"same-origin",cache:"no-store",signal:controller.signal
    });
    if(!response.ok)throw new Error("Access unavailable");
    const data=await response.json() as {videos?:LessonVideo[]};
    if(!Array.isArray(data.videos)||openedIds.some(id=>!data.videos?.some(video=>video.id===id))){
     throw new Error("Video no longer authorized");
    }
   }catch{
    if(!disposed){
     // pause() alone retains the browser buffer; unload the source too.
     for(const video of Object.values(refs.current)){
      if(video){video.pause();video.removeAttribute("src");video.load()}
     }
     setOpenedIds([]);
     setVideos([]);
     setError("Доступ к видео не подтверждён или был отозван. Проверьте доступ повторно.");
    }
   }finally{
    window.clearTimeout(timeout);
    checking=false;
   }
  }
  void revalidate();
  const timer=window.setInterval(()=>{void revalidate()},1000);
  return ()=>{disposed=true;window.clearInterval(timer);controller.abort()};
 },[available,enrollmentId,lessonId,openedIds]);
 async function retryVideo(id:string){
  const player=refs.current[id];
  if(!player)return;
  setPlaybackError(old=>({...old,[id]:""}));
  try{
   // A failed media element has no reusable source or trustworthy buffer.
   // Prove the canonical entitlement again before creating a new request.
   const response=await fetch(`/v1/academy/enrollments/${enrollmentId}/lessons/${lessonId}/videos`,{
    credentials:"same-origin",cache:"no-store"
   });
   if(!response.ok)throw new Error("Access unavailable");
   const data=await response.json() as {videos?:LessonVideo[]};
   if(!Array.isArray(data.videos)||!data.videos.some(video=>video.id===id)){
    throw new Error("Video no longer authorized");
   }
   if(refs.current[id]!==player)return;
   // Only the APGIC gateway URL is ever placed in the browser, never cloud hrefs.
   player.src=`/v1/academy/enrollments/${enrollmentId}/lessons/${lessonId}/videos/${id}/stream`;
   player.load();
   await player.play();
  }catch{
   if(refs.current[id]!==player)return;
   player.pause();player.removeAttribute("src");player.load();
   setPlaybackError(old=>({...old,[id]:"Не удалось подтвердить доступ или возобновить просмотр. Проверьте доступ к уроку."}));
  }
 }
 if(!available)return <p>Доступ к видео сейчас закрыт.</p>;
 return <section aria-label="Видеоматериалы урока">
  {error&&<div role="alert"><p>{error}</p>
   <button type="button" onClick={()=>{setError("");setReloadVersion(value=>value+1)}}>Проверить доступ повторно</button>
  </div>}
  {videos.map(video=><div key={video.id}>
   <h4>{video.ordinal}. {video.title}</h4>
   <div style={{position:"relative",maxWidth:900,background:"#101827",borderRadius:12,overflow:"hidden"}}>
    <video controls playsInline preload="none" controlsList="nodownload"
     ref={node=>{refs.current[video.id]=node}}
     onPlay={()=>setOpenedIds(current=>current.includes(video.id)?current:[...current,video.id])}
     onError={event=>{
      // A failed stream may still have playable buffered bytes. Never stop
      // entitlement polling while leaving those bytes attached to the player.
      const player=event.currentTarget;
      const code=player.error?.code;
      player.pause();player.removeAttribute("src");player.load();
      setOpenedIds(current=>current.filter(id=>id!==video.id));
      const message=code===3||code===4
       ? "Браузер не поддерживает формат этого видео или файл повреждён."
       : "Видео недоступно. Проверьте доступ к курсу и повторите загрузку.";
      setPlaybackError(prev=>({...prev,[video.id]:message}));
     }}
     onPlaying={()=>setPlaybackError(prev=>({...prev,[video.id]:""}))}
     aria-label={video.title}
     style={{width:"100%",display:"block"}}
     src={`/v1/academy/enrollments/${enrollmentId}/lessons/${lessonId}/videos/${video.id}/stream`}>
     Ваш браузер не поддерживает HTML5-видео.
    </video>
    <div style={{position:"absolute",top:12,left:12,pointerEvents:"none",color:"white",fontWeight:700,textShadow:"0 1px 4px black"}}>APGIC Academy</div>
   </div>
   {playbackError[video.id]&&<div role="alert">
    <p>{playbackError[video.id]}</p>
    <button type="button" onClick={()=>retryVideo(video.id)}>Повторить загрузку видео</button>
   </div>}
   <p>APGIC Academy · видеоматериал автора курса</p>
  </div>)}
 </section>
}
