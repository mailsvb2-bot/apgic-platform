import React,{useEffect,useState} from "react";
import {AppState,Pressable,Text,View} from "react-native";
import Video from "react-native-video";
import {canonicalAPGICOrigin} from "./mobile-deep-link-client.ts";
import {nativeAcademyVideoPath} from "./mobile-academy-video-url.ts";

type LearnerVideo={id:string;title:string;ordinal:number};
type Manifest={videos:LearnerVideo[]};
export type FetchManifest=(path:string)=>Promise<Manifest>;
export default function NativeAcademyLessonVideos({enrollmentId,lessonId,available,requestManifest}:{
 enrollmentId:string;lessonId:string;available:boolean;requestManifest:FetchManifest
}){
 const [videos,setVideos]=useState<LearnerVideo[]>([]);
 const [error,setError]=useState("");
 const [generation,setGeneration]=useState(0);
 const [visible,setVisible]=useState(AppState.currentState==="active");
 const [failed,setFailed]=useState<Record<string,boolean>>({});
 const [blocked,setBlocked]=useState(false);
 const manifestPath=`/v1/academy/enrollments/${encodeURIComponent(enrollmentId)}/lessons/${encodeURIComponent(lessonId)}/videos`;

 useEffect(()=>{
  const subscription=AppState.addEventListener("change",state=>{
   // Unmount the native player on background, not merely pause its cursor:
   // the decoded/seek buffer may outlive a revoked entitlement.
   setVisible(state==="active");
   if(state!=="active"){setBlocked(true);setVideos([]);setError("Для возобновления видео подтвердите доступ.");}
  });
  return ()=>subscription.remove();
 },[]);
 useEffect(()=>{
  if(!available||!visible||blocked){
   setVideos([]);
   return;
  }
  let disposed=false,checking=false;
  const check=async()=>{
   if(disposed||checking)return;
   checking=true;
   try{
    const data=await requestManifest(manifestPath);
    if(disposed)return;
    if(!Array.isArray(data.videos)||data.videos.length===0||
     data.videos.some(v=>!nativeAcademyVideoPath(enrollmentId,lessonId,v.id))||
     videos.some(v=>!data.videos.some(next=>next.id===v.id))){
     setBlocked(true);setVideos([]);setError("Видеодоступ изменился или отозван. Повторите проверку.");return;
    }
    if(videos.length===0){setVideos(data.videos);setError("");}
   }catch{
    if(!disposed){setBlocked(true);setVideos([]);setError("Видеодоступ не подтверждён. Проигрыватель остановлен.");}
   }finally{checking=false;}
  };
  void check();
  // The gateway also verifies every media byte write and PostgreSQL
  // entitlement. This poll drops already buffered native bytes on revoke.
  const interval=setInterval(()=>void check(),1000);
  return ()=>{disposed=true;clearInterval(interval);};
 },[available,visible,blocked,manifestPath,enrollmentId,lessonId,generation,videos,requestManifest]);
 if(!available)return <Text>Видеоурок ещё не открыт.</Text>;
 return <View style={{gap:10}} accessibilityLabel="Видео Academy">
  {!visible?<Text>Просмотр приостановлен при сворачивании приложения.</Text>:null}
  {error?<View style={{gap:6}}>
   <Text accessibilityRole="alert">{error}</Text>
   <Pressable accessibilityRole="button" onPress={()=>{setFailed({});setBlocked(false);setGeneration(n=>n+1);}}>
    <Text>Проверить доступ повторно</Text>
   </Pressable>
  </View>:null}
  {visible&&videos.map(video=>{
   const path=nativeAcademyVideoPath(enrollmentId,lessonId,video.id);
   if(!path||failed[video.id])return <View key={video.id}>
    <Text>{video.title}: воспроизведение недоступно.</Text>
    <Pressable accessibilityRole="button" onPress={()=>{setVideos([]);setBlocked(false);setGeneration(n=>n+1);}}>
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
     onError={()=>{setFailed(old=>({...old,[video.id]:true}));setBlocked(true);setVideos([]);setError("Видео не удалось загрузить. Проверьте вход и доступ к курсу.");}}
     style={{width:"100%",height:220,backgroundColor:"#101827"}}
    />
    <Text>Видео загружается только через защищённый APGIC. Сохранение офлайн отключено.</Text>
   </View>;
  })}
 </View>;
}
