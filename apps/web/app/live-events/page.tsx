"use client";
import { useEffect, useState } from "react";
import Link from "next/link";

type LiveEvent = { id: string; title: string; description: string; format: string; starts_at: string; ends_at: string; capacity: number; registered: number; state: string; price_minor: number; };
const formatNames: Record<string,string>={ WEBINAR:"Вебинар",GROUP:"Групповая встреча",WORKSHOP:"Мастер-класс",QA:"Вопросы и ответы",CONFERENCE:"Конференция" };
export default function LiveEventsPage(){
 const [items,setItems]=useState<LiveEvent[]>([]);
 const [loading,setLoading]=useState(true);
 const [error,setError]=useState("");
 useEffect(()=>{
  let active=true;
  fetch("/v1/live-events",{cache:"no-store",credentials:"include"}).then(async r=>{
   if(!r.ok)throw new Error(r.status===404?"Раздел пока не подключён.":"Мероприятия сейчас недоступны.");
   return r.json() as Promise<{events:LiveEvent[]}>;
  }).then(x=>{if(active)setItems(x.events||[])}).catch(e=>{if(active)setError(e instanceof Error?e.message:"Не удалось загрузить список.")}).finally(()=>{if(active)setLoading(false)});
  return ()=>{active=false};
 },[]);
 return <main className="site-shell">
  <header className="site-header"><Link className="brand" href="/"><span className="brand-mark">A</span> APGIC</Link><nav className="top-nav" aria-label="Разделы"><Link href="/">Главная</Link><Link href="/organization/live-events">Организатору</Link></nav></header>
  <section className="live-events-panel">
   <p className="section-kicker">Прямые эфиры и группы</p>
   <h1>Встречи, вебинары и мастер-классы</h1>
   <p>Регистрация проходит в APGIC. Подключение к эфиру будет доступно только после подтверждения готовности видеосервиса.</p>
   {loading&&<p role="status">Загружаем мероприятия…</p>}
   {error&&<p role="alert">{error}</p>}
   {!loading&&!error&&items.length===0&&<p>Сейчас нет опубликованных мероприятий.</p>}
   <ul className="live-events-grid">{items.map(e=><li key={e.id} className="live-event-card">
    <p className="section-kicker">{formatNames[e.format]||e.format}</p>
    <h2><Link href={`/live-events/${encodeURIComponent(e.id)}`}>{e.title}</Link></h2>
    <p>{e.description}</p>
    <p><strong>Начало:</strong> {new Date(e.starts_at).toLocaleString("ru-RU")}</p>
    <p><strong>Мест:</strong> {e.registered} из {e.capacity}</p>
    <p><strong>Участие:</strong> {e.price_minor===0?"Бесплатно":"Уточняется"}</p>
    <Link className="button-link primary-action" href={`/live-events/${encodeURIComponent(e.id)}`}>Подробнее</Link>
   </li>)}</ul>
  </section>
 </main>
}
