"use client";
import { use, useCallback, useEffect, useState } from "react";
import Link from "next/link";

type LiveEvent={id:string;title:string;description:string;format:string;starts_at:string;ends_at:string;capacity:number;registered:number;state:string;price_minor:number;recording_available:boolean};
type Registration={id:string;event_id:string;state:string;registered_at:string};
type Access={allowed:boolean;reason_code:string;provider_ready:boolean};
function messageFromError(value:unknown){return value instanceof Error?value.message:"Не удалось выполнить операцию."}
async function api<T>(path:string,method:"GET"|"POST"="GET"):Promise<T>{
 const r=await fetch(path,{method,credentials:"include",cache:"no-store",headers:method==="POST"?{"Content-Type":"application/json"}:{}});
 const json=await r.json().catch(()=>({})) as {error?:{message?:string};message?:string};
 if(!r.ok)throw new Error(json.error?.message||json.message||`Ошибка ${r.status}`);
 return json as T;
}
export default function EventPage({params}:{params:Promise<{id:string}>}){
 const {id}=use(params);
 const [event,setEvent]=useState<LiveEvent|null>(null);
 const [registration,setRegistration]=useState<Registration|null>(null);
 const [access,setAccess]=useState<Access|null>(null);
 const [busy,setBusy]=useState(false);
 const [loading,setLoading]=useState(true);
 const [error,setError]=useState("");
 const [notice,setNotice]=useState("");
 const path=`/v1/live-events/${encodeURIComponent(id)}`;
 const reload=useCallback(async ()=>{
  const e=await api<LiveEvent>(path);setEvent(e);
  const registrationResponse=await fetch(path+"/registration",{credentials:"include",cache:"no-store"});
  if(registrationResponse.ok)setRegistration(await registrationResponse.json() as Registration);
  else setRegistration(null);
 },[path]);
 useEffect(()=>{let active=true;reload().catch(e=>{if(active)setError(messageFromError(e))}).finally(()=>{if(active)setLoading(false)});return()=>{active=false}},[reload]);
 async function act(operation:"registrations"|"unregister"){
  setBusy(true);setError("");setNotice("");
  try{
   await api(path+"/"+operation,"POST");
   await reload();
   setAccess(null);
   setNotice(operation==="registrations"?"Регистрация подтверждена.":"Регистрация отменена.");
  }catch(e){setError(messageFromError(e))}finally{setBusy(false)}
 }
 async function checkAccess(){
  setBusy(true);setError("");
  try{setAccess(await api<Access>(path+"/access"))}
  catch(e){setError(messageFromError(e))}finally{setBusy(false)}
 }
 return <main className="site-shell">
  <header className="site-header"><Link className="brand" href="/"><span className="brand-mark">A</span> APGIC</Link><Link href="/live-events">Все мероприятия</Link></header>
  <article className="live-events-panel" aria-live="polite">
   {loading&&<p role="status">Загружаем мероприятие…</p>}
   {event&&<><p className="section-kicker">{event.format}</p><h1>{event.title}</h1><p>{event.description}</p>
    <p><strong>Начало:</strong> {new Date(event.starts_at).toLocaleString("ru-RU")}</p>
    <p><strong>Завершение:</strong> {new Date(event.ends_at).toLocaleString("ru-RU")}</p>
    <p><strong>Свободно:</strong> {Math.max(0,event.capacity-event.registered)} из {event.capacity}</p>
    <p><strong>Стоимость:</strong> {event.price_minor===0?"Бесплатно":"Проверка оплаты требуется"}</p>
    <p>Запись трансляции: {event.recording_available?"Предусмотрена с отдельным согласием":"не ведётся"}</p>
    {registration?.state==="REGISTERED"?<div>
     <p role="status">Вы зарегистрированы.</p>
     <div className="live-action-row">
      <button type="button" disabled={busy} onClick={()=>void checkAccess()}>Проверить доступ к эфиру</button>
      <button type="button" disabled={busy} onClick={()=>void act("unregister")}>Отменить регистрацию</button>
     </div>
     {access&&<p role="status">{access.reason_code==="LIVE_PROVIDER_TICKET_NOT_CONFIGURED"?"Подключение ещё не подтверждено провайдером.":access.reason_code}</p>}
    </div>:<button type="button" disabled={busy||event.state!=="PUBLISHED"||event.capacity<=event.registered} onClick={()=>void act("registrations")}>Зарегистрироваться бесплатно</button>}
   </>}
   {notice&&<p role="status">{notice}</p>}
   {error&&<p role="alert">{error}</p>}
  </article>
 </main>
}
