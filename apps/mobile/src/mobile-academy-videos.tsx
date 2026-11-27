import React,{useEffect,useRef,useState} from "react";
import {AppState,Pressable,Text,View} from "react-native";
import Video from "react-native-video";
import {canonicalAPGICOrigin} from "./mobile-deep-link-client.ts";
import {nativeAcademyManifestPath,nativeAcademyVideoPath,nativeAcademyValidatedVideos} from "./mobile-academy-video-url.ts";

type LearnerVideo={id:string;title:string;ordinal:number};
type Manifest={videos:LearnerVideo[]};
export type FetchManifest=(path:string,signal?:AbortSignal)=>Promise<Manifest>;
export default function NativeAcademyLessonVideos({enrollmentId,authorVersionId,lessonId,available,requestManifest}:({enrollmentId:string;authorVersionId?:never}|{authorVersionId:string;enrollmentId?:never})&{lessonId:string;available:boolean;requestManifest:FetchManifest}){
 const scope=authorVersionId?"author":"learner";
 const subjectId=authorVersionId??enrollmentId??"";
 const [videos,setVideos]=useState<LearnerVideo[]>([]);
 const [error,setError]=useState("");
 const [generation,setGeneration]=useState(0);
 // Native callbacks may arrive after an old decoder has been unmounted.
 // Only the currently rendered decoder generation may mutate access state.
 const currentDecoder=useRef("");
 const [visible,setVisible]=useState(AppState.currentState==="active");
 const [failed,setFailed]=useState<{scope:string;ids:Record<string,boolean>}>({scope:"",ids:{}});
 const [blocked,setBlocked]=useState(false);
 const [selectedVideoId,setSelectedVideoId]=useState<string|null>(null);
 const [loadingVideoId,setLoadingVideoId]=useState<string|null>(null);
 // Keep a decoder tied to the exact enrollment/lesson that was verified.
 // React may reuse this component when navigation switches lesson props.
 const scopeKey=`${scope}/${subjectId}/${lessonId}`;
 const [verifiedScope,setVerifiedScope]=useState<string|null>(null);
 const manifestPath=nativeAcademyManifestPath(subjectId,lessonId,scope);
 const decoderIdentity=`${scopeKey}/${generation}/${selectedVideoId??""}`;
 currentDecoder.current=decoderIdentity;

 useEffect(()=>{
  const subscription=AppState.addEventListener("change",state=>{
   // Unmount the native player on background, not merely pause its cursor:
   // the decoded/seek buffer may outlive a revoked entitlement.
   setVisible(state==="active");
   if(state!=="active"){setLoadingVideoId(null);setBlocked(true);setVideos([]);setSelectedVideoId(null);setError("Для возобновления видео подтвердите доступ.");}
  });
  return ()=>subscription.remove();
 },[]);
 // Native decoders may hang without emitting onError (e.g. missing cookies,
 // provider outage). Never leave a black player mounted indefinitely.
 useEffect(()=>{
  if(!loadingVideoId||!visible||blocked||verifiedScope!==scopeKey)return;
  const timer=setTimeout(()=>{
   setSelectedVideoId(null);setLoadingVideoId(null);setBlocked(true);
   setVideos([]);setVerifiedScope(null);
   setError("Время загрузки видео истекло. Проверьте вход и доступ к курсу.");
  },12000);
  return ()=>clearTimeout(timer);
 },[loadingVideoId,generation,scopeKey,visible,blocked,verifiedScope]);
 useEffect(()=>{
  if(!available||!visible||blocked||!manifestPath){
   setLoadingVideoId(null);setVideos([]);setSelectedVideoId(null);setVerifiedScope(null);
   return;
  }
  let disposed=false,checking=false;
  let currentRequest:AbortController|null=null;
  const check=async()=>{
   if(disposed||checking)return;
   checking=true;
   const controller=new AbortController();
   currentRequest=controller;
   // AbortSignal is advisory to a custom transport. A stalled native fetch
   // must never leave a previously authorized decoder running indefinitely.
   let rejectTimeout:(reason?:unknown)=>void=()=>{};
   const deadline=new Promise<never>((_,reject)=>{rejectTimeout=reject;});
   const timeout=setTimeout(()=>{
    controller.abort();
    rejectTimeout(new Error("ACADEMY_MANIFEST_TIMEOUT"));
   },2000);
   try{
    const data=await Promise.race([requestManifest(manifestPath,controller.signal),deadline]);
    if(disposed)return;
    const verified=nativeAcademyValidatedVideos(data,subjectId,lessonId);
    // A binding can change without changing its media ID (title/order), and
    // a new video can be added while a native decoder still holds old bytes.
    // Treat any manifest mutation as an authorization boundary.
    if(!verified|| (verifiedScope===scopeKey && videos.length>0 &&
      (verified.length!==videos.length||videos.some((v,i)=>
        v.id!==verified[i].id||v.title!==verified[i].title||v.ordinal!==verified[i].ordinal)))){
     setLoadingVideoId(null);setBlocked(true);setVideos([]);setSelectedVideoId(null);setError("Видеодоступ изменился или отозван. Повторите проверку.");return;
    }
    if(videos.length===0||verifiedScope!==scopeKey){setVideos(verified);setVerifiedScope(scopeKey);setSelectedVideoId(null);setError("");}
   }catch{
    if(!disposed){setLoadingVideoId(null);setBlocked(true);setVideos([]);setSelectedVideoId(null);setError("Видеодоступ не подтверждён. Проигрыватель остановлен.");}
   }finally{clearTimeout(timeout);if(currentRequest===controller)currentRequest=null;checking=false;}
  };
  void check();
  // The gateway also verifies every media byte write and PostgreSQL
  // entitlement. This poll drops already buffered native bytes on revoke.
  const interval=setInterval(()=>void check(),1000);
  return ()=>{disposed=true;clearInterval(interval);currentRequest?.abort();};
 },[available,visible,blocked,manifestPath,subjectId,lessonId,scopeKey,verifiedScope,generation,videos,requestManifest]);
 if(!available)return <Text>Видеоурок ещё не открыт.</Text>;
 if(!manifestPath)return <Text accessibilityRole="alert">Некорректный идентификатор видеоурока. Доступ закрыт.</Text>;
 return <View style={{gap:10}} accessibilityLabel="Видео Academy">
  {!visible?<Text>Просмотр приостановлен при сворачивании приложения.</Text>:null}
  {error&&visible?<View style={{gap:6}}>
   <Text accessibilityRole="alert">{error}</Text>
   <Pressable accessibilityRole="button" onPress={()=>{setLoadingVideoId(null);setSelectedVideoId(null);setVideos([]);setVerifiedScope(null);setFailed({scope:"",ids:{}});setError("");setBlocked(false);setGeneration(n=>n+1);}}>
    <Text>Проверить доступ повторно</Text>
   </Pressable>
  </View>:null}
  {available&&visible&&!blocked&&verifiedScope===scopeKey&&videos.map(video=>{
   const path=nativeAcademyVideoPath(subjectId,lessonId,video.id,scope);
   if(!path||(failed.scope===scopeKey&&failed.ids[video.id]))return <View key={video.id}>
    <Text>{video.title}: воспроизведение недоступно.</Text>
    <Pressable accessibilityRole="button" onPress={()=>{setLoadingVideoId(null);setSelectedVideoId(null);setVideos([]);setVerifiedScope(null);setFailed({scope:"",ids:{}});setError("");setBlocked(false);setGeneration(n=>n+1);}}>
     <Text>Повторить проверку доступа</Text>
    </Pressable>
   </View>;
   return <View key={video.id} style={{gap:6}}>
    <Pressable accessibilityRole="button" onPress={()=>{setGeneration(n=>n+1);setLoadingVideoId(video.id);setSelectedVideoId(video.id);}}>
     <Text>{video.ordinal}. {video.title}{selectedVideoId===video.id?" · воспроизводится":" · смотреть"}</Text>
    </Pressable>
    {selectedVideoId===video.id&&<Video
     key={`${scopeKey}/${generation}/${video.id}`}
     source={{uri:canonicalAPGICOrigin+path}}
     controls
     onLoad={()=>{if(currentDecoder.current===`${scopeKey}/${generation}/${video.id}`)setLoadingVideoId(current=>current===video.id?null:current);}}
     resizeMode="contain"
     preventsDisplaySleepDuringVideoPlayback
     onError={()=>{if(currentDecoder.current!==`${scopeKey}/${generation}/${video.id}`)return;setLoadingVideoId(null);setFailed(old=>({scope:scopeKey,ids:{...(old.scope===scopeKey?old.ids:{}),[video.id]:true}}));setSelectedVideoId(null);setBlocked(true);setVideos([]);setError("Видео не удалось загрузить. Проверьте вход и доступ к курсу.");}}
     style={{width:"100%",height:220,backgroundColor:"#101827"}}
    />}
    <Text>Видео загружается только через защищённый APGIC. Сохранение офлайн отключено.</Text>
   </View>;
  })}
 </View>;
}
