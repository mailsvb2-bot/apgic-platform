"use client";

import {useEffect,useState} from "react";

import type {
 PublicSpecialist as Profile,
 PublicSpecialistDirectory as Directory,
} from "../../../../packages/contracts/src/generated/apgic-v1";

const topics:Record<string,string>={
 anxiety:"Тревога и стресс",sleep:"Сон",relationships:"Отношения",career:"Карьера и работа"
};
const professions:Record<string,string>={PSYCHOLOGIST:"Психолог",CAREER_COACH:"Карьерный консультант"};
function initials(name:string){return name.split(/\s+/).slice(0,2).map(p=>p[0]??"").join("").toUpperCase();}
export function SpecialistDirectory(){
 const [topic,setTopic]=useState("");
 const [offset,setOffset]=useState(0);
 const [data,setData]=useState<Directory|null>(null);
 const [loading,setLoading]=useState(true);
 const [error,setError]=useState("");
 useEffect(()=>{
  const controller=new AbortController();
  setLoading(true);setError("");setData(null);
  const params=new URLSearchParams({limit:"10",offset:String(offset)});
  if(topic)params.set("topic",topic);
  void fetch(`/v1/specialists?${params}`,{cache:"no-store",signal:controller.signal})
   .then(async res=>{
    if(!res.ok)throw new Error(res.status===503?"Каталог временно недоступен. Попробуйте повторить запрос.":"Не удалось загрузить каталог.");
    return await res.json() as Directory;
   }).then(result=>{
    if(!controller.signal.aborted){setData(result);setLoading(false);}
   }).catch(()=>{
    if(!controller.signal.aborted){setError("Не удалось получить проверенные анкеты. Попробуйте ещё раз.");setLoading(false);}
   });
  return ()=>controller.abort();
 },[topic,offset]);
 return <section className="marketplace-directory" aria-labelledby="directory-title">
  <div className="marketplace-filter">
   <div>
    <h2 id="directory-title">Опубликованные анкеты</h2>
    <p>Каталог не является персональной рекомендацией. Подбор под ваш запрос выполняется после подтверждения HelpIntent.</p>
   </div>
   <label htmlFor="directory-topic">Направление
    <select id="directory-topic" value={topic} onChange={e=>{setTopic(e.target.value);setOffset(0);}}>
     <option value="">Все направления</option>
     {Object.entries(topics).map(([key,label])=><option key={key} value={key}>{label}</option>)}
    </select>
   </label>
  </div>
  {loading?<p role="status">Загружаем актуальные анкеты…</p>:null}
  {error?<p role="alert">{error}</p>:null}
  {data && data.profiles.length===0?<div className="panel"><p>Сейчас нет опубликованных проверенных анкет по этому направлению.</p><a href="/#start">Описать запрос и найти альтернативу</a></div>:null}
  {data && data.profiles.length>0?<ul className="marketplace-grid">
   {data.profiles.map(p=><li className="panel marketplace-profile-card" key={p.id}>
    <div className="marketplace-avatar" aria-hidden="true">{initials(p.display_name)}</div>
    <div className="marketplace-profile-copy">
     <h3><a href={`/specialist/${encodeURIComponent(p.id)}`}>{p.display_name}</a></h3>
     <p>{professions[p.profession_code]??"Специалист"}</p>
     {p.headline?<p>{p.headline}</p>:null}
     {p.experience_years>0?<p>Опыт: {p.experience_years} лет</p>:null}
      {p.verified_review_count>=5&&typeof p.verified_rating==="number"?
       <p>Подтверждённый рейтинг: {p.verified_rating.toFixed(1)} / 5 ({p.verified_review_count} оценок)</p>:
       <p>Рейтинг по завершённым консультациям пока не сформирован</p>}
     <p className="marketplace-topic-list">{p.published_topics.map(x=>topics[x]??x).join(" · ")}</p>
     <p className="marketplace-verified">Подтверждены только перечисленные направления</p>
    </div>
    <a className="button-link primary-action" href={`/specialist/${encodeURIComponent(p.id)}`}>Посмотреть анкету</a>
   </li>)}
  </ul>:null}
  {data?<div className="marketplace-pagination">
   <button type="button" disabled={offset===0||loading} onClick={()=>setOffset(Math.max(0,offset-10))}>Назад</button>
   <span aria-live="polite">Страница {Math.floor(offset/10)+1}</span>
   <button type="button" disabled={!data.has_more||loading} onClick={()=>setOffset(offset+10)}>Далее</button>
  </div>:null}
 </section>;
}
