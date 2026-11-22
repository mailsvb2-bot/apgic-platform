import React,{useEffect,useState} from "react";
import {AppState,Pressable,Text,View} from "react-native";
import Video from "react-native-video";
import {canonicalAPGICOrigin} from "./mobile-deep-link-client.ts";
import {nativeAcademyVideoPath,nativeAcademyValidatedVideos} from "./mobile-academy-video-url.ts";

type LearnerVideo={id:string;title:string;ordinal:number};
type Manifest={videos:LearnerVideo[]};
export type FetchManifest=(path:string,signal?:AbortSignal)=>Promise<Manifest>;
export default function NativeAcademyLessonVideos({enrollmentId,lessonId,available,requestManifest}:{
 enrollmentId:string;lessonId:string;available:boolean;requestManifest:FetchManifest
}){
 const [videos,setVideos]=useState<LearnerVideo[]>([]);
 const [error,setError]=useState("");
 const [generation,setGeneration]=useState(0);
 const [visible,setVisible]=useState(AppState.currentState!=="background"&&AppState.currentState!=="inactive");
 const [failed,setFailed]=useState<Record<string,boolean>>({});
 const [blocked,setBlocked]=useState(false);
 const [selectedVideoId,setSelectedVideoId]=useState<string|null>(null);
 const manifestPath=`/v1/academy/enrollments/${encodeURIComponent(enrollmentId)}/lessons/${encodeURIComponent(lessonId)}/videos`;

 useEffect(()=>{
  const subscription=AppState.addEventListener("change",state=>{
   // Unmount the native player on background, not merely pause its cursor:
   // the decoded/seek buffer may outlive a revoked entitlement.
   setVisible(state==="active");
   if(state!=="active"){setBlocked(true);setVideos([]);setSelectedVideoId(null);setError("Для возобновления видео подтвердите доступ.");}
  });
  return ()=>subscription.remove();
 },[]);
 useEffect(()=>{
  if(!available||!visible||blocked){
   setVideos([]);setSelectedVideoId(null);
   return;
  }
  let disposed=false,checking=false;
  let currentRequest:AbortController|null=null;
  const check=async()=>{
   if(disposed||checking)return;
   checking=true;
   const controller=new AbortController();
   currentRequest=controller;
   const timeout=setTimeout(()=>controller.abort(),2000);
   try{
    const data=await requestManifest(manifestPath,controller.signal);
    if(disposed)return;
    const verified=nativeAcademyValidatedVideos(data,enrollmentId,lessonId);
    // A binding can change without changing its media ID (title/order), and
    // a new video can be added while a native decoder still holds old bytes.
    // Treat any manifest mutation as an authorization boundary.
    if(!verified|| (videos.length>0 &&
      (verified.length!==videos.length||videos.some((v,i)=>
        v.id!==verified[i].id||v.title!==verified[i].title||v.ordinal!==verified[i].ordinal)))){
     setBlocked(true);setVideos([]);setSelectedVideoId(null);setError("Видеодоступ изменился или отозван. Повторите проверку.");return;
    }
    if(videos.length===0){setVideos(verified);setError("");}
   }catch{
    if(!disposed){setBlocked(true);setVideos([]);setSelectedVideoId(null);setError("Видеодоступ не подтверждён. Проигрыватель остановлен.");}
   }finally{clearTimeout(timeout);if(currentRequest===controller)currentRequest=null;checking=false;}
  };
  void check();
  // The gateway also verifies every media byte write and PostgreSQL
  // entitlement. This poll drops already buffered native bytes on revoke.
  const interval=setInterval(()=>void check(),1000);
  return ()=>{disposed=true;clearInterval(interval);currentRequest?.abort();};
 },[available,visible,blocked,manifestPath,enrollmentId,lessonId,generation,videos,requestManifest]);
 if(!available)return <Text>Видеоурок ещё не открыт.</Text>;
 return <View style={{gap:10}} accessibilityLabel="Видео Academy">
  {!visible?<Text>Просмотр приостановлен при сворачивании приложения.</Text>:null}
  {error?<View style={{gap:6}}>
   <Text accessibilityRole="alert">{error}</Text>
   <Pressable accessibilityRole="button" onPress={()=>{setSelectedVideoId(null);setFailed({});setBlocked(false);setGeneration(n=>n+1);}}>
    <Text>Проверить доступ повторно</Text>
   </Pressable>
  </View>:null}
  {available&&visible&&!blocked&&videos.map(video=>{
   const path=nativeAcademyVideoPath(enrollmentId,lessonId,video.id);
   if(!path||failed[video.id])return <View key={video.id}>
    <Text>{video.title}: воспроизведение недоступно.</Text>
    <Pressable accessibilityRole="button" onPress={()=>{setSelectedVideoId(null);setVideos([]);setBlocked(false);setGeneration(n=>n+1);}}>
     <Text>Повторить проверку доступа</Text>
    </Pressable>
   </View>;
   return <View key={video.id} style={{gap:6}}>
    <Pressable accessibilityRole="button" onPress={()=>setSelectedVideoId(video.id)}>
     <Text>{video.ordinal}. {video.title}{selectedVideoId===video.id?" · воспроизводится":" · смотреть"}</Text>
    </Pressable>
    {selectedVideoId===video.id&&<Video
     source={{uri:canonicalAPGICOrigin+path}}
     controls
     resizeMode="contain"
     preventsDisplaySleepDuringVideoPlayback
     onError={()=>{setFailed(old=>({...old,[video.id]:true}));setSelectedVideoId(null);setBlocked(true);setVideos([]);setError("Видео не удалось загрузить. Проверьте вход и доступ к курсу.");}}
     style={{width:"100%",height:220,backgroundColor:"#101827"}}
    />}
    <Text>Видео загружается только через защищённый APGIC. Сохранение офлайн отключено.</Text>
   </View>;
  })}
 </View>;
}
