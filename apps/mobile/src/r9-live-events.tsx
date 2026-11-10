import React, {useCallback, useEffect, useState} from "react";
import {ActivityIndicator, Pressable, StyleSheet, Text, View} from "react-native";
import type {LiveEvent,LiveRegistration,LiveAccessEligibility} from "../../../packages/contracts/src/r9-live-events.ts";

type Props={baseURL:string;enabled:boolean};
type ListResponse={events:LiveEvent[]};
function r9URL(base:string,path:string){return base.replace(/\/$/,"")+path}
async function jsonRequest<T>(url:string,method:"GET"|"POST"="GET"):Promise<T>{
 const response=await fetch(url,{method,headers:{"Accept":"application/json",...(method==="POST"?{"Content-Type":"application/json"}:{})},credentials:"include"});
 const result:unknown=await response.json().catch(()=>null);
 if(!response.ok)throw new Error(`Сервис мероприятий недоступен: ${response.status}`);
 return result as T;
}
/** R9 is an opt-in native surface of the same APGIC server truth. */
export function LiveEventsPanel({baseURL,enabled}:Props){
 const [events,setEvents]=useState<LiveEvent[]>([]);
 const [selected,setSelected]=useState<string|null>(null);
 const [registered,setRegistered]=useState<LiveRegistration|null>(null);
 const [access,setAccess]=useState<LiveAccessEligibility|null>(null);
 const [loading,setLoading]=useState(false);
 const [busy,setBusy]=useState(false);
 const [error,setError]=useState("");
 const [message,setMessage]=useState("");
 const refresh=useCallback(async()=>{
  if(!enabled)return;
  setLoading(true);setError("");
  try{
   const result=await jsonRequest<ListResponse>(r9URL(baseURL,"/v1/live-events"));
   setEvents(result.events??[]);
  }catch{setError("Мероприятия пока недоступны.")}
  finally{setLoading(false)}
 },[baseURL,enabled]);
 useEffect(()=>{void refresh()},[refresh]);
 useEffect(()=>{
  if(!selected||!enabled){setRegistered(null);return}
  let alive=true;
  jsonRequest<LiveRegistration>(r9URL(baseURL,`/v1/live-events/${encodeURIComponent(selected)}/registration`))
   .then(x=>{if(alive)setRegistered(x)}).catch(()=>{if(alive)setRegistered(null)});
  return ()=>{alive=false}
 },[baseURL,selected,enabled]);
 async function change(method:"registrations"|"unregister"){
  if(!selected)return;
  setBusy(true);setMessage("");setError("");
  try{
   const record=await jsonRequest<LiveRegistration>(r9URL(baseURL,`/v1/live-events/${encodeURIComponent(selected)}/${method}`),"POST");
   setRegistered(record);setAccess(null);
   setMessage(method==="registrations"?"Регистрация подтверждена.":"Вы отменили регистрацию.");
   await refresh();
  }catch{setError("Действие недоступно. Проверьте запись и попробуйте ещё раз.")}
  finally{setBusy(false)}
 }
 async function verifyAccess(){
  if(!selected)return;
  setBusy(true);setError("");
  try{
   const response=await jsonRequest<LiveAccessEligibility>(r9URL(baseURL,`/v1/live-events/${encodeURIComponent(selected)}/access`));
   setAccess(response);
  }catch{setError("Проверка подключения сейчас недоступна.")}
  finally{setBusy(false)}
 }
 if(!enabled)return null;
 const event=events.find(x=>x.id===selected);
 return <View style={styles.root} accessibilityLabel="native-r9-live-events">
  <Text style={styles.title} accessibilityRole="header">Группы и прямые эфиры</Text>
  <Text>Участие без оплаты. Видеосервис подключается только после подтверждения готовности.</Text>
  {loading&&<ActivityIndicator accessibilityLabel="Загрузка мероприятий"/>}
  {error!==""&&<Text accessibilityRole="alert">{error}</Text>}
  {!loading&&!error&&events.length===0&&<Text>Пока нет опубликованных мероприятий.</Text>}
  {events.map(item=><Pressable key={item.id} accessibilityRole="button" style={styles.action}
   accessibilityLabel={`Выбрать мероприятие ${item.title}`} onPress={()=>{setSelected(item.id);setAccess(null)}}>
   <Text style={styles.strong}>{item.title}</Text>
   <Text>{new Date(item.starts_at).toLocaleString("ru-RU")} · {item.registered}/{item.capacity}</Text>
  </Pressable>)}
  {event&&<View style={styles.detail}><Text style={styles.strong}>{event.title}</Text><Text>{event.description}</Text>
    <Text>Запись: {event.recording_available?"По согласованию":"не ведётся"}</Text>
    {registered?.state==="REGISTERED"?<>
     <Text accessibilityRole="summary">Вы зарегистрированы.</Text>
     <Pressable accessibilityRole="button" disabled={busy} style={styles.action} onPress={()=>void verifyAccess()}><Text>Проверить подключение</Text></Pressable>
     <Pressable accessibilityRole="button" disabled={busy} style={styles.action} onPress={()=>void change("unregister")}><Text>Отменить регистрацию</Text></Pressable>
     {access&&<Text accessibilityRole="summary">{access.reason_code==="LIVE_PROVIDER_TICKET_NOT_CONFIGURED"?"Сервис видеосвязи пока не выдал доступ.":access.reason_code}</Text>}
    </>:<Pressable accessibilityRole="button" disabled={busy||event.registered>=event.capacity} style={styles.action} onPress={()=>void change("registrations")}><Text>Зарегистрироваться</Text></Pressable>}
   {message!==""&&<Text accessibilityRole="summary">{message}</Text>}
  </View>}
 </View>;
}
const styles=StyleSheet.create({
 root:{marginTop:24,gap:12,padding:12,borderWidth:1,borderColor:"#aebfb2",borderRadius:12},
 title:{fontWeight:"800",fontSize:22},
 strong:{fontWeight:"700",fontSize:17},
 detail:{gap:12,padding:12},
 action:{padding:14,borderRadius:8,borderWidth:1,borderColor:"#748f80",minHeight:48,justifyContent:"center"}
});
