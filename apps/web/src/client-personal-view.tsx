"use client";

import {useCallback,useEffect,useRef,useState} from "react";
import {
  personalBookingLabel,personalConsultationLabel,personalProgressLabel,
  type ClientPersonalCheckInInput,
  type ClientPersonalBooking,type ClientPersonalView,type ClientPersonalPreferences,
} from "../../../packages/contracts/src/client-personal";
import {loadClientPersonal,saveClientPersonalPreferences,recordPersonalCheckIn,PersonalAPIError} from "./client-personal-api";

function formatDate(iso:string,zone:string) {
  try{return new Intl.DateTimeFormat("ru-RU",{dateStyle:"medium",timeStyle:"short",timeZone:zone}).format(new Date(iso));}
  catch{return new Date(iso).toLocaleString("ru-RU");}
}
function PersonalCheckIn({booking,onSaved}:{booking:ClientPersonalBooking,onSaved:()=>void}){
  const [progress,setProgress]=useState<ClientPersonalCheckInInput["progress_code"]>("UNSURE");
  const [rating,setRating]=useState("");
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  if(booking.consultation_state!=="COMPLETED")return null;
  if(booking.client_progress)return <p aria-label="Ваша оценка">
    Вы отметили: {personalProgressLabel[booking.client_progress]}
    {booking.match_satisfaction?` · Контакт со специалистом: ${booking.match_satisfaction} из 5`:""}.
  </p>;
  const submit=async(e:React.FormEvent)=>{
    e.preventDefault();
    if(busy)return;
    setBusy(true);setError("");
    try{
      await recordPersonalCheckIn({booking_id:booking.booking_id,progress_code:progress,
        ...(rating?{match_satisfaction:Number(rating)}:{})});
      onSaved();
    }catch(cause){setError(cause instanceof Error?cause.message:"Ответ не сохранён.");}
    finally{setBusy(false);}
  };
  return <form className="personal-check-in" onSubmit={e=>void submit(e)}>
    <p>Как вы сейчас оцениваете своё состояние? Это ваша личная оценка, не медицинское заключение.</p>
    <label>Самочувствие
      <select value={progress} onChange={e=>setProgress(e.target.value as typeof progress)}>
        {(Object.keys(personalProgressLabel) as Array<typeof progress>).map(code=>
          <option key={code} value={code}>{personalProgressLabel[code]}</option>)}
      </select>
    </label>
    <label>Насколько подошёл специалист (необязательно)
      <select value={rating} onChange={e=>setRating(e.target.value)}>
        <option value="">Не оценивать</option>
        {[1,2,3,4,5].map(n=><option key={n} value={String(n)}>{n} из 5</option>)}
      </select>
    </label>
    <button type="submit" disabled={busy}>{busy?"Сохраняем…":"Сохранить оценку"}</button>
    {error?<p role="alert">{error}</p>:null}
  </form>;
}
function BookingList({items,zone,onSaved}:{items:ClientPersonalBooking[],zone:string,onSaved?:()=>void}){
  if(!items.length)return <p className="personal-muted">Здесь пока нет записей.</p>;
  return <ul className="personal-booking-list">{items.map(item=>
    <li className="personal-booking" key={item.booking_id}>
      <div><strong>{item.specialist_name}</strong>
        {item.specialist_profession?<span> · {item.specialist_profession}</span>:null}
      </div>
      <div><time dateTime={item.starts_at}>{formatDate(item.starts_at,zone)}</time>
        <span className="personal-state">{personalBookingLabel[item.state]}</span>
      </div>
      {item.consultation_state?<p>Консультация: {personalConsultationLabel[item.consultation_state]}</p>:null}
      <a href={`/bookings/${encodeURIComponent(item.booking_id)}`}>Детали записи</a>
      {onSaved?<PersonalCheckIn booking={item} onSaved={onSaved}/>:null}
    </li>)}</ul>;
}
export default function ClientPersonalView(){
  const [data,setData]=useState<ClientPersonalView|null>(null);
  const [status,setStatus]=useState<"loading"|"ready"|"unauthorized"|"error">("loading");
  const [error,setError]=useState("");
  const [loadingMore,setLoadingMore]=useState(false);
  const [saving,setSaving]=useState(false);
  const [draft,setDraft]=useState<ClientPersonalPreferences|null>(null);
  const epoch=useRef(0);
  const reload=useCallback(async()=>{
    const current=++epoch.current;
    setStatus("loading");setData(null);setError("");setDraft(null);
    try{
      const value=await loadClientPersonal();
      if(current!==epoch.current)return;
      setData(value);setDraft(value.preferences);setStatus("ready");
    }catch(e){
      if(current!==epoch.current)return;
      setStatus(e instanceof PersonalAPIError&&e.reason==="SESSION"?"unauthorized":"error");
      setError(e instanceof Error?e.message:"Личный кабинет временно недоступен.");
    }
  },[]);
  useEffect(()=>{void reload();return ()=>{epoch.current++;};},[reload]);
  const loadMore=async()=>{
    if(!data?.next_history_cursor||loadingMore)return;
    const cursor=data.next_history_cursor;
    const current=epoch.current;setLoadingMore(true);
    try{
      const value=await loadClientPersonal(cursor);
      if(current!==epoch.current)return;
      if(value.identity_id!==data.identity_id)throw new Error("Сессия изменилась. Обновите страницу.");
      setData(previous=>{
        if(!previous||previous.identity_id!==value.identity_id)return previous;
        const existing=new Set(previous.history.map(x=>x.booking_id));
        return {...previous,
          history:[...previous.history,...value.history.filter(x=>!existing.has(x.booking_id))],
          next_history_cursor:value.next_history_cursor};
      });
    }catch(e){
      if(current===epoch.current){
        if(e instanceof PersonalAPIError&&e.reason==="SESSION"){
          epoch.current++;setData(null);setDraft(null);setStatus("unauthorized");
        }
        setError(e instanceof Error?e.message:"Не удалось загрузить историю.");
      }
    }
    finally{if(current===epoch.current)setLoadingMore(false);}
  };
  const save=async(e:React.FormEvent)=>{
    e.preventDefault();
    if(!draft||!data||saving)return;
    const current=epoch.current;
    setSaving(true);setError("");
    try{
      const actual=await saveClientPersonalPreferences(draft);
      if(current!==epoch.current)return;
      setData(previous=>previous?.identity_id===data.identity_id?
        {...previous,preferences:actual}:previous);
      setDraft(actual);
    }catch(e){
      if(current===epoch.current){
        if(e instanceof PersonalAPIError&&e.reason==="SESSION"){
          epoch.current++;setData(null);setDraft(null);setStatus("unauthorized");
        }
        setError(e instanceof Error?e.message:"Не удалось сохранить настройки.");
      }
    }
    finally{setSaving(false);}
  };
  return <section className="personal-main" aria-label="Личный кабинет клиента">
    {status==="loading"?<p role="status">Загружаем ваши записи с сервера…</p>:null}
    {status==="unauthorized"?<div role="alert" className="personal-empty"><p>{error}</p><a href="/#start">Начать подбор</a></div>:null}
    {status==="error"?<div role="alert" className="personal-empty"><p>{error}</p><button onClick={()=>void reload()}>Повторить</button></div>:null}
    {data&&status==="ready"?<>
      <div className="personal-toolbar">
        <p>Добро пожаловать{data.preferences.display_name?`, ${data.preferences.display_name}`:""}.</p>
        <button type="button" onClick={()=>void reload()}>Обновить историю</button>
      </div>
      <div className="personal-grid">
        <section className="personal-section" aria-labelledby="personal-upcoming">
          <h2 id="personal-upcoming">Предстоящие записи</h2>
          <BookingList items={data.upcoming} zone={data.preferences.time_zone}/>
          <a href="/#start">Найти специалиста</a>
        </section>
        <section className="personal-section" aria-labelledby="personal-requests">
          <h2 id="personal-requests">Ваши запросы</h2>
          {!data.requests.length?<p className="personal-muted">Ваши запросы появятся здесь после первого обращения.</p>:
          <ul className="personal-request-list">{data.requests.map(x=><li key={x.id}>
            <strong>{x.state==="CONFIRMED"?"Подтверждённый запрос":"Черновик запроса"}</strong>
            <span>{formatDate(x.created_at,data.preferences.time_zone)}</span>
            <p>{x.topics.length?x.topics.join(" · "):"Темы ещё не подтверждены"}</p>
          </li>)}</ul>}
        </section>
      </div>
      <section className="personal-section" id="personal-history" aria-labelledby="personal-history">
        <h2 id="personal-history">История встреч</h2>
        <BookingList items={data.history} zone={data.preferences.time_zone} onSaved={()=>void reload()}/>
        {data.next_history_cursor?<button type="button" disabled={loadingMore} onClick={()=>void loadMore()}>
          {loadingMore?"Загружаем…":"Показать более ранние записи"}
        </button>:null}
      </section>
      <section className="personal-section" aria-labelledby="personal-profile">
        <h2 id="personal-profile">Персональные настройки</h2>
        <form className="personal-preferences" onSubmit={e=>void save(e)}>
          <label>Как к вам обращаться
            <input maxLength={80} autoComplete="nickname" value={draft?.display_name??""}
              onChange={e=>setDraft(p=>p?{...p,display_name:e.target.value}:p)}/>
          </label>
          <label>Язык
            <select value={draft?.locale??"ru"} onChange={e=>setDraft(p=>p?{...p,locale:e.target.value as "ru"|"en"}:p)}>
              <option value="ru">Русский</option><option value="en">English</option>
            </select>
          </label>
          <label>Часовой пояс
            <select value={draft?.time_zone??"Europe/Moscow"} onChange={e=>setDraft(p=>p?{...p,time_zone:e.target.value}:p)}>
              {Array.from(new Set(["Europe/Moscow","Europe/Amsterdam","Asia/Yekaterinburg","Asia/Novosibirsk","Asia/Vladivostok","UTC",draft?.time_zone].filter((x):x is string=>Boolean(x)))).map(zone=><option key={zone} value={zone}>{zone}</option>)}
            </select>
          </label>
          <button type="submit" disabled={saving}>{saving?"Сохраняем…":"Сохранить настройки"}</button>
        </form>
        <p className="personal-muted">История консультаций и сведения об оплатах поступают только из подтверждённых данных APGIC. Содержимое бесед здесь не хранится.</p>
        <p><a href="/#start">Новый запрос</a></p>
      </section>
      {error?<p role="alert">{error}</p>:null}
    </>:null}
  </section>;
}
