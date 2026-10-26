import React,{useCallback,useEffect,useRef,useState} from "react";
import {Pressable,Text,TextInput,View} from "react-native";
import {
  parseClientPersonalView,personalBookingLabel,personalProgressLabel,
  type ClientPersonalCheckInInput,
  type ClientPersonalView,type ClientPersonalPreferences,
} from "../../../packages/contracts/src/client-personal.ts";

// Server-owned data only, never persisted on the device or sent to telemetry.
export default function ClientPersonalPanel({baseURL}:{baseURL:string}) {
  const [view,setView]=useState<ClientPersonalView|null>(null);
  const [status,setStatus]=useState<"loading"|"ready"|"auth"|"error">("loading");
  const [message,setMessage]=useState("");
  const [editing,setEditing]=useState<ClientPersonalPreferences|null>(null);
  const [busy,setBusy]=useState(false);
  const [paging,setPaging]=useState(false);
  const epoch=useRef(0);
  const pagingInFlight=useRef(false);
  const load=useCallback(async()=>{
    const generation=++epoch.current;
    pagingInFlight.current=false;
    setPaging(false);
    setStatus("loading");setView(null);setEditing(null);setMessage("");
    try{
      const response=await fetch(new URL("/v1/client/personal",baseURL).toString(),{
        credentials:"include",headers:{Accept:"application/json"},cache:"no-store",
      });
      if(generation!==epoch.current)return;
      if(response.status===401||response.status===403){setStatus("auth");return;}
      if(!response.ok)throw new Error("Данные кабинета пока недоступны.");
      const value=parseClientPersonalView(await response.json());
      if(generation!==epoch.current)return;
      setView(value);setEditing(value.preferences);setStatus("ready");
    }catch{
      if(generation!==epoch.current)return;
      setMessage("Не получилось получить историю с сервера.");setStatus("error");
    }
  },[baseURL]);
  useEffect(()=>{void load();return()=>{epoch.current++;};},[load]);
  const loadMore=async()=>{
    if(paging||pagingInFlight.current||!view?.next_history_cursor)return;
    pagingInFlight.current=true;
    const generation=epoch.current;
    const originalID=view.identity_id;
    const cursor=view.next_history_cursor;
    setPaging(true);setMessage("");
    try{
      const url=new URL("/v1/client/personal",baseURL);
      url.searchParams.set("history_cursor",cursor);
      const response=await fetch(url.toString(),{
        credentials:"include",headers:{Accept:"application/json"},cache:"no-store",
      });
      if(!response.ok)throw new Error("История недоступна.");
      const next=parseClientPersonalView(await response.json());
      if(generation!==epoch.current)return;
      if(next.identity_id!==originalID){
        await load();return;
      }
      setView(previous=>{
        if(!previous||previous.identity_id!==originalID)return previous;
        const existing=new Set(previous.history.map(x=>x.booking_id));
        return {...previous,history:[...previous.history,...next.history.filter(x=>!existing.has(x.booking_id))],
          next_history_cursor:next.next_history_cursor};
      });
    }catch{
      if(generation===epoch.current)setMessage("Не удалось загрузить следующую страницу истории.");
    }finally{
      if(generation===epoch.current){pagingInFlight.current=false;setPaging(false);}
    }
  };
  const save=async()=>{
    if(!editing||busy)return;
    setBusy(true);setMessage("");
    try{
      const res=await fetch(new URL("/v1/client/personal/preferences",baseURL).toString(),{
        method:"PUT",credentials:"include",headers:{"content-type":"application/json"},
        body:JSON.stringify({display_name:editing.display_name,locale:editing.locale,time_zone:editing.time_zone}),
      });
      if(!res.ok)throw new Error();
      await load();
    }catch{setMessage("Изменения не сохранены. Повторите попытку.");}
    finally{setBusy(false);}
  };
  const checkIn=async(bookingID:string,progress:ClientPersonalCheckInInput["progress_code"])=>{
    if(busy)return;
    setBusy(true);setMessage("");
    try{
      const res=await fetch(new URL("/v1/client/personal/check-ins",baseURL).toString(),{
        method:"POST",credentials:"include",headers:{"content-type":"application/json"},
        body:JSON.stringify({booking_id:bookingID,progress_code:progress}),
      });
      if(!res.ok)throw new Error("CHECK_IN_NOT_SAVED");
      await load();
    }catch{setMessage("Ответ не сохранён. Проверьте статус консультации.");}
    finally{setBusy(false);}
  };
  return <View style={{gap:10,padding:14,borderWidth:1,borderColor:"#cbd5cf",borderRadius:14}} accessibilityLabel="Личный кабинет клиента">
    <Text style={{fontWeight:"700",fontSize:20}}>Мой кабинет</Text>
    {status==="loading"?<Text accessibilityRole="summary">Загружаем данные с сервера…</Text>:null}
    {status==="auth"?<Text accessibilityRole="alert">После оформления первого запроса здесь появятся ваши записи. Войдите в сессию APGIC.</Text>:null}
    {status==="error"?<Text accessibilityRole="alert">{message}</Text>:null}
    {view?<><Text>Предстоящие записи: {view.upcoming.length}</Text>
      {view.upcoming.map(x=><View key={x.booking_id} style={{paddingVertical:7}}>
        <Text>{x.specialist_name}</Text><Text>{new Date(x.starts_at).toLocaleString("ru-RU")}</Text>
        <Text>{personalBookingLabel[x.state]}</Text>
      </View>)}
      <Text style={{fontWeight:"700"}}>История встреч</Text>
      {view.history.map(x=><View key={x.booking_id} style={{paddingVertical:7}}>
        <Text>{x.specialist_name} · {personalBookingLabel[x.state]}</Text>
        {x.client_progress?<Text>{personalProgressLabel[x.client_progress]}</Text>:null}
        {x.consultation_state==="COMPLETED"&&!x.client_progress?
          <View style={{gap:8,paddingVertical:8}}>
            <Text>Как вы сейчас себя чувствуете? Это не медицинский вывод.</Text>
            {(["IMPROVED","UNCHANGED","WORSE","UNSURE"] as const).map(code=>
              <Pressable key={code} accessibilityRole="button" disabled={busy}
                onPress={()=>void checkIn(x.booking_id,code)}
                style={{padding:12,minHeight:48,borderWidth:1,borderRadius:8}}>
                <Text>{personalProgressLabel[code]}</Text>
              </Pressable>)}
          </View>:null}
      </View>)}
      {view.next_history_cursor?<Pressable accessibilityRole="button" disabled={paging||busy}
        onPress={()=>void loadMore()}
        style={{padding:12,minHeight:48,borderWidth:1,borderRadius:8}}>
        <Text>{paging?"Загружаем историю…":"Показать более ранние записи"}</Text>
      </Pressable>:null}
      <Text>Последние запросы: {view.requests.length}</Text>
      <TextInput accessibilityLabel="Как к вам обращаться" placeholder="Ваше имя"
        maxLength={80} value={editing?.display_name??""}
        onChangeText={value=>setEditing(p=>p?{...p,display_name:value}:p)}
        style={{borderWidth:1,padding:12,borderRadius:8}}/>
      <Pressable accessibilityRole="button" disabled={busy} onPress={()=>void save()} style={{padding:12,borderWidth:1,borderRadius:8,minHeight:48}}>
        <Text>{busy?"Сохраняем…":"Сохранить имя"}</Text>
      </Pressable>
    </>:null}
    {message&&status==="ready"?<Text accessibilityRole="alert">{message}</Text>:null}
    <Pressable accessibilityRole="button" onPress={()=>void load()} style={{padding:12,minHeight:48,borderWidth:1,borderRadius:8}}>
      <Text>Обновить кабинет</Text>
    </Pressable>
  </View>;
}
