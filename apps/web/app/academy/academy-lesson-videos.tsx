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
 const [playingIds,setPlayingIds]=useState<string[]>([]);
 const refs=useRef<Record<string,HTMLVideoElement|null>>({});
 useEffect(()=>{
  setPlayingIds([]);
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
 // A previously buffered HTML5 video can continue playing even after the
 // server closes its current stream. Recheck canonical learner access while
 // playback is active and discard buffered bytes if it is no longer proven.
 useEffect(()=>{
  if(!available||playingIds.length===0)return;
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
    if(!Array.isArray(data.videos)||playingIds.some(id=>!data.videos?.some(video=>video.id===id))){
     throw new Error("Video no longer authorized");
    }
   }catch{
    if(!disposed){
     // pause() alone retains the browser buffer; unload the source too.
     for(const video of Object.values(refs.current)){
      if(video){video.pause();video.removeAttribute("src");video.load()}
     }
     setPlayingIds([]);
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
 },[available,enrollmentId,lessonId,playingIds]);
 function retryVideo(id:string){
  const player=refs.current[id];
  if(!player)return;
  setPlaybackError(old=>({...old,[id]:""}));
  // Retry the same authenticated APGIC stream URL, never a cloud origin.
  // load() forces a new request after a network/entitlement error.
  player.load();
  void player.play().catch(()=>{
   setPlaybackError(old=>({...old,[id]:"Не удалось возобновить просмотр. Проверьте доступ к уроку."}));
  });
 }
 if(!available)return <p>Доступ к видео сейчас закрыт.</p>;
 return <section aria-label="Видеоматериалы урока">
  {error&&<div role="alert"><p>{error}</p>
   <button type="button" onClick={()=>{setError("");setReloadVersion(value=>value+1)}}>Проверить доступ повторно</button>
  </div>}
  {videos.map(video=><div key={video.id}>
   <h4>{video.ordinal}. {video.title}</h4>
   <div style={{position:"relative",maxWidth:900,background:"#101827",borderRadius:12,overflow:"hidden"}}>
    <video controls playsInline preload="metadata" controlsList="nodownload"
     ref={node=>{refs.current[video.id]=node}}
     onPlay={()=>setPlayingIds(current=>current.includes(video.id)?current:[...current,video.id])}
     onPause={()=>setPlayingIds(current=>current.filter(id=>id!==video.id))}
     onEnded={()=>setPlayingIds(current=>current.filter(id=>id!==video.id))}
     onError={event=>{
      setPlayingIds(current=>current.filter(id=>id!==video.id));
      const code=event.currentTarget.error?.code;
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
