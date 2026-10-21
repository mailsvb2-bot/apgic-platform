"use client";
import {FormEvent,useEffect,useState} from "react";
import type {SpecialistPublicDetails as Details} from "../../../../packages/contracts/src/generated/apgic-v1";
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
   if(!Number.isSafeInteger(years)||years<0||years>75){setNotice("Укажите стаж от 0 до 75 лет.");return;}
   const normalizedLanguages=languages.split(",").map(x=>x.trim().toLowerCase()).filter(Boolean);
   if(normalizedLanguages.length===0||normalizedLanguages.some(x=>!/^[a-z]{2,3}(-[a-z]{2,4})?$/.test(x))){setNotice("Укажите языки двух- или трёхбуквенными кодами, например ru, en.");return;}
   const res=await fetch("/v1/specialist/public-details",{
    method:"PUT",headers:{"content-type":"application/json"},
    body:JSON.stringify({headline,about,experience_years:years,
     languages:normalizedLanguages,
     limitations:limitations.split("\n").map(x=>x.trim()).filter(Boolean)})
   });
   if(!res.ok){
    const data=await res.json().catch(()=>({})) as {message_safe?:string};
    setNotice(data.message_safe??"Не удалось сохранить анкету. Проверьте поля и попробуйте ещё раз.");
    return;
   }
   // Do not assert durable persistence based only on the write response:
   // reload canonical truth from PostgreSQL through the same authorized API.
   const readback=await fetch("/v1/specialist/public-details",{cache:"no-store"});
   if(!readback.ok){
    setNotice("Сервер принял изменение, но проверить сохранённые данные не удалось. Обновите кабинет.");
    return;
   }
   const stored=await readback.json() as Details;
   setHeadline(stored.headline);setAbout(stored.about);
   setYears(stored.experience_years);
   setLanguages(stored.languages.join(", "));
   setLimitations(stored.limitations.join("\n"));
   setNotice("Описание сохранено и повторно загружено с сервера. Публикация зависит от проверки квалификации.");
  }catch{setNotice("Не удалось завершить сохранение или контрольное чтение анкеты. Проверьте соединение и обновите кабинет.");}
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
  {notice?<p className="form-help" role="status">{notice}</p>:null}
 </section>;
}
