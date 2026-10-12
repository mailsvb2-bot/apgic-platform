"use client";

import {useEffect,useState} from "react";

import type {PublicBookableOfferList,PublicSpecialist as Profile} from "../../../../../packages/contracts/src/generated/apgic-v1";

const topics:Record<string,string>={anxiety:"Тревога и стресс",sleep:"Сон",relationships:"Отношения",career:"Карьера и работа"};
const professions:Record<string,string>={PSYCHOLOGIST:"Психолог",CAREER_COACH:"Карьерный консультант"};
export function PublicSpecialistProfile({id}:{id:string}){
 const [profile,setProfile]=useState<Profile|null>(null);
 const [bookable,setBookable]=useState<PublicBookableOfferList|null>(null);
 const [bookableError,setBookableError]=useState(false);
 const [state,setState]=useState<"LOADING"|"NOT_PUBLIC"|"ERROR"|"READY">("LOADING");
 useEffect(()=>{
  const ctrl=new AbortController();
  setState("LOADING");setProfile(null);
  void fetch(`/v1/specialists/${encodeURIComponent(id)}`,{cache:"no-store",signal:ctrl.signal})
   .then(async response=>{
    if(response.status===404){if(!ctrl.signal.aborted)setState("NOT_PUBLIC");return null;}
    if(!response.ok)throw new Error("Unavailable");
    return await response.json() as Profile;
   })
   .then(data=>{if(data&&!ctrl.signal.aborted){setProfile(data);setState("READY");}})
   .catch(()=>{if(!ctrl.signal.aborted)setState("ERROR");});
  return ()=>ctrl.abort();
 },[id]);
 useEffect(()=>{
  if(!profile){setBookable(null);setBookableError(false);return;}
  const ctrl=new AbortController();
  setBookable(null);setBookableError(false);
  void fetch(`/v1/specialists/${encodeURIComponent(profile.id)}/offers`,{cache:"no-store",signal:ctrl.signal})
   .then(async res=>{
    if(!res.ok)throw new Error("Bookable supply unavailable");
    return await res.json() as PublicBookableOfferList;
   }).then(data=>{if(!ctrl.signal.aborted)setBookable(data);})
   .catch(()=>{if(!ctrl.signal.aborted)setBookableError(true);});
  return ()=>ctrl.abort();
 },[profile]);
 return <main className="site-shell marketplace-shell">
  <header className="site-header">
   <a href="/" className="brand"><span className="brand-mark" aria-hidden="true">A</span> APGIC</a>
   <nav className="top-nav"><a href="/specialists">Каталог</a><a href="/#start">Подобрать под запрос</a></nav>
  </header>
  {state==="LOADING"?<p role="status">Проверяем актуальность публикации…</p>:null}
  {state==="NOT_PUBLIC"?<section className="panel"><h1>Анкета не опубликована</h1>
   <p>Публикация могла быть отозвана, квалификация — отправлена на повторную проверку.</p><a href="/specialists">В каталог</a></section>:null}
  {state==="ERROR"?<section role="alert"><h1>Анкета временно недоступна</h1><a href="/specialists">Вернуться в каталог</a></section>:null}
  {profile?<article className="marketplace-public-profile">
   <div className="marketplace-avatar marketplace-avatar-large" aria-hidden="true">
    {profile.display_name.split(/\s+/).slice(0,2).map(x=>x[0]??"").join("").toUpperCase()}
   </div>
   <div>
    <p className="eyebrow">Опубликованная анкета APGIC</p>
    <h1>{profile.display_name}</h1>
    <p className="hero-lead">{professions[profile.profession_code]??"Специалист"}</p>
    {profile.headline?<p>{profile.headline}</p>:null}
    {profile.experience_years>0?<p>Заявленный опыт: {profile.experience_years} лет</p>:null}
    {profile.about?<section><h2>О специалисте</h2><p className="marketplace-bio">{profile.about}</p></section>:null}
    <section><h2>Подтверждённые направления</h2>
     <ul>{profile.published_topics.map(t=><li key={t}>{topics[t]??t}</li>)}</ul>
     <p>Допуск действует только в указанных направлениях. Подробные документы и личные данные проверки не публикуются.</p>
    </section>
    {profile.languages.length>0?<p>Языки: {profile.languages.join(", ")}</p>:null}
    {profile.limitations.length>0?<section><h2>С чем не работает</h2><ul>{profile.limitations.map((x,i)=><li key={i}>{x}</li>)}</ul></section>:null}
    <section aria-labelledby="specialist-capabilities">
     <h2 id="specialist-capabilities">Что можно получить у этого специалиста</h2>
     {bookableError?<p role="status">Проверить доступные предложения сейчас не удалось. Цена и время записи не подтверждены.</p>:null}
     {!bookable&&!bookableError?<p>Проверяем актуальную стоимость и свободное время…</p>:null}
     {bookable&&bookable.offers.length===0?<p>Подтверждённых свободных слотов сейчас нет. Это не означает, что консультация бесплатна или доступна немедленно.</p>:null}
     {bookable&&bookable.offers.length>0?<ul className="marketplace-offers">
      {bookable.offers.map(offer=><li key={offer.id} className="panel marketplace-offer-card">
       <h3>Консультация: {topics[offer.topic_id]??offer.topic_id}</h3>
       <p className="marketplace-offer-price">{new Intl.NumberFormat("ru-RU",{style:"currency",currency:offer.currency}).format(offer.price_minor/100)}</p>
       <p>Доступное время на момент проверки:</p>
       <ul>{offer.slots.map(slot=><li key={slot.slot_id}>
        {new Intl.DateTimeFormat("ru-RU",{dateStyle:"medium",timeStyle:"short"}).format(new Date(slot.starts_at))}
        {" · "}{Math.round((new Date(slot.ends_at).getTime()-new Date(slot.starts_at).getTime())/60000)} мин.
       </li>)}</ul>
       <a href={`/?specialist_id=${encodeURIComponent(profile.id)}&offer_id=${encodeURIComponent(offer.id)}#start`}>Выбрать эту услугу и перейти к записи</a>
       <p className="status-note">Свободное время не резервируется при просмотре. Доступность и стоимость перепроверяются при бронировании.</p>
      </li>)}
     </ul>:null}
    </section>
    <div className="hero-actions"><a className="button-link primary-action" href={`/?specialist_id=${encodeURIComponent(profile.id)}#start`}>Уточнить запрос и выбрать время</a>
     <a className="button-link secondary-action" href="/specialists">Сравнить специалистов</a></div>
    <p className="status-note">Стоимость и свободное время подтверждаются отдельно при подборе. Анкета не является гарантией записи.</p>
   </div>
  </article>:null}
 </main>;
}
