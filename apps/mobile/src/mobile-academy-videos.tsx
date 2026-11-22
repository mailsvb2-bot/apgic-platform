import React,{useCallback,useEffect,useState} from "react";
import {AppState,Pressable,Text,View} from "react-native";
import Video from "react-native-video";
import {canonicalAPGICOrigin} from "./mobile-deep-link-client.ts";

type LearnerVideo={id:string;title:string;ordinal:number};
type Manifest={videos:LearnerVideo[]};
export type FetchManifest=(path:string)=>Promise<Manifest>;
const idPattern=/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

// The only media origin in the native app is the authenticated APGIC gateway.
// Author paths, signed cloud URLs and OAuth credentials must never enter it.
export function nativeAcademyVideoPath(enrollmentId:string,lessonId:string,mediaId:string):string|null{
 if(![enrollmentId,lessonId,mediaId].every(id=>idPattern.test(id)))return null;
 return `/v1/academy/enrollments/${enrollmentId}/lessons/${lessonId}/videos/${mediaId}/stream`;
}

export default function NativeAcademyLessonVideos({enrollmentId,lessonId,available,requestManifest}:{
 enrollmentId:string;lessonId:string;available:boolean;requestManifest:FetchManifest
}){
 const [videos,setVideos]=useState<LearnerVideo[]>([]);
 const [error,setError]=useState("");
 const [generation,setGeneration]=useState(0);
 const [visible,setVisible]=useState(AppState.currentState==="active");
 const [failed,setFailed]=useState<Record<string,boolean>>({});
 const manifestPath=`/v1/academy/enrollments/${encodeURIComponent(enrollmentId)}/lessons/${encodeURIComponent(lessonId)}/videos`;
 const load=useCallback(async()=>{
  if(!available||!visible)return;
  try{
   const result=await requestManifest(manifestPath);
   if(!Array.isArray(result.videos)||result.videos.length===0||
    result.videos.some(v=>!nativeAcademyVideoPath(enrollmentId,lessonId,v.id))){
    throw new Error("Invalid video manifest");
   }
   setVideos(result.videos);setFailed({});setError("");
  }catch{
   setVideos([]);setError("Видео или подтверждённый доступ сейчас недоступны.");
  }
 },[available,visible,manifestPath,enrollmentId,lessonId]);
 useEffect(()=>{
  const subscription=AppState.addEventListener("change",state=>{
   // Unmount the native player on background, not merely pause its cursor:
   // the decoded/seek buffer may outlive a revoked entitlement.
   setVisible(state==="active");
   if(state!=="active"){setVideos([]);setError("Для возобновления видео подтвердите доступ.");}
  });
  return ()=>subscription.remove();
 },[]);
 useEffect(()=>{
  if(!available||!visible){
   setVideos([]);
   return;
  }
  let disposed=false,checking=false;
  const check=async()=>{
   if(disposed||checking)return;
   checking=true;
   try{
    const data=await academyRequest<Manifest>(manifestPath);
    if(disposed)return;
    if(!Array.isArray(data.videos)||data.videos.length===0||
     data.videos.some(v=>!nativeAcademyVideoPath(enrollmentId,lessonId,v.id))||
     videos.some(v=>!data.videos.some(next=>next.id===v.id))){
     setVideos([]);setError("Видеодоступ изменился или отозван. Повторите проверку.");return;
    }
    if(videos.length===0){setVideos(data.videos);setError("");}
   }catch{
    if(!disposed){setVideos([]);setError("Видеодоступ не подтверждён. Проигрыватель остановлен.");}
   }finally{checking=false;}
  };
  void check();
  // The gateway also verifies every media byte write and PostgreSQL
  // entitlement. This poll drops already buffered native bytes on revoke.
  const interval=setInterval(()=>void check(),1000);
  return ()=>{disposed=true;clearInterval(interval);};
 },[available,visible,manifestPath,enrollmentId,lessonId,generation,videos]);
 if(!available)return <Text>Видеоурок ещё не открыт.</Text>;
 return <View style={{gap:10}} accessibilityLabel="Видео Academy">
  {!visible?<Text>Просмотр приостановлен при сворачивании приложения.</Text>:null}
  {error?<View style={{gap:6}}>
   <Text accessibilityRole="alert">{error}</Text>
   <Pressable accessibilityRole="button" onPress={()=>{setFailed({});setGeneration(n=>n+1);}}>
    <Text>Проверить доступ повторно</Text>
   </Pressable>
  </View>:null}
  {visible&&videos.map(video=>{
   const path=nativeAcademyVideoPath(enrollmentId,lessonId,video.id);
   if(!path||failed[video.id])return <View key={video.id}>
    <Text>{video.title}: воспроизведение недоступно.</Text>
    <Pressable accessibilityRole="button" onPress={()=>{setVideos([]);setGeneration(n=>n+1);}}>
     <Text>Повторить проверку доступа</Text>
    </Pressable>
   </View>;
   return <View key={video.id} style={{gap:6}}>
    <Text>{video.ordinal}. {video.title}</Text>
    <Video
     source={{uri:canonicalAPGICOrigin+path}}
     paused={false}
     controls
     resizeMode="contain"
     preventsDisplaySleepDuringVideoPlayback
     onError={()=>{setFailed(old=>({...old,[video.id]:true}));setError("Видео не удалось загрузить. Проверьте вход и доступ к курсу.");}}
     style={{width:"100%",height:220,backgroundColor:"#101827"}}
    />
    <Text>Видео загружается только через защищённый APGIC. Сохранение офлайн отключено.</Text>
   </View>;
  })}
 </View>;
}
