"use client";
import { FormEvent, useEffect, useState } from "react";
import Link from "next/link";
type Organization={id:string;name:string};
type Event={id:string;title:string;state:string};
export default function ManageLiveEvents(){
 const [organizations,setOrganizations]=useState<Organization[]>([]);
 const [org,setOrg]=useState("");
 const [title,setTitle]=useState("");
 const [description,setDescription]=useState("");
 const [format,setFormat]=useState("WEBINAR");
 const [start,setStart]=useState("");
 const [duration,setDuration]=useState(60);
 const [capacity,setCapacity]=useState(25);
 const [created,setCreated]=useState<Event|null>(null);
 const [error,setError]=useState("");
 const [busy,setBusy]=useState(false);
 useEffect(()=>{
  fetch("/v1/organizations",{credentials:"include",cache:"no-store"}).then(async r=>{
   if(!r.ok)throw new Error("Для создания мероприятия нужен вход и организация.");
   return r.json() as Promise<{organizations:Organization[]}>;
  }).then(r=>{setOrganizations(r.organizations||[]);if(r.organizations?.[0])setOrg(r.organizations[0].id)})
  .catch(e=>setError(e instanceof Error?e.message:"Организации недоступны."));
 },[]);
 async function submit(e:FormEvent){
  e.preventDefault();setError("");setBusy(true);
  try{
   const startDate=new Date(start);if(!Number.isFinite(startDate.getTime()))throw new Error("Укажите дату.");
   const data={title,description,format,starts_at:startDate.toISOString(),ends_at:new Date(startDate.getTime()+duration*60000).toISOString(),capacity,price_minor:0,currency:"RUB"};
   const key=crypto.randomUUID()+"-"+crypto.randomUUID();
   const response=await fetch(`/v1/organizations/${encodeURIComponent(org)}/live-events`,{method:"POST",credentials:"include",
    headers:{"Content-Type":"application/json","Idempotency-Key":key},body:JSON.stringify(data)});
   const payload=await response.json();
   if(!response.ok)throw new Error(payload.error?.message||payload.message||"Не удалось создать мероприятие.");
   setCreated(payload as Event);
  }catch(e){setError(e instanceof Error?e.message:"Не удалось создать мероприятие.")}finally{setBusy(false)}
 }
 return <main className="site-shell"><header className="site-header"><Link className="brand" href="/"><span className="brand-mark">A</span> APGIC</Link><Link href="/live-events">Афиша</Link></header>
  <section className="live-events-panel"><p className="section-kicker">Организатору</p><h1>Новое групповое мероприятие</h1>
  <p>Создаётся черновик. Публикация требует подтверждённой работоспособности SFU-провайдера. Платные мероприятия пока заблокированы, чтобы не создавать фиктивные платежи.</p>
  <form className="live-form" onSubmit={e=>void submit(e)}>
   <label>Организация<select required value={org} onChange={e=>setOrg(e.target.value)}>{organizations.map(x=><option key={x.id} value={x.id}>{x.name}</option>)}</select></label>
   <label>Название<input required minLength={5} maxLength={160} value={title} onChange={e=>setTitle(e.target.value)}/></label>
   <label>Описание<textarea maxLength={4000} value={description} onChange={e=>setDescription(e.target.value)} /></label>
   <label>Формат<select value={format} onChange={e=>setFormat(e.target.value)}><option value="WEBINAR">Вебинар</option><option value="GROUP">Групповая встреча</option><option value="WORKSHOP">Мастер-класс</option><option value="QA">Вопросы и ответы</option><option value="CONFERENCE">Конференция</option></select></label>
   <label>Начало<input required type="datetime-local" value={start} onChange={e=>setStart(e.target.value)}/></label>
   <label>Продолжительность, минут<input type="number" min={15} max={480} required value={duration} onChange={e=>setDuration(Number(e.target.value))}/></label>
   <label>Мест<input type="number" min={2} max={1000} required value={capacity} onChange={e=>setCapacity(Number(e.target.value))}/></label>
   <button disabled={busy||!org} type="submit">Создать черновик</button>
  </form>
  {error&&<p role="alert">{error}</p>}
  {created&&<p role="status">Черновик «{created.title}» создан. <Link href={`/live-events/${created.id}`}>Открыть</Link>. Для публикации требуется готовность коммуникационного провайдера.</p>}
  </section>
 </main>
}
