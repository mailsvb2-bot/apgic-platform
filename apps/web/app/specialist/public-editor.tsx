"use client";
import {FormEvent,useEffect,useState} from "react";
type Details={headline:string;about:string;experience_years:number;languages:string[];limitations:string[]};
export function SpecialistPublicEditor(){
 const [ready,setReady]=useState(false),[busy,setBusy]=useState(false);
 const [notice,setNotice]=useState("");
 const [headline,setHeadline]=useState(""),[about,setAbout]=useState("");
 const [years,setYears]=useState(0),[languages,setLanguages]=useState("ru"),[limitations,setLimitations]=useState("");
 useEffect(()=>{
  let active=true;
  void fetch("/v1/specialist/public-details",{cache:"no-store"}).then(async res=>{
   if(!res.ok){if(active)setNotice(res.status===404?"Сначала создайте профессиональный профиль выше.":"Данные анкеты пока недоступны.");return;}
   const data=await res.json() as Details;
   if(active){setHeadline(data.headline);setAbout(data.about);setYears(data.experience_years);
    setLanguages(data.languages.join(", "));setLimitations(data.limitations.join("\n"));setReady(true);}
  }).catch(()=>{if(active)setNotice("Не удалось загрузить анкету.");});
  return ()=>{active=false};
 },[]);
 async function save(e:FormEvent){
  e.preventDefault();setBusy(true);setNotice("");
  try{
   const res=await fetch("/v1/specialist/public-details",{
    method:"PUT",headers:{"content-type":"application/json"},
    body:JSON.stringify({headline,about,experience_years:years,
     languages:languages.split(",").map(x=>x.trim()).filter(Boolean),
     limitations:limitations.split("\n").map(x=>x.trim()).filter(Boolean)})
   });
   const data=await res.json() as {notice?:string;message_safe?:string};
   setNotice(res.ok?"Анкета сохранена. Если данные изменились, публикация приостанавливается до повторного одобрения.":data.message_safe??"Не удалось сохранить анкету.");
  }catch{setNotice("Не удалось отправить анкету. Повторите попытку.");}
  finally{setBusy(false)}
 }
 return <section className="panel specialist-form" aria-labelledby="public-details-title">
  <h3 id="public-details-title">Описание публичной анкеты</h3>
  <p>Эти сведения увидят клиенты только после проверки и публикации. Не вводите личные контакты, реквизиты или номера документов.</p>
  {ready?<form onSubmit={save}>
   <label htmlFor="public-headline">Короткое профессиональное описание</label>
   <input id="public-headline" maxLength={120} value={headline} onChange={e=>setHeadline(e.target.value)}/>
   <label htmlFor="public-bio">О себе и подходе</label>
   <textarea id="public-bio" maxLength={3000} rows={6} value={about} onChange={e=>setAbout(e.target.value)}/>
   <label htmlFor="public-experience">Опыт работы (лет, по заявлению специалиста)</label>
   <input id="public-experience" type="number" min={0} max={75} value={years} onChange={e=>setYears(Number(e.target.value))}/>
   <label htmlFor="public-languages">Языки (коды через запятую, например ru, en)</label>
   <input id="public-languages" value={languages} onChange={e=>setLanguages(e.target.value)}/>
   <label htmlFor="public-limitations">С чем не работаете (одна тема на строку)</label>
   <textarea id="public-limitations" rows={3} value={limitations} onChange={e=>setLimitations(e.target.value)}/>
   <button type="submit" disabled={busy}>Сохранить описание</button>
  </form>:<p>Описание станет доступно после создания профиля.</p>}
  {notice?<p role="status">{notice}</p>:null}
 </section>;
}
