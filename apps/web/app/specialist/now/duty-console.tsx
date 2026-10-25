"use client";
import {useCallback,useEffect,useState} from "react";
type Offer={id:string;topic_id:string;price_minor:number;currency:string;state:string};
type Presence={state:string;offer_id?:string;available_until?:string};
type Dispatch={id:string;state:string;topic_id?:string;price_minor?:number;currency?:string;expires_at?:string;next_action:string};
async function request<T>(path:string,method="GET",body?:unknown):Promise<T>{
 const r=await fetch("/v1"+path,{method,credentials:"same-origin",cache:"no-store",headers:{"Content-Type":"application/json"},body:body===undefined?undefined:JSON.stringify(body)});
 const v=await r.json();if(!r.ok)throw new Error(v.message_safe||v.code||"Ошибка сервиса");return v as T;
}
export function DutyConsole(){
 const [offers,setOffers]=useState<Offer[]>([]);const [selected,setSelected]=useState("");
 const [presence,setPresence]=useState<Presence|null>(null);const [inbox,setInbox]=useState<Dispatch[]>([]);
 const [busy,setBusy]=useState(false);const [error,setError]=useState("");const [message,setMessage]=useState("");
 const refresh=useCallback(async()=>{
  const [p,m]=await Promise.all([request<Presence>("/instant/presence"),request<{dispatches:Dispatch[]}>("/instant/inbox")]);
  setPresence(p);setInbox(m.dispatches||[]);
 },[]);
 useEffect(()=>{
  (async()=>{try{const data=await request<{offers:Offer[]}>("/specialist/offers");setOffers(data.offers||[]);setSelected(data.offers?.find(x=>x.state==="ACTIVE")?.id||"");await refresh();}catch(e){setError(e instanceof Error?e.message:"Ошибка");}})();
 },[refresh]);
 useEffect(()=>{const timer=window.setInterval(()=>{void refresh().catch(e=>setError(e instanceof Error?e.message:"Ошибка"));},3000);return()=>window.clearInterval(timer);},[refresh]);
 async function toggle(available:boolean){
  setBusy(true);setError("");setMessage("");
  try{setPresence(await request<Presence>("/instant/presence","PUT",{available,offer_id:available?selected:""}));setMessage(available?"Дежурство включено на 10 минут. Продлевайте его при необходимости.":"Дежурство выключено.");}
  catch(e){setError(e instanceof Error?e.message:"Ошибка");}finally{setBusy(false);}
 }
 async function answer(id:string,accept:boolean){
  setBusy(true);setError("");
  try{const next=await request<Dispatch>("/instant/dispatches/"+id+"/answer","POST",{accept});setMessage(next.state==="ACCEPTED"?"Предложение принято. Оплата ещё не подтверждена, консультация не началась.":"Предложение отклонено.");await refresh();}
  catch(e){setError(e instanceof Error?e.message:"Ошибка");}finally{setBusy(false);}
 }
 return <div style={{maxWidth:690}}>
  <section style={{padding:20,border:"1px solid #bbb",borderRadius:12,marginTop:18}}>
   <h2>Моя доступность</h2>
   <p aria-live="polite">Текущий статус: <strong>{presence?.state||"Не загружен"}</strong>{presence?.available_until?" · до "+new Date(presence.available_until).toLocaleTimeString("ru-RU"):""}</p>
   <label htmlFor="duty-offer">Предложение для дежурства</label>
   <select id="duty-offer" value={selected} onChange={e=>setSelected(e.target.value)} style={{display:"block",padding:10,marginTop:8,width:"100%"}}>
    {offers.filter(o=>o.state==="ACTIVE").map(o=><option key={o.id} value={o.id}>{o.topic_id} · {(o.price_minor/100).toLocaleString("ru-RU")} {o.currency}</option>)}
   </select>
   {!offers.some(o=>o.state==="ACTIVE")&&<p>Сначала создайте проверенный профиль, опубликуйте продукт и добавьте платное предложение в профессиональном кабинете.</p>}
   <div style={{display:"flex",flexWrap:"wrap",gap:12,marginTop:16}}>
    <button type="button" disabled={busy||!selected} onClick={()=>toggle(true)}>🟢 Принимать сейчас</button>
    <button type="button" disabled={busy} onClick={()=>toggle(false)}>Выйти из дежурства</button>
   </div>
  </section>
  <section style={{padding:20,border:"1px solid #bbb",borderRadius:12,marginTop:18}}>
   <h2>Ожидающие предложения</h2>
   <p>У предложения есть короткий срок ответа. Истёкшие предложения исчезают автоматически.</p>
   {inbox.length===0&&<p>Ожидающих предложений нет.</p>}
   {inbox.map(d=><article key={d.id} style={{padding:"12px 0",borderTop:"1px solid #ccc"}}>
    <strong>Тема: {d.topic_id}</strong> · {d.state}
    {d.price_minor&&<p>Цена: {(d.price_minor/100).toLocaleString("ru-RU")} {d.currency}</p>}
    {d.expires_at&&<p>Срок: {new Date(d.expires_at).toLocaleTimeString("ru-RU")}</p>}
    {d.state==="OFFERED"&&<div style={{display:"flex",gap:12}}><button disabled={busy} onClick={()=>answer(d.id,true)}>Принять</button><button disabled={busy} onClick={()=>answer(d.id,false)}>Отклонить</button></div>}
    {d.state==="ACCEPTED"&&<p>Принято. Для начала требуется подтверждение оплаты; комната недоступна.</p>}
   </article>)}
  </section>
  {message&&<p role="status">{message}</p>}{error&&<p role="alert" style={{color:"#ba2525"}}>{error}</p>}
 </div>;
}
