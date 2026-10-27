import React,{useEffect,useRef,useState} from "react";
import {Pressable,Text,View} from "react-native";

type Receipt={id:string;state:string;source:string;created_at:string;updated_at:string;notice:string;idempotent:boolean};
const path="/v1/client/account-deletion-requests";
function parse(v:unknown):Receipt{
  if(!v||typeof v!=="object")throw Error("invalid receipt");
  const x=v as Partial<Receipt>;
  if(!x.id||!x.state||!["WEB","IOS","ANDROID"].includes(x.source??"")||
     !x.created_at||!x.updated_at||typeof x.notice!=="string"||typeof x.idempotent!=="boolean")
    throw Error("invalid receipt");
  return x as Receipt;
}
export default function ClientDeletionRequest({baseURL,source}:{baseURL:string;source:"IOS"|"ANDROID"}){
  const [state,setState]=useState<Receipt|null>(null);
  const [confirmed,setConfirmed]=useState(false);
  const [busy,setBusy]=useState(false);
  const [message,setMessage]=useState("");
  const epoch=useRef(0);
  const alive=useRef(true);
  const load=async()=>{
    const token=++epoch.current;setMessage("");
    try{
      const response=await fetch(new URL(path,baseURL).toString(),{credentials:"include",cache:"no-store"});
      if(!alive.current||token!==epoch.current)return;
      if(response.status===401||response.status===403){setState(null);setMessage("Требуется действующая сессия.");return;}
      if(response.status===404){setState(null);return;}
      if(!response.ok)throw Error("Статус пока недоступен.");
      const result=parse(await response.json());
      if(alive.current&&token===epoch.current)setState(result);
    }catch{if(alive.current&&token===epoch.current)setMessage("Не удалось получить статус запроса.");}
  };
  useEffect(()=>{alive.current=true;void load();return()=>{alive.current=false;epoch.current++;};},[baseURL]);
  const submit=async()=>{
    if(!confirmed||busy||state)return;
    const token=++epoch.current;setBusy(true);setMessage("");
    try{
      const response=await fetch(new URL(path,baseURL).toString(),{
        method:"POST",credentials:"include",cache:"no-store",headers:{"content-type":"application/json"},
        body:JSON.stringify({source}),
      });
      if(!alive.current||token!==epoch.current)return;
      if(response.status===401||response.status===403){setState(null);setMessage("Требуется действующая сессия.");return;}
      if(!response.ok)throw Error("request unavailable");
      const result=parse(await response.json());
      if(alive.current&&token===epoch.current)setState(result);
    }catch{if(alive.current&&token===epoch.current)setMessage("Запрос не зарегистрирован. Повторите попытку.");}
    finally{if(alive.current&&token===epoch.current)setBusy(false);}
  };
  return <View accessibilityLabel="Удаление учётной записи" style={{gap:8,paddingVertical:14}}>
    <Text style={{fontWeight:"700"}}>Удаление учётной записи</Text>
    <Text>Это запрос, а не завершённое стирание. Данные могут временно сохраняться согласно закону.</Text>
    {state?<View><Text>Запрос: {state.id}</Text><Text>Статус: {state.state}</Text>
      <Text>{state.notice}</Text></View>:
    <View style={{gap:8}}>
      <Pressable accessibilityRole="checkbox" accessibilityState={{checked:confirmed}}
        onPress={()=>setConfirmed(v=>!v)} style={{padding:12,borderWidth:1}}>
        <Text>{confirmed?"☑":"☐"} Понимаю, что подаю запрос, а не удаляю данные немедленно</Text>
      </Pressable>
      <Pressable accessibilityRole="button" disabled={!confirmed||busy}
        onPress={()=>void submit()} style={{padding:12,borderWidth:1}}>
        <Text>{busy?"Отправляем…":"Подать запрос на удаление"}</Text>
      </Pressable>
    </View>}
    <Pressable accessibilityRole="button" onPress={()=>void load()} style={{padding:12,borderWidth:1}}>
      <Text>Обновить статус удаления</Text>
    </Pressable>
    {message?<Text accessibilityRole="alert">{message}</Text>:null}
  </View>;
}
