"use client";

import {useCallback,useEffect,useMemo,useRef,useState} from "react";

type Entry = {slot_id:string;booking_id?:string;booking_state?:string;consultation_state?:string;starts_at:string;ends_at:string;topic_id?:string};
const topics:Record<string,string>={anxiety:"Тревога и стресс",sleep:"Сон",relationships:"Отношения",career:"Карьера"};
function monthRange(month:string){
 const [year,part]=month.split("-").map(Number);
 const from=new Date(year,part-1,1);
 const until=new Date(year,part,1);
 return {from:from.toISOString(),until:until.toISOString()};
}
function state(entry:Entry){
 if(entry.consultation_state==="COMPLETED")return "Консультация завершена";
 if(entry.consultation_state==="TECHNICAL_FAILURE")return "Техническая проблема";
 if(entry.booking_state==="COMPLETED")return "Запись завершена; результат уточняется";
 if(entry.booking_state==="CONFIRMED")return "Запись подтверждена";
 if(entry.booking_id)return "Запись: "+entry.booking_state;
 return "Свободное время";
}
function dateLabel(value:string){return new Intl.DateTimeFormat("ru-RU",{dateStyle:"medium",timeStyle:"short"}).format(new Date(value));}
function localDay(value:string){
 const date=new Date(value);
 if(!Number.isFinite(date.getTime()))return "";
 const year=date.getFullYear(),month=String(date.getMonth()+1).padStart(2,"0"),day=String(date.getDate()).padStart(2,"0");
 return `${year}-${month}-${day}`;
}
function localMonth(){const now=new Date();return `${now.getFullYear()}-${String(now.getMonth()+1).padStart(2,"0")}`;}
function validMonth(value:string){return /^(20[2-9][0-9])-((0[1-9])|(1[0-2]))$/.test(value);}
export function SpecialistWorkspaceCalendar(){
 const [month,setMonth]=useState(localMonth);
 const [items,setItems]=useState<Entry[]>([]);
 const [error,setError]=useState("");
 const [loading,setLoading]=useState(true);
 const requestRef=useRef(0);
 const fetchEntries=useCallback(async(signal?:AbortSignal)=>{
  const request=++requestRef.current;
  if(!validMonth(month)){setError("Выберите корректный месяц.");setLoading(false);return;}
  setLoading(true);setError("");
  try{
   const range=monthRange(month);
   const params=new URLSearchParams(range);
   const response=await fetch(`/v1/specialist/workspace?${params}`,{cache:"no-store",credentials:"include",signal});
   if(!response.ok)throw new Error(response.status===401?"Войдите в кабинет специалиста.":"Не удалось загрузить календарь и записи.");
   const data=await response.json() as {entries:Entry[]};
   if(!signal?.aborted&&request===requestRef.current)setItems(Array.isArray(data.entries)?data.entries:[]);
  }catch(e){if(!signal?.aborted&&request===requestRef.current){setError(e instanceof Error?e.message:"Ошибка загрузки.");setItems([]);}}
  finally{if(!signal?.aborted&&request===requestRef.current)setLoading(false);}
 },[month]);
 useEffect(()=>{const controller=new AbortController();void fetchEntries(controller.signal);return()=>controller.abort();},[fetchEntries]);
 const booked=useMemo(()=>items.filter(e=>Boolean(e.booking_id)&&e.booking_state!=="CANCELLED"),[items]);
 const completed=booked.filter(e=>e.consultation_state==="COMPLETED");
 const upcoming=booked.filter(e=>e.consultation_state!=="COMPLETED"&&new Date(e.ends_at).getTime()>=Date.now());
 const history=booked.filter(e=>e.consultation_state!=="COMPLETED"&&new Date(e.ends_at).getTime()<Date.now());
 const byDay=useMemo(()=>items.reduce<Record<string,Entry[]>>((all,e)=>{
  const day=localDay(e.starts_at);if(day)(all[day]??=[]).push(e);return all;
 },{}),[items]);
 const first=new Date(month+"-01T12:00:00");
 const offset=(first.getDay()+6)%7;
 const days=new Date(first.getFullYear(),first.getMonth()+1,0).getDate();
 return <section className="panel" id="specialist-calendar" aria-label="Календарь и записи специалиста">
  <h2>Мои записи и календарь</h2>
  <p>Данные из APGIC. История завершения определяется подтверждённым состоянием консультации, а не наступлением даты.</p>
  <label htmlFor="specialist-calendar-month">Месяц</label>{" "}
  <input type="month" id="specialist-calendar-month" value={month} onChange={e=>setMonth(e.target.value)} min="2020-01" max="2099-12"/>
  {" "}<button type="button" disabled={loading} onClick={()=>void fetchEntries()}>Обновить</button>
  {loading?<p role="status">Загрузка расписания…</p>:null}
  {error?<p role="alert">{error}</p>:null}
  {!loading&&!error?<div>
   <div style={{display:"grid",gridTemplateColumns:"repeat(7,minmax(0,1fr))",gap:"4px",marginTop:"16px"}}>
    {["Пн","Вт","Ср","Чт","Пт","Сб","Вс"].map(d=><strong key={d} style={{textAlign:"center"}}>{d}</strong>)}
    {Array.from({length:offset},(_,i)=><span key={"blank"+i}/>)}
    {Array.from({length:days},(_,i)=>{
     const date=month+"-"+String(i+1).padStart(2,"0");
     const entries=byDay[date]??[];
     return <div key={date} style={{border:"1px solid var(--line, #ccc)",borderRadius:"8px",padding:"8px 4px",minHeight:"60px",textAlign:"center"}}>
      <strong>{i+1}</strong>
      {entries.length>0?<p style={{fontSize:"0.8em"}}>{entries.filter(e=>e.booking_id).length} зап. / {entries.length} сл.</p>:null}
     </div>;
    })}
   </div>
   <h3>Предстоящие записи ({upcoming.length})</h3>
   {upcoming.length===0?<p>Подтверждённых предстоящих записей за этот месяц пока нет.</p>:<ul>{upcoming.map(e=><li key={e.slot_id}>{dateLabel(e.starts_at)} · {topics[e.topic_id??""]??e.topic_id} · {state(e)} · <a href={`/specialist/consultations/${encodeURIComponent(e.booking_id!)}`}>Открыть консультацию</a></li>)}</ul>}
   <h3>Завершённые консультации ({completed.length})</h3>
   {completed.length===0?<p>Завершённых консультаций за этот месяц пока нет.</p>:<ul>{completed.map(e=><li key={e.slot_id}>{dateLabel(e.starts_at)} · {topics[e.topic_id??""]??e.topic_id} · {state(e)} · <a href={`/specialist/consultations/${encodeURIComponent(e.booking_id!)}`}>История встречи</a></li>)}</ul>}
   <h3>Прошедшие записи без подтверждённого завершения ({history.length})</h3>
   {history.length>0?<ul>{history.map(e=><li key={e.slot_id}>{dateLabel(e.starts_at)} · {state(e)} · <a href={`/specialist/consultations/${encodeURIComponent(e.booking_id!)}`}>Проверить состояние</a></li>)}</ul>:<p>Нет.</p>}
   <h3>Свободное время ({items.filter(e=>!e.booking_id).length})</h3>
   <ul>{items.filter(e=>!e.booking_id).map(e=><li key={e.slot_id}>{dateLabel(e.starts_at)} · {topics[e.topic_id??""]??e.topic_id}</li>)}</ul>
  </div>:null}
 </section>;
}
